package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/getsentry/sentry-go"
	"github.com/rs/zerolog"
	globalLogger "github.com/rs/zerolog/log"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	toolsWatch "k8s.io/client-go/tools/watch"
)

func getObjectNameTag(object *v1.ObjectReference) string {
	if object.Kind == "" {
		return "object_name"
	} else {
		return fmt.Sprintf("%s_name", strings.ToLower(object.Kind))
	}
}

func processKubernetesEvent(ctx context.Context, eventObject *v1.Event, scope *sentry.Scope) *sentry.Event {
	logger := zerolog.Ctx(ctx)

	logger.Debug().Msgf("EventObject: %#v", eventObject)
	logger.Debug().Msgf("Event type: %#v", eventObject.Type)

	originalEvent := eventObject.DeepCopy()
	eventObject = eventObject.DeepCopy()

	involvedObject := eventObject.InvolvedObject

	setTagIfNotEmpty(scope, "event_type", eventObject.Type)
	setTagIfNotEmpty(scope, "reason", eventObject.Reason)
	setTagIfNotEmpty(scope, "kind", involvedObject.Kind)
	setTagIfNotEmpty(scope, "object_uid", string(involvedObject.UID))
	setTagIfNotEmpty(scope, "namespace", involvedObject.Namespace)

	name_tag := getObjectNameTag(&involvedObject)
	setTagIfNotEmpty(scope, name_tag, involvedObject.Name)

	if source, err := prettyJson(eventObject.Source); err == nil {
		scope.SetExtra("Event Source", source)
	}
	setTagIfNotEmpty(scope, "event_source_component", eventObject.Source.Component)
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

	// The entire (remaining) event
	if kubeEvent, err := prettyJson(eventObject); err == nil {
		scope.SetExtra("~ Misc Event Fields", kubeEvent)
	}

	sentryEvent := buildSentryEvent(ctx, originalEvent, scope)
	return sentryEvent
}

func handleWatchEvent(ctx context.Context, event *watch.Event, cutoffTime metav1.Time) {
	logger := zerolog.Ctx(ctx)

	eventObjectRaw := event.Object
	// Watch event type: Added, Delete, Bookmark...
	if (event.Type != watch.Added) && (event.Type != watch.Modified) {
		logger.Debug().Msgf("Skipping a watch event of type %s", event.Type)
		return
	}

	objectKind := eventObjectRaw.GetObjectKind()
	eventObject, ok := eventObjectRaw.(*v1.Event)
	if !ok {
		logger.Warn().Msgf("Skipping an event of kind '%v' because it cannot be casted", objectKind)
		return
	}

	namespace := eventObject.Namespace
	if namespace != "" {
		ctx, logger = getLoggerWithTag(ctx, "namespace", namespace)
	}

	// Get event timestamp
	eventTs := eventObject.LastTimestamp
	if eventTs.IsZero() {
		eventTs = metav1.Time(eventObject.EventTime)
	}

	if !cutoffTime.IsZero() && !eventTs.IsZero() && eventTs.Before(&cutoffTime) {
		logger.Debug().Msgf("Ignoring an event because it is too old")
		return
	}

	if eventObject.Type == v1.EventTypeNormal {
		logger.Debug().Msgf("Skipping an event of type %s", eventObject.Type)
		addEventToBuffer(eventObject)
		return
	}

	hub := sentry.GetHubFromContext(ctx)
	if hub == nil {
		logger.Error().Msgf("Cannot get Sentry hub from context")
		return
	}
	hub.WithScope(func(scope *sentry.Scope) {
		sentryEvent := processKubernetesEvent(ctx, eventObject, scope)
		if sentryEvent != nil {
			hub.CaptureEvent(sentryEvent)
		}
	})

	addEventToBuffer(eventObject)
}

func watchEventsInNamespace(ctx context.Context, namespace string, watchSince time.Time) (err error) {
	logger := zerolog.Ctx(ctx)

	clientset, err := getClientsetFromContext(ctx)
	if err != nil {
		return err
	}

	watchFunc := func(options metav1.ListOptions) (watch.Interface, error) {
		opts := metav1.ListOptions{
			Watch: true,
		}
		return clientset.CoreV1().Events(namespace).Watch(ctx, opts)
	}
	logger.Debug().Msg("Getting the event watcher...")
	retryWatcher, err := toolsWatch.NewRetryWatcher("1", &cache.ListWatch{WatchFunc: watchFunc})
	if err != nil {
		return err
	}

	watchCh := retryWatcher.ResultChan()
	defer retryWatcher.Stop()

	watchSinceWrapped := metav1.Time{Time: watchSince}

	logger.Debug().Msg("Reading from the event channel...")
	for event := range watchCh {
		handleWatchEvent(ctx, &event, watchSinceWrapped)
	}

	return nil
}

func watchEventsInNamespaceForever(ctx context.Context, config *rest.Config, namespace string) error {
	localHub := sentry.CurrentHub().Clone()
	ctx = sentry.SetHubOnContext(ctx, localHub)

	where := fmt.Sprintf("in namespace '%s'", namespace)
	namespaceTag := namespace
	if namespace == v1.NamespaceAll {
		where = "in all namespaces"
		namespaceTag = "__all__"
	}

	// Attach the "namespace" tag to logger
	ctx, logger := getLoggerWithTag(ctx, "namespace", namespaceTag)

	watchFromBeginning := isTruthy(os.Getenv("SENTRY_K8S_WATCH_HISTORICAL"))
	var watchSince time.Time
	if watchFromBeginning {
		watchSince = time.Time{}
		logger.Info().Msgf("Watching all available events (no starting timestamp)")
	} else {
		watchSince = time.Now()
		logger.Info().Msgf("Watching events starting from: %s", watchSince.Format("Mon, 02 Jan 2006 15:04:05 -0700"))
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return err
	}

	ctx = setClientsetOnContext(ctx, clientset)

	if isTruthy(os.Getenv("SENTRY_K8S_MONITOR_CRONJOBS")) {
		logger.Info().Msgf("Enabling CronJob monitoring...")
		go startCronJobInformer(ctx, namespace)
	} else {
		logger.Info().Msgf("CronJob monitoring is disabled.")
	}

	for {
		if err := watchEventsInNamespace(ctx, namespace, watchSince); err != nil {
			logger.Error().Msgf("Error while watching events %s: %s", where, err)
		}
		watchSince = time.Now()
		time.Sleep(time.Second * 1)
	}
}

func configureLogging() {
	levelMap := map[string]zerolog.Level{
		"trace":    zerolog.TraceLevel,
		"debug":    zerolog.DebugLevel,
		"info":     zerolog.InfoLevel,
		"warn":     zerolog.WarnLevel,
		"error":    zerolog.ErrorLevel,
		"fatal":    zerolog.FatalLevel,
		"panic":    zerolog.PanicLevel,
		"disabled": zerolog.Disabled,
	}
	logLevelRaw := strings.ToLower(os.Getenv("SENTRY_K8S_LOG_LEVEL"))

	var logLevel zerolog.Level
	logLevel, ok := levelMap[logLevelRaw]
	if !ok {
		logLevel = zerolog.InfoLevel
	}

	zerolog.SetGlobalLevel(logLevel)
	globalLogger.Logger = globalLogger.Output(zerolog.ConsoleWriter{Out: os.Stdout})
}

func main() {
	configureLogging()
	initSentrySDK()
	defer sentry.Flush(time.Second)
	checkCommonEnhancerPatterns()

	config, err := getClusterConfig()
	if err != nil {
		globalLogger.Fatal().Msgf("Config init error: %s", err)
	}

	setKubernetesSentryContext(config)
	setGlobalSentryTags()
	runIntegrations()

	watchAllNamespaces, namespaces, err := getNamespacesToWatch()
	if err != nil {
		globalLogger.Fatal().Msgf("Cannot parse namespaces to watch: %s", err)
	}

	if watchAllNamespaces {
		namespaces = []string{v1.NamespaceAll}
	}

	ctx := globalLogger.Logger.WithContext(context.Background())
	for _, namespace := range namespaces {
		go watchEventsInNamespaceForever(ctx, config, namespace)
	}

	// Sleep forever
	select {}
}
