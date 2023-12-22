package main

import (
	"context"
	"reflect"
	"sort"
	"testing"

	"github.com/getsentry/sentry-go"
	v1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestRunEnhancers(t *testing.T) {
	// Create empty context
	ctx := context.Background()
	// Create simple fake client
	fakeClientset := fake.NewSimpleClientset()

	var replicas int32 = 3
	replicasetObj := &v1.ReplicaSet{
		TypeMeta: metav1.TypeMeta{
			Kind: "ReplicaSet",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "TestRunPodEnhancerReplicaset",
			Namespace: "TestRunPodEnhancerNamespace",
			UID:       "218fc5a9-a5f1-4b54-aa05-46717d0ab26d",
		},
		Spec: v1.ReplicaSetSpec{
			Replicas: &replicas,
		},
		Status: v1.ReplicaSetStatus{
			Replicas: replicas,
		},
	}

	// Create pod object with an error status
	podObj := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "TestRunPodEnhancerPod",
			Namespace: "TestRunPodEnhancerNamespace",
			UID:       "123g4e1p-a591-5b50-aa28-12345d0ab26f",
			OwnerReferences: []metav1.OwnerReference{
				{
					Kind: "ReplicaSet",
					Name: "TestRunPodEnhancerReplicaset",
					UID:  "218fc5a9-a5f1-4b54-aa05-46717d0ab26d",
				},
			},
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
	_, err := fakeClientset.AppsV1().ReplicaSets("TestRunPodEnhancerNamespace").Create(context.TODO(), replicasetObj, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("Error injecting replicaset add: %v", err)
	}
	_, err = fakeClientset.CoreV1().Pods("TestRunPodEnhancerNamespace").Create(context.TODO(), podObj, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("error injecting pod add: %v", err)
	}
	ctx = setClientsetOnContext(ctx, fakeClientset)

	// Create empty scope
	scope := sentry.NewScope()
	// Create empty event
	event := sentry.NewEvent()
	// Add event message
	event.Message = "This event is for TestRunPodEnhancer"
	// Call pod enhancer to modify scope and event
	err = runEnhancers(ctx, nil, KindPod, podObj, scope, event)
	if err != nil {
		t.Errorf("runEnhancers returned an error: %v", err)
	}

	// Apply the scope to the event
	// so we can check the tags
	scope.ApplyToEvent(event, nil)

	expectedTags := map[string]string{
		"node_name": "TestRunPodEnhancerNode",
	}

	// The test fails if any tag key, value pair does not match
	for key, val := range expectedTags {
		if event.Tags[key] != expectedTags[key] {
			t.Errorf("For Sentry tag with key [%s], received \"%s\", wanted \"%s\"", key, event.Tags[key], val)
		}
	}
	expectedFingerprints := []string{
		"This event is for TestRunPodEnhancer",
		"ReplicaSet",
		"TestRunPodEnhancerReplicaset",
	}

	// This helps ensure that the enhancer has all the necessary fingerprint
	// strings (it is important that only replicaset's kind and name is included
	// and not the pod's so all events from the replicaset are grouped together)
	sort.Strings(expectedFingerprints)
	sort.Strings(event.Fingerprint)
	if !reflect.DeepEqual(expectedFingerprints, event.Fingerprint) {
		t.Errorf("The fingerprint is incorrect")
	}

	// Check message is changed to include pod name
	expectedMessage := "TestRunPodEnhancerPod: This event is for TestRunPodEnhancer"
	if event.Message != expectedMessage {
		t.Errorf("For event message, received \"%s\", wanted \"%s\"", event.Message, expectedMessage)
	}
}

func TestFindRootOwner(t *testing.T) {
	// Create empty context
	ctx := context.Background()
	// Create simple fake client
	fakeClientset := fake.NewSimpleClientset()

	// Create pod object with no owning references
	podObj := &corev1.Pod{
		TypeMeta: metav1.TypeMeta{
			Kind: "Pod",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "TestFindRootOwnerPod",
			Namespace: "TestFindRootOwnerNamespace",
		},
		Spec: corev1.PodSpec{
			NodeName: "TestFindRootOwnerNode",
		},
	}

	_, err := fakeClientset.CoreV1().Pods("TestFindRootOwnerNamespace").Create(context.TODO(), podObj, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("Error injecting pod add: %v", err)
	}

	ctx = setClientsetOnContext(ctx, fakeClientset)

	// Check the findRootOwners function does not return a slice
	// with pod which is the object passed in since it has no
	// owning references
	rootOwners, err := findRootOwners(ctx, &KindObjectPair{
		kind:   "Pod",
		object: podObj,
	})
	if err != nil {
		t.Errorf("Function returned error: %#v", err)
	}
	if len(rootOwners) != 0 {
		t.Errorf("Function did not return empty slice as expected")
	}
}

func TestOwnerRefDFS(t *testing.T) {
	// Create empty context
	ctx := context.Background()
	// Create simple fake client
	fakeClientset := fake.NewSimpleClientset()

	// Create pod object with replicaset as owning reference
	podObj := &corev1.Pod{
		TypeMeta: metav1.TypeMeta{
			Kind: "Pod",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "TestOwnerRefDFSPod",
			Namespace: "TestOwnerRefDFSNamespace",
			OwnerReferences: []metav1.OwnerReference{
				{
					Kind: "ReplicaSet",
					Name: "TestOwnerRefDFSReplicaset",
				},
			},
		},
		Spec: corev1.PodSpec{
			NodeName: "TestOwnerRefDFSNode",
		},
	}

	var replicas int32 = 3
	replicasetObj := &v1.ReplicaSet{
		TypeMeta: metav1.TypeMeta{
			Kind: "ReplicaSet",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "TestOwnerRefDFSReplicaset",
			Namespace: "TestOwnerRefDFSNamespace",
		},
		Spec: v1.ReplicaSetSpec{
			Replicas: &replicas,
		},
		Status: v1.ReplicaSetStatus{
			Replicas: replicas,
		},
	}

	_, err := fakeClientset.CoreV1().Pods("TestOwnerRefDFSNamespace").Create(context.TODO(), podObj, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("Error injecting pod add: %v", err)
	}
	_, err = fakeClientset.AppsV1().ReplicaSets("TestOwnerRefDFSNamespace").Create(context.TODO(), replicasetObj, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("Error injecting replicaset add: %v", err)
	}

	ctx = setClientsetOnContext(ctx, fakeClientset)

	rootOwners, err := ownerRefDFS(ctx, &KindObjectPair{
		kind:   podObj.Kind,
		object: podObj,
	})
	if err != nil {
		t.Errorf("the DFS produced an error: %#v", err)
	}
	if rootOwners == nil {
		t.Errorf("Failed to return a slice of root owners: %#v", err)
		return
	}
	if len(rootOwners) != 1 {
		t.Errorf("Failed to produce correct number of root owners")
		return
	}
	if rootOwners[0].kind != replicasetObj.Kind {
		t.Errorf("The root owner's kind is incorrect")
	}
	if rootOwners[0].object.GetName() != replicasetObj.Name {
		t.Errorf("The root owner's object is incorrect")
	}
}
