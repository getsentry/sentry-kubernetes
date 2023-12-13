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

func (d *DsnClientMapping) GetClientFromObject(ctx context.Context, objectMeta *metav1.ObjectMeta, clientOptions sentry.ClientOptions) (*sentry.Client, bool) {

	// If the custom DSN flag is set to false
	// then avoid searching for the custom DSN
	// or adding an alternative client and instead
	// just return nil as the client
	if !d.customDsnFlag {
		return nil, false
	}

	// Find DSN annotation from the object
	altDsn, err := searchDsn(ctx, objectMeta)
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
func searchDsn(ctx context.Context, object *metav1.ObjectMeta) (string, error) {

	dsn, ok := object.Annotations[DSNAnnotation]
	if ok {
		return dsn, nil
	}

	if len(object.OwnerReferences) == 0 {
		return "", nil
	}

	owningRef := object.OwnerReferences[0]
	owningObjectMeta, ok := findObjectMeta(ctx, owningRef.Kind, object.Namespace, owningRef.Name)

	if !ok {
		return "", errors.New("the DSN cannot be found")
	}

	return searchDsn(ctx, owningObjectMeta)
}

func findObjectMeta(ctx context.Context, kind string, namespace string, name string) (*metav1.ObjectMeta, bool) {

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
		return &pod.ObjectMeta, true
	case "ReplicaSet":
		replicaSet, err := clientset.AppsV1().ReplicaSets(namespace).Get(context.Background(), name, metav1.GetOptions{})
		if err != nil {
			return nil, false
		}
		return &replicaSet.ObjectMeta, true
	case "Deployment":
		deployment, err := clientset.AppsV1().Deployments(namespace).Get(context.Background(), name, metav1.GetOptions{})
		if err != nil {
			return nil, false
		}
		return &deployment.ObjectMeta, true
	case "Job":
		job, err := clientset.BatchV1().Jobs(namespace).Get(context.Background(), name, metav1.GetOptions{})
		if err != nil {
			return nil, false
		}
		return &job.ObjectMeta, true
	case "CronJob":
		cronjob, err := clientset.BatchV1().CronJobs(namespace).Get(context.Background(), name, metav1.GetOptions{})
		if err != nil {
			return nil, false
		}
		return &cronjob.ObjectMeta, true
	default:
		return nil, false
	}
}
