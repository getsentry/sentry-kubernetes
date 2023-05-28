package main

import (
	"context"
	"fmt"

	"github.com/getsentry/sentry-go"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func runPodEnhancer(ctx context.Context, event *v1.Event, scope *sentry.Scope) error {
	clientset, err := getClientsetFromContext(ctx)
	if err != nil {
		return err
	}

	namespace := event.Namespace
	podName := event.InvolvedObject.Name
	opts := metav1.GetOptions{}
	pod, err := clientset.CoreV1().Pods(namespace).Get(context.Background(), podName, opts)
	if err != nil {
		return err
	}

	// Clean-up the object
	pod.ManagedFields = []metav1.ManagedFieldsEntry{}

	nodeName := pod.Spec.NodeName
	if nodeName != "" {
		scope.SetTag("node_name", nodeName)
	}

	metadataJson, err := prettyJson(pod.ObjectMeta)
	if err == nil {
		scope.SetExtra("Event Metadata", metadataJson)
	}

	return nil
}

func runEnhancers(ctx context.Context, event *v1.Event, scope *sentry.Scope) {
	fmt.Println("Running enhancers...")
	switch event.InvolvedObject.Kind {
	case "Pod":
		runPodEnhancer(ctx, event, scope)
	}
}
