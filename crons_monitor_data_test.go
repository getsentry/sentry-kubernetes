package main

import (
	"testing"

	"github.com/getsentry/sentry-go"
	batchv1 "k8s.io/api/batch/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestNewCronsJobData(t *testing.T) {

	fakeId := "080181f33ca343f89b0bf55d50abfeee"

	cronsJobData := NewCronsJobData(sentry.EventID(fakeId))
	if cronsJobData == nil {
		t.Errorf("Failed to create cronsJobData")
		return
	}
	if cronsJobData.CheckinId != sentry.EventID(fakeId) {
		t.Errorf("The cronsJobData set to incorrect ID")
	}
}

func TestGetCheckinId(t *testing.T) {

	fakeId := "080181f33ca343f89b0bf55d50abfeee"

	cronsJobData := NewCronsJobData(sentry.EventID(fakeId))
	if cronsJobData == nil {
		t.Errorf("Failed to create cronsJobData")
		return
	}
	if cronsJobData.getCheckinId() != sentry.EventID(fakeId) {
		t.Errorf("Retrieved incorrect checkin ID")
	}
}

func TestNewCronsMonitorData(t *testing.T) {

	fakeMonitorSlug := "cronjob-slug"
	fakeSchedule := "* * * * *"
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

	fakeId := "080181f33ca343f89b0bf55d50abfeee"
	fakeMonitorSlug := "cronjob-slug"
	fakeSchedule := "* * * * *"
	var fakeCompletions int32 = 3

	cronsMonitorData := NewCronsMonitorData(fakeMonitorSlug, fakeSchedule, &fakeCompletions)

	jobObj := &batchv1.Job{
		ObjectMeta: v1.ObjectMeta{
			Name: "TestAddJobJob",
		},
	}
	cronsMonitorData.addJob(jobObj, sentry.EventID(fakeId))

	jobData, ok := cronsMonitorData.JobDatas["TestAddJobJob"]
	if !ok {
		t.Errorf("Failed to add job data")
	}
	if jobData.CheckinId != sentry.EventID(fakeId) {
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

	fakeMonitorSlug := "TestAddCronsMonitorDataCronJob"
	fakeSchedule := "* * * * *"
	var fakeCompletions int32 = 3
	cronsMonitorData := NewCronsMonitorData(fakeMonitorSlug, fakeSchedule, &fakeCompletions)

	cronsMetaData.addCronsMonitorData("TestAddCronsMonitorDataCronJob", cronsMonitorData)

	retCronsMonitorData, ok := cronsMetaData.cronsMonitorDataMap["TestAddCronsMonitorDataCronJob"]
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

	fakeMonitorSlug := "TestAddCronsMonitorDataCronJob"
	fakeSchedule := "* * * * *"
	var fakeCompletions int32 = 3
	cronsMonitorData := NewCronsMonitorData(fakeMonitorSlug, fakeSchedule, &fakeCompletions)

	cronsMetaData.cronsMonitorDataMap["TestAddCronsMonitorDataCronJob"] = cronsMonitorData

	cronsMetaData.deleteCronsMonitorData("TestAddCronsMonitorDataCronJob")

	_, ok := cronsMetaData.cronsMonitorDataMap["TestAddCronsMonitorDataCronJob"]

	if ok {
		t.Errorf("Failed to delete cronsMonitorData from map")
	}

}

func TestGetCronsMonitorData(t *testing.T) {

	cronsMetaData := NewCronsMetaData()
	if cronsMetaData.cronsMonitorDataMap == nil {
		t.Errorf("Failed to create cronsMonitorDataMap")
	}

	fakeMonitorSlug := "TestAddCronsMonitorDataCronJob"
	fakeSchedule := "* * * * *"
	var fakeCompletions int32 = 3
	cronsMonitorData := NewCronsMonitorData(fakeMonitorSlug, fakeSchedule, &fakeCompletions)

	cronsMetaData.cronsMonitorDataMap["TestAddCronsMonitorDataCronJob"] = cronsMonitorData

	retCronsMonitorData, ok := cronsMetaData.getCronsMonitorData("TestAddCronsMonitorDataCronJob")
	if !ok {
		t.Errorf("Failed to get cronsMonitorData to map")
	}
	if retCronsMonitorData != cronsMonitorData {
		t.Errorf("Failed to get correct cronsMonitorData to map")
	}

}
