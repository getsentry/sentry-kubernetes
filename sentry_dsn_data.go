package main

import (
	"context"
	"errors"
	"sync"

	"github.com/getsentry/sentry-go"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var DSN = "dsn"

// map from Sentry DSN to Client
type DsnData struct {
	mutex     sync.RWMutex
	clientMap map[string]*sentry.Client
}

func NewDsnData() *DsnData {
	return &DsnData{
		mutex:     sync.RWMutex{},
		clientMap: make(map[string]*sentry.Client),
	}
}

// return client if added successfully
func (d *DsnData) AddClient(dsn string) (*sentry.Client, error) {
	d.mutex.Lock()
	defer d.mutex.Unlock()

	// check if we already encountered this dsn
	existingClient, ok := d.clientMap[dsn]
	if ok {
		return existingClient, errors.New("a client with the given dsn already exists")
	}

	// create a new client for the dsn
	newClient, err := sentry.NewClient(
		sentry.ClientOptions{
			Dsn:              dsn,
			Debug:            true,
			AttachStacktrace: true,
		},
	)
	if err != nil {
		return nil, err
	}
	d.clientMap[dsn] = newClient

	return newClient, nil
}

// retrieve a client with given dsn
func (d *DsnData) GetClient(dsn string) (*sentry.Client, error) {
	d.mutex.RLock()
	defer d.mutex.RUnlock()

	// check if we have this dsn
	existingClient, ok := d.clientMap[dsn]
	if ok {
		return existingClient, nil
	} else {
		return nil, errors.New("a client with given DSN does not exist")
	}
}

// recursive function to find if there is a DSN annotation
func searchDsn(ctx context.Context, object metav1.ObjectMeta) (string, error) {

	clientset, err := getClientsetFromContext(ctx)
	if err != nil {
		return "", err
	}

	dsn, ok := object.Annotations[DSN]
	if ok {
		return dsn, nil
	}

	if len(object.OwnerReferences) == 0 {
		return "", nil
	}

	owningRef := object.OwnerReferences[0]
	switch kind := owningRef.Kind; kind {
	case "Pod":
		parentPod, err := clientset.CoreV1().Pods(object.Namespace).Get(context.Background(), owningRef.Name, metav1.GetOptions{})
		if err != nil {
			return "", err
		}
		return searchDsn(ctx, parentPod.ObjectMeta)
	case "ReplicaSet":
		parentReplicaSet, err := clientset.AppsV1().ReplicaSets(object.Namespace).Get(context.Background(), owningRef.Name, metav1.GetOptions{})
		if err != nil {
			return "", err
		}
		return searchDsn(ctx, parentReplicaSet.ObjectMeta)
	case "Deployment":
		parentDeployment, err := clientset.AppsV1().Deployments(object.Namespace).Get(context.Background(), owningRef.Name, metav1.GetOptions{})
		if err != nil {
			return "", err
		}
		return searchDsn(ctx, parentDeployment.ObjectMeta)
	case "Job":
		parentJob, err := clientset.BatchV1().Jobs(object.Namespace).Get(context.Background(), owningRef.Name, metav1.GetOptions{})
		if err != nil {
			return "", err
		}
		return searchDsn(ctx, parentJob.ObjectMeta)
	case "CronJob":
		parentCronjob, err := clientset.BatchV1().CronJobs(object.Namespace).Get(context.Background(), owningRef.Name, metav1.GetOptions{})
		if err != nil {
			return "", err
		}
		return searchDsn(ctx, parentCronjob.ObjectMeta)
	default:
		return "", errors.New("unsupported object kind encountered")
	}
}

func findObjectMeta(ctx context.Context, kind string, namespace string, name string) (*metav1.ObjectMeta, error) {

	clientset, err := getClientsetFromContext(ctx)
	if err != nil {
		return nil, err
	}

	switch kind {
	case "Pod":
		parentPod, err := clientset.CoreV1().Pods(namespace).Get(context.Background(), name, metav1.GetOptions{})
		if err != nil {
			return nil, err
		}
		return &parentPod.ObjectMeta, nil
	case "ReplicaSet":
		parentReplicaSet, err := clientset.AppsV1().ReplicaSets(namespace).Get(context.Background(), name, metav1.GetOptions{})
		if err != nil {
			return nil, err
		}
		return &parentReplicaSet.ObjectMeta, nil
	case "Deployment":
		parentDeployment, err := clientset.AppsV1().Deployments(namespace).Get(context.Background(), name, metav1.GetOptions{})
		if err != nil {
			return nil, err
		}
		return &parentDeployment.ObjectMeta, nil
	case "Job":
		parentJob, err := clientset.BatchV1().Jobs(namespace).Get(context.Background(), name, metav1.GetOptions{})
		if err != nil {
			return nil, err
		}
		return &parentJob.ObjectMeta, nil
	case "CronJob":
		parentCronjob, err := clientset.BatchV1().CronJobs(namespace).Get(context.Background(), name, metav1.GetOptions{})
		if err != nil {
			return nil, err
		}
		return &parentCronjob.ObjectMeta, nil
	default:
		return nil, errors.New("unsupported object kind encountered")
	}
}
