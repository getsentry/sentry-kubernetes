package main

import (
	"context"
	"errors"
	"fmt"

	"github.com/getsentry/sentry-go"
	"github.com/rs/zerolog"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const breadcrumbLimit = 20

func runPodEnhancer(ctx context.Context, pod *v1.Pod, scope *sentry.Scope, sentryEvent *sentry.Event) error {
	logger := zerolog.Ctx(ctx)
	logger.Debug().Msgf("Running the pod enhancer")
	logger.Debug().Msgf("Fetching pod data")

	// This event data will be mostly duplicated in "Pod Metadata"
	scope.RemoveExtra("Involved Object")

	// Add related events as breadcrumbs
	podEvents := filterEventsFromBuffer(pod.Namespace, "Pod", pod.Name)
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

	// Enhance message with pod name
	message := sentryEvent.Message
	sentryEvent.Message = fmt.Sprintf("%s: %s", pod.Name, sentryEvent.Message)

	logger.Trace().Msgf("Current fingerprint: %v", sentryEvent.Fingerprint)

	// Add node name as tag
	nodeName := pod.Spec.NodeName
	setTagIfNotEmpty(scope, "node_name", nodeName)

	// Clean-up the object
	pod.ManagedFields = []metav1.ManagedFieldsEntry{}
	metadataJson, err := prettyJson(pod.ObjectMeta)
	if err == nil {
		scope.SetContext("Pod", sentry.Context{
			"Metadata": metadataJson,
		})
	}

	// Adjust fingerprint.
	// If there's already a non-empty fingerprint set, we assume that it was set by
	// another enhancer, so we don't touch it.
	if len(sentryEvent.Fingerprint) == 0 {
		sentryEvent.Fingerprint = []string{message}
	}

	// Find the root owners and their kinds
	rootOwners, err := findRootOwners(ctx, &KindObjectPair{
		kind:   "Pod",
		object: pod,
	})
	if err != nil {
		return err
	}

	// Call specific enhancers for all root owners
	// (there most likely is just one root owner)
	for _, rootOwner := range rootOwners {
		switch rootOwner.kind {
		case "Pod":
			sentryEvent.Fingerprint = append(sentryEvent.Fingerprint, rootOwner.object.GetName())
		case "ReplicaSet":
			ReplicaSetPodEnhancer(ctx, scope, rootOwner.object, sentryEvent)
		case "Deployment":
			deploymentPodEnhancer(ctx, scope, rootOwner.object, sentryEvent)
		case "Job":
			jobPodEnhancer(ctx, scope, rootOwner.object, sentryEvent)
		case "CronJob":
			cronjobPodEnhancer(ctx, scope, rootOwner.object, sentryEvent)
		default:
			sentryEvent.Fingerprint = append(sentryEvent.Fingerprint, rootOwner.object.GetName())
		}
	}

	logger.Trace().Msgf("Fingerprint after adjustment: %v", sentryEvent.Fingerprint)

	addPodLogLinkToGKEContext(ctx, scope, pod.Name, pod.Namespace)

	return nil
}

func findRootOwners(ctx context.Context, kindObjPair *KindObjectPair) ([]KindObjectPair, error) {

	parents := kindObjPair.object.GetOwnerReferences()
	// the owners slice to be returned
	rootOwners := []KindObjectPair{}

	// base case: the object has no parents
	if len(parents) == 0 {
		rootOwners = append(rootOwners, *kindObjPair)
		return rootOwners, nil
	}

	// recursive case: the object has parents to explore
	for _, parent := range parents {
		parentObj, ok := findObject(ctx, parent.Kind, kindObjPair.object.GetNamespace(), parent.Name)
		if !ok {
			return nil, errors.New("error attempting to find root owneres")
		}
		partialOwners, err := findRootOwners(ctx, &KindObjectPair{
			kind:   parent.Kind,
			object: parentObj,
		})
		if err != nil {
			return nil, err
		}
		if partialOwners != nil {
			rootOwners = append(rootOwners, partialOwners...)
		}
	}
	return rootOwners, nil
}

type KindObjectPair struct {
	kind   string
	object metav1.Object
}

// TODO: add additional enhancer features more specific to workload resource kind (e.g. monitor slug for cronjob)
func jobPodEnhancer(ctx context.Context, scope *sentry.Scope, object metav1.Object, sentryEvent *sentry.Event) error {

	jobObj, ok := object.(*batchv1.Job)
	if !ok {
		return errors.New("failed to cast object to Job object")
	}

	// Add the cronjob to the fingerprint
	sentryEvent.Fingerprint = append(sentryEvent.Fingerprint, "job", object.GetName())

	// Add the cronjob to the tag
	setTagIfNotEmpty(scope, "job_name", object.GetName())
	jobObj.ManagedFields = []metav1.ManagedFieldsEntry{}
	metadataJson, err := prettyJson(jobObj.ObjectMeta)
	if err == nil {
		scope.SetContext("job", sentry.Context{
			"Metadata": metadataJson,
		})
	}

	// Add breadcrumb with cronjob timestamps
	scope.AddBreadcrumb(&sentry.Breadcrumb{
		Message:   fmt.Sprintf("Created job %s", object.GetName()),
		Level:     sentry.LevelInfo,
		Timestamp: object.GetCreationTimestamp().Time,
	}, breadcrumbLimit)

	return nil
}

func cronjobPodEnhancer(ctx context.Context, scope *sentry.Scope, object metav1.Object, sentryEvent *sentry.Event) error {

	cronjobObj, ok := object.(*batchv1.CronJob)
	if !ok {
		return errors.New("failed to cast object to CronJob object")
	}

	// Set the context for corresponding slug monitor
	scope.SetContext("Monitor", sentry.Context{
		"Slug": object.GetName(),
	})

	// Add the cronjob to the fingerprint
	sentryEvent.Fingerprint = append(sentryEvent.Fingerprint, "cronjob", object.GetName())

	// Add the cronjob to the tag
	setTagIfNotEmpty(scope, "cronjob_name", object.GetName())
	cronjobObj.ManagedFields = []metav1.ManagedFieldsEntry{}
	metadataJson, err := prettyJson(cronjobObj.ObjectMeta)
	if err == nil {
		scope.SetContext("cronjob", sentry.Context{
			"Metadata": metadataJson,
		})
	}

	// Add breadcrumb with cronjob timestamps
	scope.AddBreadcrumb(&sentry.Breadcrumb{
		Message:   fmt.Sprintf("Created cronjob %s", object.GetName()),
		Level:     sentry.LevelInfo,
		Timestamp: object.GetCreationTimestamp().Time,
	}, breadcrumbLimit)

	return nil
}

func ReplicaSetPodEnhancer(ctx context.Context, scope *sentry.Scope, object metav1.Object, sentryEvent *sentry.Event) error {

	replicasetObj, ok := object.(*appsv1.ReplicaSet)
	if !ok {
		return errors.New("failed to cast object to ReplicaSet object")
	}

	// Add the cronjob to the fingerprint
	sentryEvent.Fingerprint = append(sentryEvent.Fingerprint, "replicaset", object.GetName())

	// Add the cronjob to the tag
	setTagIfNotEmpty(scope, "replicaset_name", object.GetName())
	replicasetObj.ManagedFields = []metav1.ManagedFieldsEntry{}
	metadataJson, err := prettyJson(replicasetObj.ObjectMeta)
	if err == nil {
		scope.SetContext("replicaset", sentry.Context{
			"Metadata": metadataJson,
		})
	}

	// Add breadcrumb with cronjob timestamps
	scope.AddBreadcrumb(&sentry.Breadcrumb{
		Message:   fmt.Sprintf("Created replicaset %s", object.GetName()),
		Level:     sentry.LevelInfo,
		Timestamp: object.GetCreationTimestamp().Time,
	}, breadcrumbLimit)

	return nil
}

func deploymentPodEnhancer(ctx context.Context, scope *sentry.Scope, object metav1.Object, sentryEvent *sentry.Event) error {

	deploymentObj, ok := object.(*appsv1.Deployment)
	if !ok {
		return errors.New("failed to cast object to Deployment object")
	}
	// Add the cronjob to the fingerprint
	sentryEvent.Fingerprint = append(sentryEvent.Fingerprint, "deployment", object.GetName())

	// Add the cronjob to the tag
	setTagIfNotEmpty(scope, "deployment_name", object.GetName())
	deploymentObj.ManagedFields = []metav1.ManagedFieldsEntry{}
	metadataJson, err := prettyJson(deploymentObj.ObjectMeta)
	if err == nil {
		scope.SetContext("deployment", sentry.Context{
			"Metadata": metadataJson,
		})
	}

	// Add breadcrumb with cronjob timestamps
	scope.AddBreadcrumb(&sentry.Breadcrumb{
		Message:   fmt.Sprintf("Created deployment %s", object.GetName()),
		Level:     sentry.LevelInfo,
		Timestamp: object.GetCreationTimestamp().Time,
	}, breadcrumbLimit)

	return nil
}
