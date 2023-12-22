package main

import (
	"context"
	"reflect"
	"testing"

	"github.com/getsentry/sentry-go"
	v1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestNewDsnClientMapping(t *testing.T) {
	// Set the custom dsn flag as true
	t.Setenv("SENTRY_K8S_CUSTOM_DSNS", "TRUE")
	clientMapping := NewDsnClientMapping()
	if clientMapping.clientMap == nil {
		t.Errorf("Failed to initialize client mapping")
	}
	if !clientMapping.customDsnFlag {
		t.Errorf("Failed to read custom dsn flag as true")
	}

	// set the custom dsn flag as false
	t.Setenv("SENTRY_K8S_CUSTOM_DSNS", "")
	clientMapping = NewDsnClientMapping()
	if clientMapping.customDsnFlag {
		t.Errorf("Failed to read custom dsn flag as false")
	}
}

func TestAddClientToMap(t *testing.T) {
	fakeDsn := "https://c6f9a148ee0775891414b50b9af35959@o4506191942320128.ingest.sentry.io/1234567890"
	clientOptions := sentry.ClientOptions{
		Dsn: fakeDsn,
	}
	clientMapping := NewDsnClientMapping()

	// Add the dsn for the first time
	_, err := clientMapping.AddClientToMap(clientOptions)
	if err != nil {
		t.Errorf("Failed to add client to map")
	}
	firstClient, ok := clientMapping.clientMap[fakeDsn]
	if !ok {
		t.Errorf("Failed to add the fake dsn to the client map")
	}
	if firstClient.Options().Dsn != fakeDsn {
		t.Errorf("The DSN in the added client is incorrect")
	}

	// Try to add the same dsn again, which should create a new client
	_, err = clientMapping.AddClientToMap(clientOptions)
	if err != nil {
		t.Errorf("Failed to add client to map")
	}
	secondClient, ok := clientMapping.clientMap[fakeDsn]
	if !ok {
		t.Errorf("Failed to add client if dsn already exists in the map")
	}
	if firstClient == secondClient {
		t.Errorf("Failed to create new client if the dsn already exists")
	}
}

func TestGetClientFromMap(t *testing.T) {
	fakeDsn := "https://c6f9a148ee0775891414b50b9af35959@o4506191942320128.ingest.sentry.io/1234567890"
	clientOptions := sentry.ClientOptions{
		Dsn: fakeDsn,
	}
	clientMapping := NewDsnClientMapping()

	// Add the DSN and its corresponding client
	_, err := clientMapping.AddClientToMap(clientOptions)
	if err != nil {
		t.Errorf("Failed to add client to map")
	}
	// Test function to retrieve the client with DSN
	client, ok := clientMapping.GetClientFromMap(fakeDsn)
	if !ok {
		t.Errorf("Failed to retrieve a client")
	}
	if client.Options().Dsn != fakeDsn {
		t.Errorf("Failed to retrieve client with correct DSN")
	}
}

func TestGetClientFromObject(t *testing.T) {
	// Set the custom dsn flag as true
	t.Setenv("SENTRY_K8S_CUSTOM_DSNS", "TRUE")
	clientMapping := NewDsnClientMapping()
	fakeDsn := "https://c6f9a148ee0775891414b50b9af35959@o4506191942320128.ingest.sentry.io/1234567890"

	// Create empty context
	ctx := context.Background()
	// Create simple fake client
	fakeClientset := fake.NewSimpleClientset()

	// Create annotation map that includes the DSN
	annotations := make(map[string]string)
	annotations["k8s.sentry.io/dsn"] = fakeDsn

	// Create replicaset object with DSN annotation
	var replicas int32 = 3
	replicasetObj := &v1.ReplicaSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "TestSearchDsnReplicaset",
			Namespace:   "TestSearchDsnNamespace",
			Annotations: annotations,
		},
		Spec: v1.ReplicaSetSpec{
			Replicas: &replicas,
		},
		Status: v1.ReplicaSetStatus{
			Replicas: replicas,
		},
	}

	// Add the replicaset object to the fake clientset
	_, err := fakeClientset.AppsV1().ReplicaSets("TestSearchDsnNamespace").Create(context.TODO(), replicasetObj, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("Error injecting replicaset add: %v", err)
	}
	ctx = setClientsetOnContext(ctx, fakeClientset)

	// Create client options with DSN
	clientOptions := sentry.ClientOptions{
		Dsn: fakeDsn,
	}

	// Test function to retrieve client from object using DSN
	firstClient, ok := clientMapping.GetClientFromObject(ctx, replicasetObj, clientOptions)
	if !ok {
		t.Errorf("The function failed to create and retrieve new client from object")
	}
	if firstClient.Options().Dsn != fakeDsn {
		t.Errorf("The retrieved client has the incorrect DSN")
	}
	secondClient, ok := clientMapping.GetClientFromObject(ctx, replicasetObj, clientOptions)
	if !ok {
		t.Errorf("The function failed to retrieve existing client from object")
	}
	if !reflect.DeepEqual(firstClient, secondClient) {
		t.Errorf("The function failed to retrieve the client it originally created")
	}
}

func TestSearchDsn(t *testing.T) {
	// Create empty context
	ctx := context.Background()
	// Create simple fake client
	fakeClientset := fake.NewSimpleClientset()

	fakeDsn := "https://c6f9a148ee0775891414b50b9af35959@o4506191942320128.ingest.sentry.io/1234567890"

	// Create annotation map that includes the DSN
	annotations := make(map[string]string)
	annotations["k8s.sentry.io/dsn"] = fakeDsn

	// Create pod object with replicaset as owning reference
	podObj := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "TestSearchDsnPod",
			Namespace: "TestSearchDsnNamespace",
			OwnerReferences: []metav1.OwnerReference{
				{
					Kind: "ReplicaSet",
					Name: "TestSearchDsnReplicaset",
				},
			},
		},
		Spec: corev1.PodSpec{
			NodeName: "TestSearchDsnNode",
		},
	}

	var replicas int32 = 3
	replicasetObj := &v1.ReplicaSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "TestSearchDsnReplicaset",
			Namespace:   "TestSearchDsnNamespace",
			Annotations: annotations,
		},
		Spec: v1.ReplicaSetSpec{
			Replicas: &replicas,
		},
		Status: v1.ReplicaSetStatus{
			Replicas: replicas,
		},
	}

	_, err := fakeClientset.CoreV1().Pods("TestSearchDsnNamespace").Create(context.TODO(), podObj, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("Error injecting pod add: %v", err)
	}
	_, err = fakeClientset.AppsV1().ReplicaSets("TestSearchDsnNamespace").Create(context.TODO(), replicasetObj, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("Error injecting replicaset add: %v", err)
	}

	ctx = setClientsetOnContext(ctx, fakeClientset)

	// The replicaset contains the annotation for the DSN
	replicasetDsn, err := searchDsn(ctx, replicasetObj)
	if err != nil {
		t.Error(err)
		t.Errorf("Failed to find replicaset's owning object's DSN")
	}
	if replicasetDsn != fakeDsn {
		t.Errorf("DSN expected: %s, actual: %s", fakeDsn, replicasetDsn)
	}

	// The pod object does not itself contain the DSN annotation
	// but its owning object (replicaset) contains the DSN
	podDsn, err := searchDsn(ctx, podObj)
	if err != nil {
		t.Error(err)
		t.Errorf("Failed to find pod's owning object's DSN")
	}
	if podDsn != fakeDsn {
		t.Errorf("DSN expected: %s, actual: %s", fakeDsn, podDsn)
	}
}

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
