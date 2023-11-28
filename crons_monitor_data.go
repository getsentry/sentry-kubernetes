package main

import (
	"sync"

	"github.com/getsentry/sentry-go"
	batchv1 "k8s.io/api/batch/v1"
)

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
	mutex               sync.RWMutex
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
		mutex:       sync.RWMutex{},
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
	c.mutex.Lock()
	c.JobDatas[job.Name] = NewCronsJobData(checkinId)
	c.mutex.Unlock()
	return nil
}

// wrapper struct over crons monitor map that
// handles syncrhonization
type CronsMetaData struct {
	mutex               *sync.RWMutex
	cronsMonitorDataMap map[string]*CronsMonitorData
}

func (c *CronsMetaData) addCronsMonitorData(cronjobName string, newCronsMonitorData *CronsMonitorData) {
	c.mutex.Lock()
	c.cronsMonitorDataMap[cronjobName] = newCronsMonitorData
	c.mutex.Unlock()
}

func (c *CronsMetaData) deleteCronsMonitorData(cronjobName string) {
	c.mutex.Lock()
	delete(c.cronsMonitorDataMap, cronjobName)
	c.mutex.Unlock()
}

func (c *CronsMetaData) getCronsMonitorData(cronjobName string) (*CronsMonitorData, bool) {
	c.mutex.RLock()
	cronsMonitorData, ok := c.cronsMonitorDataMap[cronjobName]
	c.mutex.RUnlock()
	return cronsMonitorData, ok
}

func NewCronsMetaData() *CronsMetaData {
	return &CronsMetaData{
		mutex:               &sync.RWMutex{},
		cronsMonitorDataMap: make(map[string]*CronsMonitorData),
	}
}
