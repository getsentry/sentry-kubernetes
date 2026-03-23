package agent

import (
	"context"
	"errors"
	"time"

	"k8s.io/client-go/informers"
	"k8s.io/client-go/tools/cache"
)

var cronjobInformer cache.SharedIndexInformer
var jobInformer cache.SharedIndexInformer
var replicasetInformer cache.SharedIndexInformer
var deploymentInformer cache.SharedIndexInformer

// Starts all informers (jobs, cronjobs, replicasets, deployments)
// if we opt into cronjob, attach the job/cronjob event handlers
// and add to the crons monitor data struct for Sentry Crons
func startInformers(ctx context.Context, namespace string) error {
	clientset, err := getClientsetFromContext(ctx)
	if err != nil {
		return errors.New("failed to get clientset")
	}

	// Create factory that will produce both the cronjob informer and job informer
	factory := informers.NewSharedInformerFactoryWithOptions(
		clientset,
		5*time.Second,
		informers.WithNamespace(namespace),
	)

	// Create the job informer
	jobInformer = createJobInformer(ctx, factory)
	// Create the cronjob informer
	cronjobInformer = createCronjobInformer(ctx, factory)
	// Create the replicaset informer
	replicasetInformer = createReplicasetInformer(ctx, factory)
	// Create the deployment informer
	deploymentInformer = createDeploymentInformer(ctx, factory)

	// Channel to tell the factory to stop the informers
	stopCh := make(chan struct{})
	go func() {
		<-ctx.Done()
		close(stopCh)
	}()
	factory.Start(stopCh)

	// Sync the informer caches
	if ok := cache.WaitForCacheSync(stopCh, cronjobInformer.HasSynced); !ok {
		return errors.New("cronjob informer failed to sync")
	}
	if ok := cache.WaitForCacheSync(stopCh, jobInformer.HasSynced); !ok {
		return errors.New("job informer failed to sync")
	}
	if ok := cache.WaitForCacheSync(stopCh, replicasetInformer.HasSynced); !ok {
		return errors.New("replicaset informer failed to sync")
	}
	if ok := cache.WaitForCacheSync(stopCh, deploymentInformer.HasSynced); !ok {
		return errors.New("deployment informer failed to sync")
	}

	// Block until context is cancelled
	<-stopCh

	return nil
}
