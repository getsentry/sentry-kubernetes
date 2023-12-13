package main

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/getsentry/sentry-go"
	"github.com/rs/zerolog"
	batchv1 "k8s.io/api/batch/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/tools/cache"
)

type EventHandlerType string

const (
	EventHandlerAdd    EventHandlerType = "ADD"
	EventHandlerUpdate EventHandlerType = "UPDATE"
	EventHandlerDelete EventHandlerType = "DELETE"
)

var cronjobInformer cache.SharedIndexInformer
var jobInformer cache.SharedIndexInformer

// Starts the crons informer which has event handlers
// adds to the crons monitor data struct used for sending
// checkin events to Sentry
func startCronsInformers(ctx context.Context, namespace string) error {

	clientset, err := getClientsetFromContext(ctx)
	if err != nil {
		return errors.New("failed to get clientset")
	}

	// Create factory that will produce both the cronjob informer and job informer
	factory := informers.NewSharedInformerFactoryWithOptions(
		clientset,
		5*time.Second,
		informers.WithNamespace(namespace),
	)

	// Create the cronjob informer
	cronjobInformer, err = createCronjobInformer(ctx, factory, namespace)
	if err != nil {
		return err
	}
	// Create the job informer
	jobInformer, err = createJobInformer(ctx, factory, namespace)
	if err != nil {
		return err
	}

	// Channel to tell the factory to stop the informers
	doneChan := make(chan struct{})
	factory.Start(doneChan)

	// Sync the cronjob informer cache
	if ok := cache.WaitForCacheSync(doneChan, cronjobInformer.HasSynced); !ok {
		return errors.New("cronjob informer failed to sync")
	}
	// Sync the job informer cache
	if ok := cache.WaitForCacheSync(doneChan, jobInformer.HasSynced); !ok {
		return errors.New("job informer failed to sync")
	}

	// Wait for the channel to be closed
	<-doneChan

	return nil
}

// Captures sentry crons checkin event if appropriate
// by checking the job status to determine if the job just created pod (job starting)
// or if the job exited
func runSentryCronsCheckin(ctx context.Context, job *batchv1.Job, eventHandlerType EventHandlerType) error {

	hub := sentry.GetHubFromContext(ctx)
	if hub == nil {
		return errors.New("cannot get hub from context")
	}

	// To avoid concurrency issue
	hub = hub.Clone()

	// Try to find the cronJob name that owns the job
	// in order to get the crons monitor data
	if len(job.OwnerReferences) == 0 {
		return errors.New("job does not have cronjob reference")
	}
	cronjobRef := job.OwnerReferences[0]
	if !*cronjobRef.Controller || cronjobRef.Kind != "CronJob" {
		return errors.New("job does not have cronjob reference")
	}
	cronsMonitorData, ok := cronsMetaData.getCronsMonitorData(cronjobRef.Name)
	if !ok {
		return errors.New("cannot find cronJob data")
	}

	hub.WithScope(func(scope *sentry.Scope) {

		// If DSN annotation provided, we bind a new client with that DSN
		client, ok := dsnClientMapping.GetClientFromObject(ctx, &job.ObjectMeta, hub.Client().Options())
		if ok {
			hub.BindClient(client)
		}

		// Pass clone hub down with context
		ctx = sentry.SetHubOnContext(ctx, hub)
		// The job just begun so check in to start
		if job.Status.Active == 0 && job.Status.Succeeded == 0 && job.Status.Failed == 0 {
			// Add the job to the cronJob informer data
			checkinJobStarting(ctx, job, cronsMonitorData)
		} else if job.Status.Active > 0 {
			return
		} else if job.Status.Failed > 0 || job.Status.Succeeded > 0 {
			checkinJobEnding(ctx, job, cronsMonitorData)
			return // Finished
		}
	})

	return nil
}

// Sends the checkin event to sentry crons for when a job starts
func checkinJobStarting(ctx context.Context, job *batchv1.Job, cronsMonitorData *CronsMonitorData) error {

	logger := zerolog.Ctx(ctx)

	hub := sentry.GetHubFromContext(ctx)
	if hub == nil {
		return errors.New("cannot get hub from context")
	}

	// Check if job already added to jobData slice
	_, ok := cronsMonitorData.JobDatas[job.Name]
	if ok {
		return nil
	}
	logger.Debug().Msgf("Checking in at start of job: %s\n", job.Name)

	// All containers running in the pod
	checkinId := hub.CaptureCheckIn(
		&sentry.CheckIn{
			MonitorSlug: cronsMonitorData.MonitorSlug,
			Status:      sentry.CheckInStatusInProgress,
		},
		cronsMonitorData.monitorConfig,
	)
	cronsMonitorData.addJob(job, *checkinId)

	return nil
}

// Sends the checkin event to sentry crons for when a job ends
func checkinJobEnding(ctx context.Context, job *batchv1.Job, cronsMonitorData *CronsMonitorData) error {

	hub := sentry.GetHubFromContext(ctx)
	if hub == nil {
		return errors.New("cannot get hub from context")
	}

	logger := zerolog.Ctx(ctx)

	// Check desired number of pods have succeeded
	var jobStatus sentry.CheckInStatus

	if job.Status.Conditions == nil {
		return nil
	} else {
		if job.Status.Conditions[0].Type == "Complete" {
			jobStatus = sentry.CheckInStatusOK
		} else if job.Status.Conditions[0].Type == "Failed" {
			jobStatus = sentry.CheckInStatusError
		} else {
			return nil
		}
	}

	// Get job data to retrieve the checkin ID
	jobData, ok := cronsMonitorData.JobDatas[job.Name]
	if !ok {
		return nil
	}

	logger.Trace().Msgf("checking in at end of job: %s\n", job.Name)
	hub.CaptureCheckIn(
		&sentry.CheckIn{
			ID:          jobData.getCheckinId(),
			MonitorSlug: cronsMonitorData.MonitorSlug,
			Status:      jobStatus,
		},
		cronsMonitorData.monitorConfig,
	)
	return nil
}

// Adds to the sentry events whenever it is associated with a cronjob
// so the sentry event contains the corresponding slug monitor, cronjob name, timestamp of when the cronjob began, and
// the k8s cronjob metadata
func runCronsDataHandler(ctx context.Context, scope *sentry.Scope, pod *v1.Pod, sentryEvent *sentry.Event) (bool, error) {

	// get owningCronJob if exists
	owningCronJob, err := getOwningCronJob(ctx, pod)
	if err != nil {
		return false, err
	}

	// pod not part of a cronjob
	if owningCronJob == nil {
		return false, nil
	}

	scope.SetContext("Monitor", sentry.Context{
		"Slug": owningCronJob.Name,
	})

	sentryEvent.Fingerprint = append(sentryEvent.Fingerprint, owningCronJob.Kind, owningCronJob.Name)

	setTagIfNotEmpty(scope, "cronjob_name", owningCronJob.Name)

	// add breadcrumb with cronJob timestamps
	scope.AddBreadcrumb(&sentry.Breadcrumb{
		Message:   fmt.Sprintf("Created cronjob %s", owningCronJob.Name),
		Level:     sentry.LevelInfo,
		Timestamp: owningCronJob.CreationTimestamp.Time,
	}, breadcrumbLimit)

	metadataJson, err := prettyJson(owningCronJob.ObjectMeta)

	if err == nil {
		scope.SetContext("Cronjob", sentry.Context{
			"Metadata": metadataJson,
		})
	} else {
		return false, err
	}

	return true, nil
}

// returns the cronjob that is the grandparent of a pod if exists
// but returns nil is no cronjob is found
func getOwningCronJob(ctx context.Context, pod *v1.Pod) (*batchv1.CronJob, error) {

	clientset, err := getClientsetFromContext(ctx)
	if err != nil {
		return nil, err
	}

	namespace := pod.Namespace

	// first attempt to group events by cronJobs
	var owningCronJob *batchv1.CronJob = nil

	// check if the pod corresponds to a cronJob
	for _, podRef := range pod.ObjectMeta.OwnerReferences {
		// check the pod has a job as an owner
		if !*podRef.Controller || podRef.Kind != "Job" {
			continue
		}
		// find the owning job
		owningJob, err := clientset.BatchV1().Jobs(namespace).Get(context.Background(), podRef.Name, metav1.GetOptions{})
		if err != nil {
			continue
		}
		// check if owning job is owned by a cronJob
		for _, jobRef := range owningJob.ObjectMeta.OwnerReferences {
			if !*jobRef.Controller || jobRef.Kind != "CronJob" {
				continue
			}
			owningCronJob, err = clientset.BatchV1().CronJobs(namespace).Get(context.Background(), jobRef.Name, metav1.GetOptions{})
			if err != nil {
				continue
			}
		}
	}

	return owningCronJob, nil
}
