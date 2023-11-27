package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/getsentry/sentry-go"
	"github.com/rs/zerolog"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	toolsWatch "k8s.io/client-go/tools/watch"
)

const eventsWatcherName = "events"

func handleGeneralEvent(ctx context.Context, eventObject *v1.Event, scope *sentry.Scope) *sentry.Event {
	logger := zerolog.Ctx(ctx)

	logger.Debug().Msgf("EventObject: %#v", eventObject)

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
		scope.SetContext("Event", sentry.Context{
			"Source": source,
		})
	}
	setTagIfNotEmpty(scope, "event_source_component", eventObject.Source.Component)
	eventObject.Source = v1.EventSource{}

	if involvedObject, err := prettyJson(eventObject.InvolvedObject); err == nil {
		scope.SetContext("InvolvedObject", sentry.Context{
			"Object": involvedObject,
		})
	}
	eventObject.InvolvedObject = v1.ObjectReference{}

	// clean-up the event a bit
	eventObject.ObjectMeta.ManagedFields = []metav1.ManagedFieldsEntry{}
	if metadata, err := prettyJson(eventObject.ObjectMeta); err == nil {
		scope.SetContext("Event", sentry.Context{
			"Metadata": metadata,
		})
	}
	eventObject.ObjectMeta = metav1.ObjectMeta{}

	// The entire (remaining) event
	if kubeEvent, err := prettyJson(eventObject); err == nil {
		scope.SetContext("Misc", sentry.Context{
			"Kube": kubeEvent,
		})
	}

	sentryEvent := buildSentryEventFromGeneralEvent(ctx, originalEvent, scope)
	return sentryEvent
}

func buildSentryEventFromGeneralEvent(ctx context.Context, event *v1.Event, scope *sentry.Scope) *sentry.Event {
	sentryEvent := &sentry.Event{Message: event.Message, Level: sentry.LevelError}
	objectRef := &v1.ObjectReference{
		Kind:      event.InvolvedObject.Kind,
		Name:      event.InvolvedObject.Name,
		Namespace: event.InvolvedObject.Namespace,
	}
	runEnhancers(ctx, objectRef, nil, scope, sentryEvent)
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

	defer addEventToBuffer(eventObject)

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
		return
	}

	if isFilteredByReason(eventObject) {
		logger.Debug().Msgf("Skipping an event with reason: %q", eventObject.Reason)
		return
	}

	if isFilteredByEventSource(eventObject) {
		logger.Debug().Msgf("Skipping an event with event source: %q", eventObject.Source.Component)
		return
	}

	hub := sentry.GetHubFromContext(ctx)
	if hub == nil {
		logger.Error().Msgf("Cannot get Sentry hub from context")
		return
	}
	hub.WithScope(func(scope *sentry.Scope) {
		setWatcherTag(scope, eventsWatcherName)
		sentryEvent := handleGeneralEvent(ctx, eventObject, scope)
		if sentryEvent != nil {
			hub.CaptureEvent(sentryEvent)
		}
	})
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
	logger.Debug().Msg("Getting the event watcher")
	retryWatcher, err := toolsWatch.NewRetryWatcher("1", &cache.ListWatch{WatchFunc: watchFunc})
	if err != nil {
		return err
	}

	watchCh := retryWatcher.ResultChan()
	defer retryWatcher.Stop()

	watchSinceWrapped := metav1.Time{Time: watchSince}

	logger.Debug().Msg("Reading from the event channel (events)")
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

	// Attach the "namespace" and "watcher" tags to logger
	ctx, logger := getLoggerWithTags(
		ctx,
		map[string]string{
			"namespace": namespaceTag,
			"watcher":   eventsWatcherName,
		},
	)

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

	for {
		if err := watchEventsInNamespace(ctx, namespace, watchSince); err != nil {
			logger.Error().Msgf("Error while watching events %s: %s", where, err)
		}
		watchSince = time.Now()
		time.Sleep(time.Second * 1)
	}
}

func startEventWatchers(ctx context.Context, config *rest.Config, namespaces []string) {
	for _, namespace := range namespaces {
		go watchEventsInNamespaceForever(ctx, config, namespace)
	}
}
