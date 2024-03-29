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

func runEnhancers(ctx context.Context, eventObject *v1.Event, kind string, object metav1.Object, scope *sentry.Scope, sentryEvent *sentry.Event) error {
	logger := zerolog.Ctx(ctx)
	logger.Debug().Msgf("Running the enhancer")

	var err error

	// First, run the common enhancer which
	// does not need neither the event object
	// nor the involved object
	err = runCommonEnhancer(ctx, scope, sentryEvent)
	if err != nil {
		return err
	}

	// If an event object is provided, we call the event enhancer
	if eventObject != nil {
		err = eventEnhancer(scope, eventObject)
		if err != nil {
			return err
		}
	}

	// If an involved object is provided, we call the object enhancer
	if object != nil {
		err = objectEnhancer(ctx, scope, &KindObjectPair{
			kind:   kind,
			object: object,
		}, sentryEvent)
		if err != nil {
			return err
		}
	}

	return nil
}

type KindObjectPair struct {
	kind   string
	object metav1.Object
}

func eventEnhancer(scope *sentry.Scope, object metav1.Object) error {
	eventObj, ok := object.(*v1.Event)
	if !ok {
		return errors.New("failed to cast object to event object")
	}

	// The involved object is likely very similar
	// to the involved object's metadata which will
	// be included when the the object's enhancer
	// eventually gets triggered
	scope.RemoveExtra("Involved Object")

	// Add related events as breadcrumbs
	objEvents := filterEventsFromBuffer(eventObj.Namespace, "Event", eventObj.Name)
	for _, objEvent := range objEvents {
		breadcrumbLevel := sentry.LevelInfo
		if objEvent.Type == v1.EventTypeWarning {
			breadcrumbLevel = sentry.LevelWarning
		}

		scope.AddBreadcrumb(&sentry.Breadcrumb{
			Message:   objEvent.Message,
			Level:     breadcrumbLevel,
			Timestamp: objEvent.LastTimestamp.Time,
		}, breadcrumbLimit)
	}

	return nil
}

func objectEnhancer(ctx context.Context, scope *sentry.Scope, kindObjectPair *KindObjectPair, sentryEvent *sentry.Event) error {
	objectTag := fmt.Sprintf("%s/%s", kindObjectPair.kind, kindObjectPair.object.GetName())
	ctx, logger := getLoggerWithTag(ctx, "object", objectTag)

	var err error

	logger.Trace().Msgf("Current fingerprint: %v", sentryEvent.Fingerprint)

	// Enhance message with object name
	message := sentryEvent.Message
	sentryEvent.Message = fmt.Sprintf("%s: %s", kindObjectPair.object.GetName(), sentryEvent.Message)

	// Adjust fingerprint.
	// If there's already a non-empty fingerprint set, we assume that it was set by
	// another enhancer, so we don't touch it.
	if len(sentryEvent.Fingerprint) == 0 {
		sentryEvent.Fingerprint = []string{message}
	}
	logger.Trace().Msgf("Fingerprint after adjustment: %v", sentryEvent.Fingerprint)

	// Find the root owners and their corresponding object kinds
	rootOwners, err := findRootOwners(ctx, kindObjectPair)
	if err != nil {
		return err
	}

	// Might reset back to old fingerprint if
	// there exists root owner(s) to the object
	oldFingerprint := sentryEvent.Fingerprint
	// Call the specific enhancer for the object
	err = getKindEnhancer(kindObjectPair.kind)(ctx, scope, kindObjectPair.object, sentryEvent)
	if err != nil {
		return err
	}
	// Remove any fingerprinting so the event
	// can be grouped by its owners instead
	if len(rootOwners) != 0 {
		sentryEvent.Fingerprint = oldFingerprint
	}

	// Call specific enhancers for all root owners
	// (there most likely is just one root owner)
	for _, rootOwner := range rootOwners {
		err = getKindEnhancer(rootOwner.kind)(ctx, scope, rootOwner.object, sentryEvent)
		if err != nil {
			return err
		}
	}

	return nil
}

func getKindEnhancer(kind string) func(context.Context, *sentry.Scope, metav1.Object, *sentry.Event) error {
	switch kind {
	case KindPod:
		return podEnhancer
	case KindReplicaset:
		return replicaSetEnhancer
	case KindDeployment:
		return deploymentEnhancer
	case KindJob:
		return jobEnhancer
	case KindCronjob:
		return cronjobEnhancer
	default:
		return func(ctx context.Context, scope *sentry.Scope, object metav1.Object, sentryEvent *sentry.Event) error {
			sentryEvent.Fingerprint = append(sentryEvent.Fingerprint, object.GetName())
			return nil
		}
	}
}

func podEnhancer(ctx context.Context, scope *sentry.Scope, object metav1.Object, sentryEvent *sentry.Event) error {
	podObj, ok := object.(*v1.Pod)
	if !ok {
		return errors.New("failed to cast object to Pod object")
	}

	// Add node name as tag
	nodeName := podObj.Spec.NodeName
	setTagIfNotEmpty(scope, "node_name", nodeName)

	// Add the pod name to the fingerprint
	sentryEvent.Fingerprint = append(sentryEvent.Fingerprint, KindPod, podObj.Name)

	// Add the pod to the tag
	setTagIfNotEmpty(scope, "pod_name", object.GetName())
	podObj.ManagedFields = []metav1.ManagedFieldsEntry{}
	metadataJSON, err := prettyJSON(podObj.ObjectMeta)
	if err == nil {
		scope.SetContext(KindPod, sentry.Context{
			"Metadata": metadataJSON,
		})
	}

	// Add breadcrumb with pod timestamps
	scope.AddBreadcrumb(&sentry.Breadcrumb{
		Message:   fmt.Sprintf("Created pod %s", object.GetName()),
		Level:     sentry.LevelInfo,
		Timestamp: object.GetCreationTimestamp().Time,
	}, breadcrumbLimit)

	addPodLogLinkToGKEContext(ctx, scope, podObj.Name, podObj.Namespace)

	return nil
}

func jobEnhancer(ctx context.Context, scope *sentry.Scope, object metav1.Object, sentryEvent *sentry.Event) error {
	jobObj, ok := object.(*batchv1.Job)
	if !ok {
		return errors.New("failed to cast object to Job object")
	}

	// Add the job to the fingerprint
	sentryEvent.Fingerprint = append(sentryEvent.Fingerprint, KindJob, jobObj.Name)

	// Add the job to the tag
	setTagIfNotEmpty(scope, "job_name", object.GetName())
	jobObj.ManagedFields = []metav1.ManagedFieldsEntry{}
	metadataJSON, err := prettyJSON(jobObj.ObjectMeta)
	if err == nil {
		scope.SetContext(KindJob, sentry.Context{
			"Metadata": metadataJSON,
		})
	}

	// Add breadcrumb with job timestamps
	scope.AddBreadcrumb(&sentry.Breadcrumb{
		Message:   fmt.Sprintf("Created job %s", object.GetName()),
		Level:     sentry.LevelInfo,
		Timestamp: object.GetCreationTimestamp().Time,
	}, breadcrumbLimit)

	return nil
}

func cronjobEnhancer(ctx context.Context, scope *sentry.Scope, object metav1.Object, sentryEvent *sentry.Event) error {
	cronjobObj, ok := object.(*batchv1.CronJob)
	if !ok {
		return errors.New("failed to cast object to CronJob object")
	}

	// Set the context for corresponding slug monitor
	scope.SetContext("Monitor", sentry.Context{
		"Slug": object.GetName(),
	})

	// Add the cronjob to the fingerprint
	sentryEvent.Fingerprint = append(sentryEvent.Fingerprint, KindCronjob, cronjobObj.Name)

	// Add the cronjob to the tag
	setTagIfNotEmpty(scope, "cronjob_name", object.GetName())
	cronjobObj.ManagedFields = []metav1.ManagedFieldsEntry{}
	metadataJSON, err := prettyJSON(cronjobObj.ObjectMeta)
	if err == nil {
		scope.SetContext(KindCronjob, sentry.Context{
			"Metadata": metadataJSON,
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

func replicaSetEnhancer(ctx context.Context, scope *sentry.Scope, object metav1.Object, sentryEvent *sentry.Event) error {
	replicasetObj, ok := object.(*appsv1.ReplicaSet)
	if !ok {
		return errors.New("failed to cast object to ReplicaSet object")
	}

	// Add the replicaset to the fingerprint
	sentryEvent.Fingerprint = append(sentryEvent.Fingerprint, KindReplicaset, replicasetObj.Name)

	// Add the replicaset to the tag
	setTagIfNotEmpty(scope, "replicaset_name", object.GetName())
	replicasetObj.ManagedFields = []metav1.ManagedFieldsEntry{}
	metadataJSON, err := prettyJSON(replicasetObj.ObjectMeta)
	if err == nil {
		scope.SetContext(KindReplicaset, sentry.Context{
			"Metadata": metadataJSON,
		})
	}

	// Add breadcrumb with replicaset timestamps
	scope.AddBreadcrumb(&sentry.Breadcrumb{
		Message:   fmt.Sprintf("Created replicaset %s", object.GetName()),
		Level:     sentry.LevelInfo,
		Timestamp: object.GetCreationTimestamp().Time,
	}, breadcrumbLimit)

	return nil
}

func deploymentEnhancer(ctx context.Context, scope *sentry.Scope, object metav1.Object, sentryEvent *sentry.Event) error {
	deploymentObj, ok := object.(*appsv1.Deployment)
	if !ok {
		return errors.New("failed to cast object to Deployment object")
	}
	// Add the deployment to the fingerprint
	sentryEvent.Fingerprint = append(sentryEvent.Fingerprint, KindDeployment, deploymentObj.Name)

	// Add the deployment to the tag
	setTagIfNotEmpty(scope, "deployment_name", object.GetName())
	deploymentObj.ManagedFields = []metav1.ManagedFieldsEntry{}
	metadataJSON, err := prettyJSON(deploymentObj.ObjectMeta)
	if err == nil {
		scope.SetContext(KindDeployment, sentry.Context{
			"Metadata": metadataJSON,
		})
	}

	// Add breadcrumb with deployment timestamps
	scope.AddBreadcrumb(&sentry.Breadcrumb{
		Message:   fmt.Sprintf("Created deployment %s", object.GetName()),
		Level:     sentry.LevelInfo,
		Timestamp: object.GetCreationTimestamp().Time,
	}, breadcrumbLimit)

	return nil
}

// Finds the root owning objects of an object
// and returns an empty slice if the object has
// no owning objects
func findRootOwners(ctx context.Context, kindObjPair *KindObjectPair) ([]KindObjectPair, error) {
	// Use DFS to find the leaves of the owner references graph
	rootOwners, err := ownerRefDFS(ctx, kindObjPair)
	if err != nil {
		return nil, err
	}

	// If the object has no owner references
	if rootOwners[0].object.GetUID() == kindObjPair.object.GetUID() {
		return []KindObjectPair{}, nil
	}

	return rootOwners, nil
}

// Performs DFS to find the leaves the owner references graph
func ownerRefDFS(ctx context.Context, kindObjPair *KindObjectPair) ([]KindObjectPair, error) {
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
		partialOwners, err := ownerRefDFS(ctx, &KindObjectPair{
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
