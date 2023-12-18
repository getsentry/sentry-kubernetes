package main

import (
	"context"

	"github.com/rs/zerolog"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/tools/cache"
)

func createDeploymentInformer(ctx context.Context, factory informers.SharedInformerFactory, namespace string) (cache.SharedIndexInformer, error) {

	logger := zerolog.Ctx(ctx)

	logger.Debug().Msgf("starting deployment informer\n")

	jobInformer := factory.Apps().V1().Deployments().Informer()

	return jobInformer, nil
}
