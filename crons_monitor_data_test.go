package main

import (
	"testing"

	"github.com/getsentry/sentry-go"
	batchv1 "k8s.io/api/batch/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	testFakeID       = "080181f33ca343f89b0bf55d50abfeee"
	testFakeSchedule = "* * * * *"
	testCronJobName  = "TestAddCronsMonitorDataCronJob"
)

func TestNewCronsJobData(t *testing.T) {
	fakeID := testFakeID

	cronsJobData := NewCronsJobData(sentry.EventID(fakeID))
	if cronsJobData == nil {
		t.Errorf("Failed to create cronsJobData")
		return
	}
	if cronsJobData.CheckinID != sentry.EventID(fakeID) {
		t.Errorf("The cronsJobData set to incorrect ID")
	}
}

func TestGetCheckinId(t *testing.T) {
	fakeID := testFakeID

	cronsJobData := NewCronsJobData(sentry.EventID(fakeID))
	if cronsJobData == nil {
		t.Errorf("Failed to create cronsJobData")
		return
	}
	if cronsJobData.getCheckinID() != sentry.EventID(fakeID) {
		t.Errorf("Retrieved incorrect checkin ID")
	}
}

func TestNewCronsMonitorData(t *testing.T) {
	fakeMonitorSlug := "cronjob-slug"
	fakeSchedule := testFakeSchedule
	var fakeCompletions int32 = 3

	cronsMonitorData := NewCronsMonitorData(fakeMonitorSlug, fakeSchedule, &fakeCompletions)

	if cronsMonitorData.MonitorSlug != fakeMonitorSlug {
		t.Errorf("The monitor slug is incorrect")
	}
	if cronsMonitorData.monitorConfig.Schedule != sentry.CrontabSchedule(fakeSchedule) {
		t.Errorf("The schedule is incorrect")
	}
	if cronsMonitorData.JobDatas == nil {
		t.Errorf("Failed to create jobDatas map")
	}
	if cronsMonitorData.requiredCompletions != fakeCompletions {
		t.Errorf("The completions is incorrect")
	}
}

func TestAddJob(t *testing.T) {
	fakeID := testFakeID
	fakeMonitorSlug := "cronjob-slug"
	fakeSchedule := testFakeSchedule
	var fakeCompletions int32 = 3

	cronsMonitorData := NewCronsMonitorData(fakeMonitorSlug, fakeSchedule, &fakeCompletions)

	jobObj := &batchv1.Job{
		ObjectMeta: v1.ObjectMeta{
			Name: "TestAddJobJob",
		},
	}
	err := cronsMonitorData.addJob(jobObj, sentry.EventID(fakeID))
	if err != nil {
		t.Errorf("Failed to add job")
	}

	jobData, ok := cronsMonitorData.JobDatas["TestAddJobJob"]
	if !ok {
		t.Errorf("Failed to add job data")
	}
	if jobData.CheckinID != sentry.EventID(fakeID) {
		t.Errorf("Incorrect checkin ID")
	}
}

func TestNewCronsMetaData(t *testing.T) {
	cronsMetaData := NewCronsMetaData()
	if cronsMetaData.cronsMonitorDataMap == nil {
		t.Errorf("Failed to create cronsMonitorDataMap")
	}
}

func TestAddCronsMonitorData(t *testing.T) {
	cronsMetaData := NewCronsMetaData()
	if cronsMetaData.cronsMonitorDataMap == nil {
		t.Errorf("Failed to create cronsMonitorDataMap")
	}

	cronjobName := testCronJobName
	fakeSchedule := testFakeSchedule
	var fakeCompletions int32 = 3
	cronsMonitorData := NewCronsMonitorData(cronjobName, fakeSchedule, &fakeCompletions)

	cronsMetaData.addCronsMonitorData(cronjobName, cronsMonitorData)

	retCronsMonitorData, ok := cronsMetaData.cronsMonitorDataMap[cronjobName]
	if !ok {
		t.Errorf("Failed to add cronsMonitorData to map")
	}
	if retCronsMonitorData != cronsMonitorData {
		t.Errorf("Failed to add correct cronsMonitorData to map")
	}
}

func TestDeleteCronsMonitorData(t *testing.T) {
	cronsMetaData := NewCronsMetaData()
	if cronsMetaData.cronsMonitorDataMap == nil {
		t.Errorf("Failed to create cronsMonitorDataMap")
	}

	fakeMonitorSlug := testCronJobName
	fakeSchedule := testFakeSchedule
	var fakeCompletions int32 = 3
	cronsMonitorData := NewCronsMonitorData(fakeMonitorSlug, fakeSchedule, &fakeCompletions)

	cronsMetaData.cronsMonitorDataMap[testCronJobName] = cronsMonitorData

	cronsMetaData.deleteCronsMonitorData(testCronJobName)

	_, ok := cronsMetaData.cronsMonitorDataMap[testCronJobName]

	if ok {
		t.Errorf("Failed to delete cronsMonitorData from map")
	}
}

func TestGetCronsMonitorData(t *testing.T) {
	cronsMetaData := NewCronsMetaData()
	if cronsMetaData.cronsMonitorDataMap == nil {
		t.Errorf("Failed to create cronsMonitorDataMap")
	}

	fakeMonitorSlug := testCronJobName
	fakeSchedule := testFakeSchedule
	var fakeCompletions int32 = 3
	cronsMonitorData := NewCronsMonitorData(fakeMonitorSlug, fakeSchedule, &fakeCompletions)

	cronsMetaData.cronsMonitorDataMap[testCronJobName] = cronsMonitorData

	retCronsMonitorData, ok := cronsMetaData.getCronsMonitorData(testCronJobName)
	if !ok {
		t.Errorf("Failed to get cronsMonitorData to map")
	}
	if retCronsMonitorData != cronsMonitorData {
		t.Errorf("Failed to get correct cronsMonitorData to map")
	}
}
