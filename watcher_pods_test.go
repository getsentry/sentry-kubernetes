package main

import (
	"context"
	"testing"

	"github.com/getsentry/sentry-go"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
)

// test the function handlePodWatchEvent
// by giving it a mock pod with a status
// and checking it causes Sentry to capture the event
// with the correct message and Sentry tags
func TestHandlePodWatchEvent(t *testing.T) {

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

	// Create the watch event which includes the mock pod
	// where the pod includes statuses to create Sentry events from
	mockEvent := watch.Event{
		Type: watch.Modified,
		Object: &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "TestHandlePodWatchEventPod",
				Namespace: "TestHandlePodWatchEventNameSpace",
			},
			Status: corev1.PodStatus{
				ContainerStatuses: []corev1.ContainerStatus{
					{
						Name: "fake_DNS_Label",
						State: corev1.ContainerState{
							Terminated: &corev1.ContainerStateTerminated{
								ExitCode: 1,
								Reason:   "Fake Reason: TestHandlePodWatchEvent",
								Message:  "Fake Message: TestHandlePodWatchEvent",
							},
						},
					},
				},
			},
		},
	}

	// The function that is tested
	// in which it should capture the mock event
	// and produce a Sentry event with the correct
	// message and corresponding Sentry tags
	handlePodWatchEvent(ctx, &mockEvent)

	// Only a single event should be created corresponding
	// to the single container status in the mock pod
	expectedNumEvents := 1
	events := transport.Events()
	if len(events) != 1 {
		t.Errorf("received %d events, expected %d event", len(events), expectedNumEvents)
	}

	// the Sentry event message should match that of the container status
	expectedMsg := "Fake Message: TestHandlePodWatchEvent"
	if events[0].Message != expectedMsg {
		t.Errorf("received %s, wanted %s", events[0].Message, expectedMsg)
	}

	// Check that the tags of event are set correctly by the innermost scope
	// corresponding to the creation of the Sentry event
	expectedTags := map[string]string{"container_name": "fake_DNS_Label",
		"event_source_component": "x-pod-controller",
		"namespace":              "TestHandlePodWatchEventNameSpace",
		"pod_name":               "TestHandlePodWatchEventPod",
		"reason":                 "Fake Reason: TestHandlePodWatchEvent",
		"watcher_name":           "pods"}
	// the test fails if any tag key, value pair does not match
	for key, val := range expectedTags {
		if events[0].Tags[key] != expectedTags[key] {
			t.Errorf("For Sentry tag with key [%s], received \"%s\", wanted \"%s\"", key, events[0].Tags[key], val)
		}
	}
}
