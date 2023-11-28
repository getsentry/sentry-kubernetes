package main

import (
	"context"
	"errors"

	"github.com/rs/zerolog"
	batchv1 "k8s.io/api/batch/v1"

	"k8s.io/client-go/informers"
	"k8s.io/client-go/tools/cache"
)

func createJobInformer(ctx context.Context, factory informers.SharedInformerFactory, namespace string) (cache.SharedIndexInformer, error) {

	logger := zerolog.Ctx(ctx)

	logger.Debug().Msgf("starting job informer\n")

	val := ctx.Value(CronsInformerDataKey{})
	if val == nil {
		return nil, errors.New("no crons informer data struct given")
	}

	jobInformer := factory.Batch().V1().Jobs().Informer()

	var handler cache.ResourceEventHandlerFuncs

	handler.AddFunc = func(obj interface{}) {
		job := obj.(*batchv1.Job)
		logger.Debug().Msgf("ADD: Job Added to Store: %s\n", job.GetName())
		err := runSentryCronsCheckin(ctx, job, EventHandlerAdd)
		if err != nil {
			return
		}
	}

	handler.UpdateFunc = func(oldObj, newObj interface{}) {

		oldJob := oldObj.(*batchv1.Job)
		newJob := newObj.(*batchv1.Job)

		if oldJob.ResourceVersion == newJob.ResourceVersion {
			logger.Debug().Msgf("UPDATE: Event sync %s/%s\n", oldJob.GetNamespace(), oldJob.GetName())
		} else {
			runSentryCronsCheckin(ctx, newJob, EventHandlerUpdate)
		}
	}

	handler.DeleteFunc = func(obj interface{}) {
		job := obj.(*batchv1.Job)
		logger.Debug().Msgf("DELETE: Job deleted from Store: %s\n", job.GetName())
		err := runSentryCronsCheckin(ctx, job, EventHandlerDelete)
		if err != nil {
			return
		}
	}

	jobInformer.AddEventHandler(handler)

	return jobInformer, nil
}
