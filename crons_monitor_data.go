package main

import (
	"github.com/getsentry/sentry-go"
	batchv1 "k8s.io/api/batch/v1"
)

type CronsInformerDataKey struct{}

// Struct associated with a job
type CronsJobData struct {
	CheckinId sentry.EventID
}

// Constructor for cronsMonitorData
func NewCronsJobData(checkinId sentry.EventID) *CronsJobData {
	return &CronsJobData{
		CheckinId: checkinId,
	}
}

func (j *CronsJobData) getCheckinId() sentry.EventID {
	return j.CheckinId
}

// Struct associated with a cronJob
type CronsMonitorData struct {
	MonitorSlug         string
	monitorConfig       *sentry.MonitorConfig
	JobDatas            map[string]*CronsJobData
	requiredCompletions int32
}

// Constructor for cronsMonitorData
func NewCronsMonitorData(monitorSlug string, schedule string, maxRunTime int64, checkinMargin int64, completions *int32) *CronsMonitorData {

	// Get required number of pods to complete
	var requiredCompletions int32
	if completions == nil {
		// If not set, any pod success is enough
		requiredCompletions = 1
	} else {
		requiredCompletions = *completions
	}
	monitorSchedule := sentry.CrontabSchedule(schedule)
	return &CronsMonitorData{
		MonitorSlug: monitorSlug,
		monitorConfig: &sentry.MonitorConfig{
			Schedule:      monitorSchedule,
			MaxRuntime:    maxRunTime,
			CheckInMargin: checkinMargin,
		},
		JobDatas:            make(map[string]*CronsJobData),
		requiredCompletions: requiredCompletions,
	}
}

// Add a job to the crons monitor
func (c *CronsMonitorData) addJob(job *batchv1.Job, checkinId sentry.EventID) error {
	c.JobDatas[job.Name] = NewCronsJobData(checkinId)
	return nil
}
