package main

import (
	"context"
	"testing"

	"github.com/getsentry/sentry-go"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestRunPodEnhancer(t *testing.T) {

	// Create empty context
	ctx := context.Background()
	// Create simple fake client
	fakeClientset := fake.NewSimpleClientset()
	// Create pod object with an error status
	podObj := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "TestRunPodEnhancerPod",
			Namespace: "TestRunPodEnhancerNamespace",
		},
		Spec: corev1.PodSpec{
			NodeName: "TestRunPodEnhancerNode",
		},
		Status: corev1.PodStatus{
			ContainerStatuses: []corev1.ContainerStatus{
				{
					Name: "FakeDnsLabel",
					State: corev1.ContainerState{
						Terminated: &corev1.ContainerStateTerminated{
							ExitCode: 1,
							Reason:   "Fake Reason: TestRunPodEnhancerEvent",
							Message:  "Fake Message: TestRunPodEnhancerEvent",
						},
					},
				},
			},
		},
	}
	_, err := fakeClientset.CoreV1().Pods("TestRunPodEnhancerNamespace").Create(context.TODO(), podObj, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("error injecting pod add: %v", err)
	}
	ctx = setClientsetOnContext(ctx, fakeClientset)
	objRef := &corev1.ObjectReference{
		Name:      "TestRunPodEnhancerPod",
		Namespace: "TestRunPodEnhancerNamespace",
	}

	// Create empty scope
	scope := sentry.NewScope()
	// Create empty event
	event := sentry.NewEvent()
	// Add event message
	event.Message = "This event is for TestRunPodEnhancer"
	// Call pod enhancer to modify scope and event
	err = runPodEnhancer(ctx, objRef, nil, scope, event)
	if err != nil {
		t.Errorf("pod enhancer returned an error: %v", err)
	}

	// Apply the scope to the event
	// so we can check the tags
	scope.ApplyToEvent(event, nil)

	expectedTags := map[string]string{
		"node_name": "TestRunPodEnhancerNode",
	}
	// the test fails if any tag key, value pair does not match
	for key, val := range expectedTags {
		if event.Tags[key] != expectedTags[key] {
			t.Errorf("For Sentry tag with key [%s], received \"%s\", wanted \"%s\"", key, event.Tags[key], val)
		}
	}
	expectedFingerprints := []string{
		"This event is for TestRunPodEnhancer",
		"TestRunPodEnhancerPod",
	}

	// The test fails if any tag key, value pair does not match
	var found bool
	for _, expectedFingerprint := range expectedFingerprints {
		found = false
		for _, fingerprint := range event.Fingerprint {
			if expectedFingerprint == fingerprint {
				found = true
			}
		}
		if !found {
			t.Errorf("The fingerprint slice does not contain the expected fingerprint: %s", expectedFingerprint)
		}
	}

	// Check message is changed to include pod name
	expectedMessage := "TestRunPodEnhancerPod: This event is for TestRunPodEnhancer"
	if event.Message != expectedMessage {
		t.Errorf("For event message, received \"%s\", wanted \"%s\"", event.Message, expectedMessage)
	}
}
