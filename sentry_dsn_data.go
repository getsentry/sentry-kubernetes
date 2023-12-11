package main

import (
	"context"
	"errors"
	"sync"

	"github.com/getsentry/sentry-go"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var DSNAnnotation = "k8s.sentry.io/dsn"

// map from Sentry DSN to Client
type DsnClientMapping struct {
	mutex     sync.RWMutex
	clientMap map[string]*sentry.Client
}

func NewDsnData() *DsnClientMapping {
	return &DsnClientMapping{
		mutex:     sync.RWMutex{},
		clientMap: make(map[string]*sentry.Client),
	}
}

// return client if added successfully
// (also returns client if already exists)
func (d *DsnClientMapping) AddClientToMap(options sentry.ClientOptions) (*sentry.Client, error) {
	d.mutex.Lock()
	defer d.mutex.Unlock()

	// check if we already encountered this dsn
	existingClient, ok := d.clientMap[options.Dsn]
	if ok {
		return existingClient, nil
	}

	// create a new client for the dsn
	newClient, err := sentry.NewClient(
		sentry.ClientOptions{
			Dsn:              options.Dsn,
			Debug:            true,
			AttachStacktrace: true,
		},
	)
	if err != nil {
		return nil, err
	}
	d.clientMap[options.Dsn] = newClient

	return newClient, nil
}

// retrieve a client with given dsn
func (d *DsnClientMapping) GetClientFromMap(dsn string) (*sentry.Client, bool) {
	d.mutex.RLock()
	defer d.mutex.RUnlock()

	// check if we have this dsn
	existingClient, ok := d.clientMap[dsn]
	if ok {
		return existingClient, true
	} else {
		return nil, false
	}
}

func (d *DsnClientMapping) GetClientFromObject(ctx context.Context, objectMeta *metav1.ObjectMeta, clientOptions sentry.ClientOptions) (*sentry.Client, bool) {

	// find DSN annotation from the object
	altDsn, err := searchDsn(ctx, objectMeta)
	if err != nil {
		return nil, false
	}

	// if we did find an alternative DSN
	if altDsn != "" {
		// attempt to retrieve the corresponding client
		client, _ := dsnData.GetClientFromMap(altDsn)
		if client == nil {
			// create new client
			clientOptions.Dsn = altDsn
			client, err = dsnData.AddClientToMap(clientOptions)
			if err != nil {
				return nil, false
			}
		}
		return client, true
	} else {
		return nil, false
	}
}

// recursive function to find if there is a DSN annotation
func searchDsn(ctx context.Context, object *metav1.ObjectMeta) (string, error) {

	dsn, ok := object.Annotations[DSNAnnotation]
	if ok {
		return dsn, nil
	}

	if len(object.OwnerReferences) == 0 {
		return "", nil
	}

	owningRef := object.OwnerReferences[0]
	owningObjectMeta, err := findObjectMeta(ctx, owningRef.Kind, object.Namespace, owningRef.Name)

	if err != nil {
		return "", err
	}

	return searchDsn(ctx, owningObjectMeta)
}

func findObjectMeta(ctx context.Context, kind string, namespace string, name string) (*metav1.ObjectMeta, error) {

	clientset, err := getClientsetFromContext(ctx)
	if err != nil {
		return nil, err
	}

	switch kind {
	case "Pod":
		pod, err := clientset.CoreV1().Pods(namespace).Get(context.Background(), name, metav1.GetOptions{})
		if err != nil {
			return nil, err
		}
		return &pod.ObjectMeta, nil
	case "ReplicaSet":
		replicaSet, err := clientset.AppsV1().ReplicaSets(namespace).Get(context.Background(), name, metav1.GetOptions{})
		if err != nil {
			return nil, err
		}
		return &replicaSet.ObjectMeta, nil
	case "Deployment":
		deployment, err := clientset.AppsV1().Deployments(namespace).Get(context.Background(), name, metav1.GetOptions{})
		if err != nil {
			return nil, err
		}
		return &deployment.ObjectMeta, nil
	case "Job":
		job, err := clientset.BatchV1().Jobs(namespace).Get(context.Background(), name, metav1.GetOptions{})
		if err != nil {
			return nil, err
		}
		return &job.ObjectMeta, nil
	case "CronJob":
		cronjob, err := clientset.BatchV1().CronJobs(namespace).Get(context.Background(), name, metav1.GetOptions{})
		if err != nil {
			return nil, err
		}
		return &cronjob.ObjectMeta, nil
	default:
		return nil, errors.New("unsupported object kind encountered")
	}
}
