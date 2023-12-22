package main

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
	jobInformer, err = createJobInformer(ctx, factory)
	if err != nil {
		return err
	}
	// Create the cronjob informer
	cronjobInformer, err = createCronjobInformer(ctx, factory)
	if err != nil {
		return err
	}
	// Create the replicaset informer
	replicasetInformer, err = createReplicasetInformer(ctx, factory)
	if err != nil {
		return err
	}
	// Create the deployment informer
	deploymentInformer, err = createDeploymentInformer(ctx, factory)
	if err != nil {
		return err
	}

	// Channel to tell the factory to stop the informers
	doneChan := make(chan struct{})
	factory.Start(doneChan)

	// Sync the cronjob informer cache
	if ok := cache.WaitForCacheSync(doneChan, cronjobInformer.HasSynced); !ok {
		return errors.New("cronjob informer failed to sync")
	}
	// Sync the job informer cache
	if ok := cache.WaitForCacheSync(doneChan, jobInformer.HasSynced); !ok {
		return errors.New("job informer failed to sync")
	}
	// Sync the replicaset informer cache
	if ok := cache.WaitForCacheSync(doneChan, replicasetInformer.HasSynced); !ok {
		return errors.New("replicaset informer failed to sync")
	}
	// Sync the deployment informer cache
	if ok := cache.WaitForCacheSync(doneChan, deploymentInformer.HasSynced); !ok {
		return errors.New("deployment informer failed to sync")
	}

	// Wait for the channel to be closed
	<-doneChan

	return nil
}
