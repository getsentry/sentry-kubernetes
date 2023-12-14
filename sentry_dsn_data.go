package main

import (
	"context"
	"errors"
	"os"
	"sync"

	"github.com/getsentry/sentry-go"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var DSNAnnotation = "k8s.sentry.io/dsn"

var dsnClientMapping = NewDsnClientMapping()

// Map from Sentry DSN to Client
type DsnClientMapping struct {
	mutex         sync.RWMutex
	clientMap     map[string]*sentry.Client
	customDsnFlag bool
}

func NewDsnClientMapping() *DsnClientMapping {
	return &DsnClientMapping{
		mutex:         sync.RWMutex{},
		clientMap:     make(map[string]*sentry.Client),
		customDsnFlag: isTruthy(os.Getenv("SENTRY_K8S_CUSTOM_DSNS")),
	}
}

// Return client if added successfully
// (also returns client if already exists)
func (d *DsnClientMapping) AddClientToMap(options sentry.ClientOptions) (*sentry.Client, error) {
	d.mutex.Lock()
	defer d.mutex.Unlock()

	// Create a new client for the dsn
	// even if client already exists, it
	// will be re-initialized with a new client
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

// Retrieve a client with given dsn
func (d *DsnClientMapping) GetClientFromMap(dsn string) (*sentry.Client, bool) {
	d.mutex.RLock()
	defer d.mutex.RUnlock()

	// Check if we have this dsn
	existingClient, ok := d.clientMap[dsn]
	return existingClient, ok
}

func (d *DsnClientMapping) GetClientFromObject(ctx context.Context, object metav1.Object, clientOptions sentry.ClientOptions) (*sentry.Client, bool) {

	// If the custom DSN flag is set to false
	// then avoid searching for the custom DSN
	// or adding an alternative client and instead
	// just return nil as the client
	if !d.customDsnFlag {
		return nil, false
	}

	// Find DSN annotation from the object
	altDsn, err := searchDsn(ctx, object)
	if err != nil {
		return nil, false
	}

	// If we did find an alternative DSN
	if altDsn != "" {
		// Attempt to retrieve the corresponding client
		client, _ := dsnClientMapping.GetClientFromMap(altDsn)
		if client == nil {
			// create new client
			clientOptions.Dsn = altDsn
			client, err = dsnClientMapping.AddClientToMap(clientOptions)
			if err != nil {
				return nil, false
			}
		}
		return client, true
	} else {
		return nil, false
	}
}

// Recursive function to find if there is a DSN annotation
func searchDsn(ctx context.Context, obj metav1.Object) (string, error) {

	dsn, ok := obj.GetAnnotations()[DSNAnnotation]
	if ok {
		return dsn, nil
	}

	if len(obj.GetOwnerReferences()) == 0 {
		return "", nil
	}

	owningRef := obj.GetOwnerReferences()[0]
	owningObject, ok := findObject(ctx, owningRef.Kind, obj.GetNamespace(), owningRef.Name)

	if !ok {
		return "", errors.New("the DSN cannot be found")
	}

	return searchDsn(ctx, owningObject)
}

func findObject(ctx context.Context, kind string, namespace string, name string) (metav1.Object, bool) {

	clientset, err := getClientsetFromContext(ctx)
	if err != nil {
		return nil, false
	}

	switch kind {
	case "Pod":
		pod, err := clientset.CoreV1().Pods(namespace).Get(context.Background(), name, metav1.GetOptions{})
		if err != nil {
			return nil, false
		}
		return pod, true
	case "ReplicaSet":
		replicaSet, err := clientset.AppsV1().ReplicaSets(namespace).Get(context.Background(), name, metav1.GetOptions{})
		if err != nil {
			return nil, false
		}
		return replicaSet, true
	case "Deployment":
		deployment, err := clientset.AppsV1().Deployments(namespace).Get(context.Background(), name, metav1.GetOptions{})
		if err != nil {
			return nil, false
		}
		return deployment, true
	case "Job":
		job, err := clientset.BatchV1().Jobs(namespace).Get(context.Background(), name, metav1.GetOptions{})
		if err != nil {
			return nil, false
		}
		return job, true
	case "CronJob":
		cronjob, err := clientset.BatchV1().CronJobs(namespace).Get(context.Background(), name, metav1.GetOptions{})
		if err != nil {
			return nil, false
		}
		return cronjob, true
	default:
		return nil, false
	}
}
