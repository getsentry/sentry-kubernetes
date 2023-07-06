package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/getsentry/sentry-go"
	"github.com/rs/zerolog"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const breadcrumbLimit = 20

func runPodEnhancer(ctx context.Context, event *v1.Event, scope *sentry.Scope, sentryEvent *sentry.Event) error {
	logger := zerolog.Ctx(ctx)

	clientset, err := getClientsetFromContext(ctx)
	if err != nil {
		return err
	}

	namespace := event.Namespace
	podName := event.InvolvedObject.Name
	opts := metav1.GetOptions{}

	logger.Debug().Msgf("Fetching pod data")
	// FIXME: this can probably be cached if we use NewSharedInformerFactory
	pod, err := clientset.CoreV1().Pods(namespace).Get(context.Background(), podName, opts)
	if err != nil {
		return err
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

	// Adjust message
	var message string
	if sentryEvent.Message == "" {
		message = event.Message
	} else {
		// Message may be already set by previous enhancers
		message = sentryEvent.Message
	}
	if !strings.Contains(message, podName) {
		sentryEvent.Message = fmt.Sprintf("%s: %s", podName, message)
	}

	// Adjust fingerprint
	if len(pod.OwnerReferences) > 0 {
		// If the pod is controlled by something (e.g. a replicaset), group all issues
		// for all controlled pod together.
		owner := pod.OwnerReferences[0]
		sentryEvent.Fingerprint = []string{event.Message, owner.Name}
	} else {
		sentryEvent.Fingerprint = []string{event.Message, podName}
	}

	return nil
}
