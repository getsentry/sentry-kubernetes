package main

import (
	"context"
	"reflect"
	"testing"

	v1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestFindObject(t *testing.T) {

	// Create empty context
	ctx := context.Background()
	// Create simple fake client
	fakeClientset := fake.NewSimpleClientset()

	// Create pod object with an error status
	podObj := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "TestFindObjectPod",
			Namespace: "TestFindObjectNamespace",
		},
		Spec: corev1.PodSpec{
			NodeName: "TestFindObjectNode",
		},
		Status: corev1.PodStatus{
			ContainerStatuses: []corev1.ContainerStatus{
				{
					Name: "FakeDnsLabel",
					State: corev1.ContainerState{
						Terminated: &corev1.ContainerStateTerminated{
							ExitCode: 1,
							Reason:   "Fake Reason: TestFindObjectEvent",
							Message:  "Fake Message: TestFindObjectEvent",
						},
					},
				},
			},
		},
	}

	var replicas int32 = 3
	replicasetObj := &v1.ReplicaSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "TestFindObjectReplicaset",
			Namespace: "TestFindObjectNamespace",
		},
		Spec: v1.ReplicaSetSpec{
			Replicas: &replicas,
		},
		Status: v1.ReplicaSetStatus{
			Replicas: replicas,
		},
	}

	_, err := fakeClientset.CoreV1().Pods("TestFindObjectNamespace").Create(context.TODO(), podObj, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("Error injecting pod add: %v", err)
	}
	_, err = fakeClientset.AppsV1().ReplicaSets("TestFindObjectNamespace").Create(context.TODO(), replicasetObj, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("Error injecting replicaset add: %v", err)
	}

	ctx = setClientsetOnContext(ctx, fakeClientset)

	// test function to find the pod object
	retObj, ok := findObject(ctx, KindPod, "TestFindObjectNamespace", "TestFindObjectPod")
	if !ok {
		t.Errorf("The function findObject was not able to find the pod object")
	}
	retPod, ok := retObj.(*corev1.Pod)
	if !ok {
		t.Errorf("The object returned is not of the Pod type")
	}
	if !reflect.DeepEqual(podObj, retPod) {
		t.Errorf("The returned pod is not equal to the original pod")
	}

	// test function to find the replicaset object
	retObj, ok = findObject(ctx, KindReplicaset, "TestFindObjectNamespace", "TestFindObjectReplicaset")
	if !ok {
		t.Errorf("The function findObject was not able to find the pod object")
	}
	retReplicaset, ok := retObj.(*v1.ReplicaSet)
	if !ok {
		t.Errorf("The object returned is not of the replicaset type")
	}
	if !reflect.DeepEqual(replicasetObj, retReplicaset) {
		t.Errorf("The returned replicaset is not equal to the original replicaset")
	}
}
