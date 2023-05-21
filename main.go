package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/getsentry/sentry-go"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/validation"
	k8sVersion "k8s.io/apimachinery/pkg/version"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

func getObjectNameTag(object *v1.ObjectReference) string {
	if object.Kind == "" {
		return "object_name"
	} else {
		return fmt.Sprintf("%s_name", strings.ToLower(object.Kind))
	}
}

func handleEvent(eventObject *v1.Event, hub *sentry.Hub) {
	log.Debug().Msgf("EventObject: %#v", eventObject)
	log.Debug().Msgf("Event type: %#v", eventObject.Type)

	involvedObject := eventObject.InvolvedObject

	hub.WithScope(func(scope *sentry.Scope) {
		scope.SetTag("event_type", eventObject.Type)
		scope.SetTag("reason", eventObject.Reason)
		scope.SetTag("namespace", involvedObject.Namespace)
		scope.SetTag("kind", involvedObject.Kind)
		scope.SetTag("object_UID", string(involvedObject.UID))

		name_tag := getObjectNameTag(&involvedObject)
		scope.SetTag(name_tag, involvedObject.Name)

		if source, err := prettyJson(eventObject.Source); err == nil {
			scope.SetExtra("Event Source", source)
		}
		eventObject.Source = v1.EventSource{}

		if involvedObject, err := prettyJson(eventObject.InvolvedObject); err == nil {
			scope.SetExtra("Involved Object", involvedObject)
		}
		eventObject.InvolvedObject = v1.ObjectReference{}

		// clean-up the event a bit
		eventObject.ObjectMeta.ManagedFields = []metav1.ManagedFieldsEntry{}
		if metadata, err := prettyJson(eventObject.ObjectMeta); err == nil {
			scope.SetExtra("Event Metadata", metadata)
		}
		eventObject.ObjectMeta = metav1.ObjectMeta{}

		// The entire event
		if kubeEvent, err := prettyJson(eventObject); err == nil {
			scope.SetExtra("~ Misc Event Fields", kubeEvent)
		}

		sentryEvent := &sentry.Event{Message: eventObject.Message, Level: sentry.LevelError}
		hub.CaptureEvent(sentryEvent)
	})

}

func watchEventsInNamespace(config *rest.Config, namespace string, watchSince time.Time, hub *sentry.Hub) (err error) {
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return err
	}
	opts := metav1.ListOptions{
		Watch: true,
	}
	log.Debug().Msg("Getting the event watcher...")

	ctx, cancelFunc := context.WithCancel(context.Background())
	defer cancelFunc()

	// FIXME: Watch() currently returns also all recent events.
	// Should we ignore events that happened in the past?
	watcher, err := clientset.CoreV1().Events(namespace).Watch(ctx, opts)
	if err != nil {
		return err
	}

	watchCh := watcher.ResultChan()
	defer watcher.Stop()

	watchSinceWrapped := metav1.Time{Time: watchSince}

	log.Debug().Msg("Reading from the event channel...")
	for event := range watchCh {
		eventObjectRaw := event.Object
		// Watch event type: Added, Delete, Bookmark...
		if (event.Type != watch.Added) && (event.Type != watch.Modified) {
			log.Debug().Msgf("Skipping a watch event of type %s", event.Type)
			continue
		}

		objectKind := eventObjectRaw.GetObjectKind()
		eventObject, ok := eventObjectRaw.(*v1.Event)
		if !ok {
			log.Warn().Msgf("Skipping an event of kind '%v' because it cannot be casted", objectKind)
			continue
		}

		// Get event timestamp
		eventTs := eventObject.LastTimestamp
		if eventTs.IsZero() {
			eventTs = metav1.Time(eventObject.EventTime)
		}

		if !watchSinceWrapped.IsZero() && !eventTs.IsZero() && eventTs.Before(&watchSinceWrapped) {
			log.Debug().Msgf("Ignoring an event because it is too old")
			continue
		}

		if eventObject.Type == v1.EventTypeNormal {
			log.Debug().Msgf("Skipping an event of type %s", eventObject.Type)
			continue
		}

		handleEvent(eventObject, hub)
	}

	return nil
}

func watchEventsInNamespaceForever(config *rest.Config, namespace string) {
	localHub := sentry.CurrentHub().Clone()

	where := fmt.Sprintf("in namespace '%s'", namespace)
	if namespace == v1.NamespaceAll {
		where = "in all namespaces"
	}

	watchFromBeginning := isTruthy(os.Getenv("SENTRY_K8S_WATCH_HISTORICAL"))
	var watchSince time.Time
	if watchFromBeginning {
		watchSince = time.Time{}
		log.Info().Msgf("Watching all available events (no starting timestamp)")
	} else {
		watchSince = time.Now()
		log.Info().Msgf("Watching events starting from: %s", watchSince.Format("Mon, 02 Jan 2006 15:04:05 -0700"))
	}

	for {
		if err := watchEventsInNamespace(config, namespace, watchSince, localHub); err != nil {
			log.Error().Msgf("Error while watching events %s: %s", where, err)
		}
		watchSince = time.Now()
		time.Sleep(time.Second * 1)
	}
}

func getClusterVersion(config *rest.Config) (*k8sVersion.Info, error) {
	versionInfo := &k8sVersion.Info{}

	discoveryClient, err := discovery.NewDiscoveryClientForConfig(config)
	if err != nil {
		return versionInfo, err
	}
	log.Debug().Msgf("Fetching cluster version...")
	versionInfo, err = discoveryClient.ServerVersion()
	log.Debug().Msgf("Cluster version: %s", versionInfo)
	return versionInfo, err
}

func setKubernetesSentryContext(config *rest.Config) {
	kubernetesContext := map[string]interface{}{
		"API endpoint": config.Host,
	}

	// Get cluster version via API
	clusterVersion, err := getClusterVersion(config)
	if err == nil {
		kubernetesContext["Server version"] = clusterVersion.String()
	} else {
		log.Error().Msgf("Error while getting cluster version: %s", err)
	}

	sentry.CurrentHub().Scope().SetContext(
		"Kubernetes",
		kubernetesContext,
	)
}

var defaultNamespacesToWatch = []string{v1.NamespaceDefault}

const allNamespacesLabel = "__all__"

func getNamespacesToWatch() (watchAll bool, namespaces []string, err error) {
	watchNamespacesRaw := strings.TrimSpace(os.Getenv("SENTRY_K8S_WATCH_NAMESPACES"))

	// Nothing in the env variable => use the default value
	if watchNamespacesRaw == "" {
		return false, defaultNamespacesToWatch, nil
	}

	// Special label => watch all namespaces
	if watchNamespacesRaw == allNamespacesLabel {
		return true, []string{}, nil
	}

	rawNamespaces := strings.Split(watchNamespacesRaw, ",")
	namespaces = make([]string, 0, len(rawNamespaces))
	for _, rawNamespace := range rawNamespaces {
		namespace := strings.TrimSpace(rawNamespace)
		if namespace == "" {
			continue
		}
		errors := validation.IsValidLabelValue(namespace)
		if len(errors) != 0 {
			// Not a valid namespace name
			return false, []string{}, fmt.Errorf(errors[0])
		}
		namespaces = append(namespaces, namespace)
	}
	namespaces = removeDuplicates(namespaces)
	if len(namespaces) == 0 {
		return false, namespaces, fmt.Errorf("no namespaces specified")
	}

	return false, namespaces, nil
}

func configureLogging() {
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stdout})
}

func setGlobalSentryTags() {
	for _, e := range os.Environ() {
		pair := strings.SplitN(e, "=", 2)
		if len(pair) != 2 {
			continue
		}
		key, value := strings.TrimSpace(pair[0]), strings.TrimSpace(pair[1])
		tagPrefix := "SENTRY_K8S_GLOBAL_TAG_"
		if strings.HasPrefix(key, tagPrefix) {
			tagKey := strings.TrimPrefix(key, tagPrefix)
			log.Info().Msgf("Global tag detected: %s=%s", tagKey, value)
			sentry.CurrentHub().Scope().SetTag(tagKey, value)
		}
	}
}

func main() {
	configureLogging()
	initSentrySDK()
	defer sentry.Flush(time.Second)

	config, err := getClusterConfig()
	if err != nil {
		log.Fatal().Msgf("Config init error: %s", err)
	}

	setKubernetesSentryContext(config)
	setGlobalSentryTags()

	watchAllNamespaces, namespaces, err := getNamespacesToWatch()
	if err != nil {
		log.Fatal().Msgf("Cannot parse namespaces to watch: %s", err)
	}

	if watchAllNamespaces {
		namespaces = []string{v1.NamespaceAll}
	}

	for _, namespace := range namespaces {
		go watchEventsInNamespaceForever(config, namespace)
	}

	// Sleep forever
	select {}
}
