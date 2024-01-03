package main

import (
	"context"
	"fmt"

	"github.com/getsentry/sentry-go"
	"github.com/rs/zerolog"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const breadcrumbLimit = 20

func runPodEnhancer(ctx context.Context, podMeta *v1.ObjectReference, cachedObject interface{}, scope *sentry.Scope, sentryEvent *sentry.Event) error {
	logger := zerolog.Ctx(ctx)

	logger.Debug().Msgf("Running the pod enhancer")

	clientset, err := getClientsetFromContext(ctx)
	if err != nil {
		return err
	}

	namespace := podMeta.Namespace
	podName := podMeta.Name
	opts := metav1.GetOptions{}

	cachedPod, _ := cachedObject.(*v1.Pod)
	var pod *v1.Pod
	if cachedPod == nil {
		logger.Debug().Msgf("Fetching pod data")
		// FIXME: this can probably be cached if we use NewSharedInformerFactory
		pod, err = clientset.CoreV1().Pods(namespace).Get(context.Background(), podName, opts)
		if err != nil {
			return err
		}
	} else {
		logger.Debug().Msgf("Reusing the available pod object")
		pod = cachedPod
	}

	if value, exists := pod.Labels["app.kubernetes.io/name"]; exists {
		podName = value
	}

	// Clean-up the object
	pod.ManagedFields = []metav1.ManagedFieldsEntry{}

	nodeName := pod.Spec.NodeName
	setTagIfNotEmpty(scope, "node_name", nodeName)

	metadataJson, err := prettyJson(pod.ObjectMeta)
	if err == nil {
		scope.SetContext("Pod", sentry.Context{
			"Metadata": metadataJson,
		})
	}

	// The data will be mostly duplicated in "Pod Metadata"
	scope.RemoveExtra("Involved Object")

	// Add related events as breadcrumbs
	podEvents := filterEventsFromBuffer(namespace, "Pod", podName)
	for _, podEvent := range podEvents {
		breadcrumbLevel := sentry.LevelInfo
		if podEvent.Type == v1.EventTypeWarning {
			breadcrumbLevel = sentry.LevelWarning
		}

		scope.AddBreadcrumb(&sentry.Breadcrumb{
			Message:   podEvent.Message,
			Level:     breadcrumbLevel,
			Timestamp: podEvent.LastTimestamp.Time,
		}, breadcrumbLimit)
	}

	message := sentryEvent.Message
	sentryEvent.Message = fmt.Sprintf("%s: %s", podName, sentryEvent.Message)

	logger.Trace().Msgf("Current fingerprint: %v", sentryEvent.Fingerprint)

	// Adjust fingerprint.
	// If there's already a non-empty fingerprint set, we assume that it was set by
	// another enhancer, so we don't touch it.
	if len(sentryEvent.Fingerprint) == 0 {
		sentryEvent.Fingerprint = []string{message}
	}

	// Using finger print to group events together
	// The pod is not owned by a higher resource
	if len(pod.OwnerReferences) == 0 {
		// Standalone pod => most probably it has a unique name
		sentryEvent.Fingerprint = append(sentryEvent.Fingerprint, podName)
		// The pod is owned by a higher resource
	} else {
		// Check if pod is part of cronJob (as grandchild workload resource)

		// check if a cronJob
		var ok bool
		if pod.OwnerReferences[0].Kind == "Job" {
			ok, err = runCronsDataHandler(ctx, scope, pod, sentryEvent)
			if err != nil {
				return err
			}
		}

		// The job is not owned by a cronJob
		if !ok {
			owner := pod.OwnerReferences[0]
			sentryEvent.Fingerprint = append(sentryEvent.Fingerprint, owner.Kind, owner.Name)
		}
	}

	logger.Trace().Msgf("Fingerprint after adjustment: %v", sentryEvent.Fingerprint)

	addPodLogLinkToGKEContext(ctx, scope, podName, namespace)

	return nil
}
