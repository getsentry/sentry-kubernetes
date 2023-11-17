package main

import (
	"context"
	"testing"
	"time"

	"github.com/getsentry/sentry-go"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
)

// Test the function handleWatchEvent
// by giving it a mock event
// and checking it causes Sentry to capture the event
// with the correct message and Sentry tags
func TestHandleWatchEvent(t *testing.T) {

	// Create empty context
	ctx := context.Background()

	// Define an SDK transport that only captures events but not send them
	transport := &TransportMock{}
	client, err := sentry.NewClient(sentry.ClientOptions{
		Transport: transport,
		Integrations: func([]sentry.Integration) []sentry.Integration {
			return []sentry.Integration{}
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	// Create new scope
	scope := sentry.NewScope()
	// Create hub with new scope
	hub := sentry.NewHub(client, scope)
	// Attach the hub to the empty context
	ctx = sentry.SetHubOnContext(ctx, hub)

	// Create the watch event which includes the mock API event
	// where event is of a warning type

	// Make up timestamps needed for Event object
	creationTime, _ := time.Parse("2006-01-02 15:04:05", "2023-11-15 01:04:04")
	firstTime, _ := time.Parse("2006-01-02 15:04:05", "2023-11-15 01:04:04")
	lastTime, _ := time.Parse("2006-01-02 15:04:05", "2023-11-15 18:42:00")

	mockEvent := watch.Event{
		Type: watch.Modified,
		Object: &corev1.Event{
			TypeMeta: v1.TypeMeta{
				Kind:       "Event",
				APIVersion: "v1",
			},
			ObjectMeta: v1.ObjectMeta{
				CreationTimestamp: v1.NewTime(creationTime),
				Name:              "TestHandleWatchEventEvent",
				Namespace:         "TestHandleWatchEventNamespace",
				ResourceVersion:   "37478",
				UID:               "b005830c-a23a-4df8-ac8b-0c866f2f9b09",
			},
			InvolvedObject: corev1.ObjectReference{
				APIVersion:      "batch/v1",
				Kind:            "CronJob",
				Name:            "cronjob-basic-success",
				Namespace:       "TestHandleWatchEventNamespace",
				ResourceVersion: "33547",
				UID:             "f825de34-6728-474a-a28f-8318de23acc1",
			},
			Reason:  "Fake Reason: TestHandlePodWatchEvent",
			Message: "Fake Message: TestHandleWatchEvent",
			Source: corev1.EventSource{
				Component: "cronjob-controller",
			},
			FirstTimestamp:    v1.NewTime(firstTime),
			LastTimestamp:     v1.NewTime(lastTime),
			Count:             12,
			Type:              "Warning",
			ReportingInstance: "",
		},
	}

	// The function that is tested
	// in which it should capture the mock event
	// and produce a Sentry event with the correct
	// message and corresponding Sentry tags

	// Create cutoff time to be before the event time so the function should
	// capture the event
	cutoffTime, _ := time.Parse("2006-01-02 15:04:05", "2023-10-15 01:04:04")
	handleWatchEvent(ctx, &mockEvent, v1.NewTime(cutoffTime))

	// Only a single event should be created
	expectedNumEvents := 1
	events := transport.Events()
	if len(events) != 1 {
		t.Errorf("received %d events, expected %d event", len(events), expectedNumEvents)
	}

	// The Sentry event message should match that of the event message
	expectedMsg := "Fake Message: TestHandleWatchEvent"
	if events[0].Message != expectedMsg {
		t.Errorf("received %s, wanted %s", events[0].Message, expectedMsg)
	}

	// Check that the tags of event are set correctly by the innermost scope
	// corresponding to the creation of the Sentry event
	expectedTags := map[string]string{
		"cronjob_name":           "cronjob-basic-success",
		"event_source_component": "cronjob-controller",
		"watcher_name":           "events",
		"event_type":             "Warning",
		"reason":                 "Fake Reason: TestHandlePodWatchEvent",
		"kind":                   "CronJob",
		"object_uid":             "f825de34-6728-474a-a28f-8318de23acc1",
		"namespace":              "TestHandleWatchEventNamespace"}

	// The test fails if any tag key, value pair does not match
	for key, val := range expectedTags {
		if events[0].Tags[key] != expectedTags[key] {
			t.Errorf("For Sentry tag with key [%s], received \"%s\", wanted \"%s\"", key, events[0].Tags[key], val)
		}
	}
}
