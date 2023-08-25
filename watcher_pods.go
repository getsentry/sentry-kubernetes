package main

import (
	"context"
	"fmt"
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

const podsWatcherName = "pods"

func handlePodTerminationEvent(ctx context.Context, containerStatus *v1.ContainerStatus, pod *v1.Pod, scope *sentry.Scope) *sentry.Event {
	logger := zerolog.Ctx(ctx)

	state := containerStatus.State.Terminated

	logger.Trace().Msgf("Container state: %#v", state)
	if state.ExitCode == 0 {
		// Nothing to do
		return nil
	}

	setTagIfNotEmpty(scope, "reason", state.Reason)
	setTagIfNotEmpty(scope, "kind", pod.Kind)
	setTagIfNotEmpty(scope, "object_uid", string(pod.UID))
	setTagIfNotEmpty(scope, "namespace", pod.Namespace)
	setTagIfNotEmpty(scope, "pod_name", pod.Name)
	setTagIfNotEmpty(scope, "container_name", containerStatus.Name)

	// FIXME: there's no proper controller we can extract here, so inventing a new one
	setTagIfNotEmpty(scope, "event_source_component", "x-pod-controller")

	if containerStatusJson, err := prettyJson(containerStatus); err == nil {
		scope.SetExtra("Container Status", containerStatusJson)
	}

	message := state.Message
	if message == "" {
		message = fmt.Sprintf(
			"%s: container %q",
			state.Reason,
			containerStatus.Name,
		)
	}

	sentryEvent := buildSentryEventFromPodTerminationEvent(ctx, pod, message, scope)
	return sentryEvent
}

func buildSentryEventFromPodTerminationEvent(ctx context.Context, pod *v1.Pod, message string, scope *sentry.Scope) *sentry.Event {
	sentryEvent := &sentry.Event{Message: message, Level: sentry.LevelError}
	objectRef := &v1.ObjectReference{
		Kind:      "Pod",
		Name:      pod.Name,
		Namespace: pod.Namespace,
	}
	runEnhancers(ctx, objectRef, pod, scope, sentryEvent)
	return sentryEvent
}

func handlePodWatchEvent(ctx context.Context, event *watch.Event) {
	logger := zerolog.Ctx(ctx)

	eventObjectRaw := event.Object
	// Watch event type: Added, Delete, Bookmark...
	if event.Type != watch.Modified {
		logger.Debug().Msgf("Skipping a pod watch event of type %s", event.Type)
		return
	}

	objectKind := eventObjectRaw.GetObjectKind()
	podObject, ok := eventObjectRaw.(*v1.Pod)
	if !ok {
		logger.Warn().Msgf("Skipping an event of kind '%v' because it cannot be casted", objectKind)
		return
	}

	logger.Trace().Msgf("Pod Object received: %#v", podObject)

	ctx, logger = getLoggerWithTag(ctx, "namespace", podObject.GetNamespace())

	if podObject.DeletionTimestamp != nil {
		logger.Debug().Msgf("Pod is about to be deleted; ignoring state modifications")
		return
	}

	hub := sentry.GetHubFromContext(ctx)
	if hub == nil {
		logger.Error().Msgf("Cannot get Sentry hub from context")
		return
	}

	containerStatuses := podObject.Status.ContainerStatuses
	logger.Trace().Msgf("Container statuses: %#v\n", containerStatuses)
	for _, status := range containerStatuses {
		state := status.State
		if state.Terminated == nil {
			// Ignore non-Terminated statuses
			continue
		}

		hub.WithScope(func(scope *sentry.Scope) {
			setWatcherTag(scope, podsWatcherName)
			sentryEvent := handlePodTerminationEvent(ctx, &status, podObject, scope)
			if sentryEvent != nil {
				hub.CaptureEvent(sentryEvent)
			}
		})
	}
}

// TODO: dedupe with events
func watchPodsInNamespace(ctx context.Context, namespace string) (err error) {
	logger := zerolog.Ctx(ctx)

	clientset, err := getClientsetFromContext(ctx)
	if err != nil {
		return err
	}

	watchFunc := func(options metav1.ListOptions) (watch.Interface, error) {
		opts := metav1.ListOptions{
			Watch: true,
		}
		return clientset.CoreV1().Pods(namespace).Watch(ctx, opts)
	}
	logger.Debug().Msg("Getting the pod watcher")
	retryWatcher, err := toolsWatch.NewRetryWatcher("1", &cache.ListWatch{WatchFunc: watchFunc})
	if err != nil {
		return err
	}

	watchCh := retryWatcher.ResultChan()
	defer retryWatcher.Stop()

	logger.Debug().Msg("Reading from the event channel (pods)")
	for event := range watchCh {
		handlePodWatchEvent(ctx, &event)
	}

	return nil
}

// TODO: dedupe with events
func watchPodsInNamespaceForever(ctx context.Context, config *rest.Config, namespace string) error {
	localHub := sentry.CurrentHub().Clone()
	ctx = sentry.SetHubOnContext(ctx, localHub)

	where := fmt.Sprintf("in namespace '%s'", namespace)
	namespaceTag := namespace
	if namespace == v1.NamespaceAll {
		where = "in all namespaces"
		namespaceTag = "__all__"
	}

	// Attach the "namespace" tag to logger
	ctx, logger := getLoggerWithTags(
		ctx,
		map[string]string{
			"namespace": namespaceTag,
			"watcher":   podsWatcherName,
		},
	)

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return err
	}

	ctx = setClientsetOnContext(ctx, clientset)

	for {
		if err := watchPodsInNamespace(ctx, namespace); err != nil {
			logger.Error().Msgf("Error while watching pods %s: %s", where, err)
		}
		// Note: some events might be lost when we're sleeping here
		time.Sleep(time.Second * 1)
	}
}

func startPodWatchers(ctx context.Context, config *rest.Config, namespaces []string) {
	for _, namespace := range namespaces {

		go watchPodsInNamespaceForever(ctx, config, namespace)

	}
}
