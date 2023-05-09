package main

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/getsentry/sentry-go"
	"github.com/rs/zerolog/log"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

func prettyJson(v any) ([]byte, error) {
	return json.MarshalIndent(v, "", "  ")
}

func handleEvent(eventObject *v1.Event) {
	fmt.Printf("EventObject: %#v\n", eventObject)
	fmt.Printf("Event type: %#v\n", eventObject.Type)
	fmt.Println()

	involvedObject := eventObject.InvolvedObject

	sentry.WithScope(func(scope *sentry.Scope) {
		// TODO: use SetTags?
		scope.SetTag("eventType", eventObject.Type)
		scope.SetTag("reason", eventObject.Reason)
		scope.SetTag("namespace", involvedObject.Namespace)
		scope.SetTag("kind", involvedObject.Kind)
		scope.SetTag("object_UID", string(involvedObject.UID))

		name_tag := "object_name"
		if involvedObject.Kind == "Pod" {
			name_tag = "pod_name"
		}
		scope.SetTag(name_tag, involvedObject.Name)

		if source, err := prettyJson(eventObject.Source); err == nil {
			scope.SetExtra("source", string(source))
		}

		if involvedObject, err := prettyJson(eventObject.InvolvedObject); err == nil {
			scope.SetExtra("involvedObject", string(involvedObject))
		}

		if metadata, err := prettyJson(eventObject.ObjectMeta); err == nil {
			scope.SetExtra("metadata", string(metadata))
		}

		// The entire event
		if kubeEvent, err := prettyJson(eventObject); err == nil {
			scope.SetExtra("kubeEvent", string(kubeEvent))
		}

		sentryEvent := &sentry.Event{Message: eventObject.Message, Level: sentry.LevelError}
		sentry.CaptureEvent(sentryEvent)
	})

}

func watchEventsInNamespace(config *rest.Config, namespace string) (err error) {
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return err
	}
	opts := metav1.ListOptions{
		// FieldSelector: "involvedObject.kind=Pod",
		Watch: true,
	}
	log.Debug().Msg("Getting the event watcher...")

	ctx, cancelFunc := context.WithCancel(context.Background())
	defer cancelFunc()

	// Set "" to get events from all namespaces.
	// TODO: how to watch only for specific ones?
	// FIXME: Watch() currently returns also all recent events.
	// Should we ignore events that happened in the past?
	watcher, err := clientset.CoreV1().Events(namespace).Watch(ctx, opts)
	if err != nil {
		return err
	}

	watchCh := watcher.ResultChan()
	defer watcher.Stop()

	log.Debug().Msg("Reading from the event channel...")
	for event := range watchCh {
		eventObjectRaw := event.Object
		// Watch event type: Added, Delete, Bookmark...
		watchEventType := string(event.Type)

		objectKind := eventObjectRaw.GetObjectKind()

		eventObject, ok := eventObjectRaw.(*v1.Event)
		if !ok {
			log.Warn().Msgf("Skipping an event of eventType '%s', kind '%v'", watchEventType, objectKind)
			continue
		}
		// log.Info().Str("type", eventType).Msgf("%#v", eventObject)

		if eventObject.Type == v1.EventTypeNormal {
			log.Debug().Msgf("Skipping an event of type Normal")
			continue
		}
		handleEvent(eventObject)
	}

	return nil
}

func main() {
	initSentrySDK()
	defer sentry.Flush(time.Second)

	// FIXME: make this configurable
	useInClusterConfig := false
	// FIXME: make this configurable
	namespace := "default"

	config, err := getClusterConfig(useInClusterConfig)
	if err != nil {
		log.Fatal().Msgf("Config init error: %s", err)
	}

	err = watchEventsInNamespace(config, namespace)
	if err != nil {
		log.Fatal().Msgf("Watch error: %s", err)
	}
}
