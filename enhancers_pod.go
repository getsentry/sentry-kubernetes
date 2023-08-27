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

	// Clean-up the object
	pod.ManagedFields = []metav1.ManagedFieldsEntry{}

	nodeName := pod.Spec.NodeName
	setTagIfNotEmpty(scope, "node_name", nodeName)

	metadataJson, err := prettyJson(pod.ObjectMeta)
	if err == nil {
		scope.SetExtra("Pod Metadata", metadataJson)
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

	// Adjust fingerprint
	if len(sentryEvent.Fingerprint) == 0 {
		sentryEvent.Fingerprint = []string{message}
	}

	if len(pod.OwnerReferences) > 0 {
		// If the pod is controlled by something (e.g. a replicaset), group all issues
		// for all controlled pod together.
		owner := pod.OwnerReferences[0]
		sentryEvent.Fingerprint = append(sentryEvent.Fingerprint, owner.Kind, owner.Name)
	} else {
		// Standalone pod => most probably it has a unique name
		sentryEvent.Fingerprint = append(sentryEvent.Fingerprint, podName)
	}

	addPodLogLinkToGKEContext(ctx, scope, podName, namespace)

	return nil
}
