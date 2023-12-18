package main

import (
	"context"
	"errors"

	"github.com/getsentry/sentry-go"
	"github.com/rs/zerolog"
	batchv1 "k8s.io/api/batch/v1"
)

type EventHandlerType string

const (
	EventHandlerAdd    EventHandlerType = "ADD"
	EventHandlerUpdate EventHandlerType = "UPDATE"
	EventHandlerDelete EventHandlerType = "DELETE"
)

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
	if !*cronjobRef.Controller || cronjobRef.Kind != CRONJOB {
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
