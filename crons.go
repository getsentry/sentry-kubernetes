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

// Starts the crons informer which has event handlers
// adds to the crons monitor data struct used for sending
// checkin events to Sentry
func startCronsInformers(ctx context.Context, namespace string) error {

	clientset, err := getClientsetFromContext(ctx)
	if err != nil {
		return errors.New("failed to get clientset")
	}

	// create factory that will produce both the cronjob informer and job informer
	factory := informers.NewSharedInformerFactoryWithOptions(
		clientset,
		5*time.Second,
		informers.WithNamespace(namespace),
	)

	// create the cronjob informer
	cronjobInformer, err := createCronjobInformer(ctx, factory, namespace)
	if err != nil {
		return err
	}
	// create the job informer
	jobInformer, err := createJobInformer(ctx, factory, namespace)
	if err != nil {
		return err
	}

	// channel to tell the factory to stop the informers
	doneChan := make(chan struct{})
	factory.Start(doneChan)

	// sync the cronjob informer cache
	if ok := cache.WaitForCacheSync(doneChan, cronjobInformer.HasSynced); !ok {
		return errors.New("cronjob informer failed to sync")
	}
	// sync the job informer cache
	if ok := cache.WaitForCacheSync(doneChan, jobInformer.HasSynced); !ok {
		return errors.New("job informer failed to sync")
	}

	// wait for the channel to be closed
	<-doneChan

	return nil
}

// Starts the jobs informer with event handlers that trigger
// checkin events during the start and end of a job (along with the exit status)
func runSentryCronsCheckin(ctx context.Context, job *batchv1.Job, eventHandlerType EventHandlerType) error {

	// Query the crons informer data
	val := ctx.Value(CronsInformerDataKey{})
	if val == nil {
		return errors.New("no crons informer data struct given")
	}
	var cronsInformerData *map[string]CronsMonitorData
	var ok bool
	if cronsInformerData, ok = val.(*map[string]CronsMonitorData); !ok {
		return errors.New("cannot convert cronsInformerData value from context")
	}

	// Try to find the cronJob name that owns the job
	// in order to get the crons monitor data
	if len(job.OwnerReferences) == 0 {
		return errors.New("job does not have cronjob reference")
	}
	cronjobRef := job.OwnerReferences[0]
	if !*cronjobRef.Controller || cronjobRef.Kind != "CronJob" {
		return errors.New("job does not have cronjob reference")
	}
	cronsMonitorData, ok := (*cronsInformerData)[cronjobRef.Name]
	if !ok {
		return errors.New("cannot find cronJob data")
	}

	// capture checkin event called for by informer handler
	if eventHandlerType == EventHandlerAdd {
		// Add the job to the cronJob informer data
		checkinJobStarting(ctx, job, cronsMonitorData)
	} else if eventHandlerType == EventHandlerUpdate || eventHandlerType == EventHandlerDelete {
		// Delete pod from the cronJob informer data
		checkinJobEnding(ctx, job, cronsMonitorData)
	}

	return nil
}

// sends the checkin event to sentry crons for when a job starts
func checkinJobStarting(ctx context.Context, job *batchv1.Job, cronsMonitorData CronsMonitorData) error {

	logger := zerolog.Ctx(ctx)

	// Check if job already added to jobData slice
	_, ok := cronsMonitorData.JobDatas[job.Name]
	if ok {
		return nil
	}
	logger.Debug().Msgf("Checking in at start of job: %s\n", job.Name)

	// All containers running in the pod
	checkinId := sentry.CaptureCheckIn(
		&sentry.CheckIn{
			MonitorSlug: cronsMonitorData.MonitorSlug,
			Status:      sentry.CheckInStatusInProgress,
		},
		cronsMonitorData.monitorConfig,
	)
	cronsMonitorData.addJob(job, *checkinId)

	return nil
}

// sends the checkin event to sentry crons for when a job ends
func checkinJobEnding(ctx context.Context, job *batchv1.Job, cronsMonitorData CronsMonitorData) error {

	logger := zerolog.Ctx(ctx)
	// do not check in to exit if there are still active pods
	if job.Status.Active > 0 {
		return nil
	}

	// Check desired number of pods have succeeded
	var jobStatus sentry.CheckInStatus
	if job.Status.Succeeded >= cronsMonitorData.requiredCompletions {
		jobStatus = sentry.CheckInStatusOK
	} else {
		jobStatus = sentry.CheckInStatusError
	}

	// Get job data to retrieve the checkin ID
	jobData, ok := cronsMonitorData.JobDatas[job.Name]
	if !ok {
		return nil
	}

	logger.Trace().Msgf("checking in at end of job: %s\n", job.Name)
	sentry.CaptureCheckIn(
		&sentry.CheckIn{
			ID:          jobData.getCheckinId(),
			MonitorSlug: cronsMonitorData.MonitorSlug,
			Status:      jobStatus,
		},
		cronsMonitorData.monitorConfig,
	)
	return nil
}

// adds to the sentry events whenever it is associated with a cronjob
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
