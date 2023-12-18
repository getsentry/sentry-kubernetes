package main

import (
	"context"
	"os"

	"github.com/rs/zerolog"
	batchv1 "k8s.io/api/batch/v1"

	"k8s.io/client-go/informers"
	"k8s.io/client-go/tools/cache"
)

func createCronjobInformer(ctx context.Context, factory informers.SharedInformerFactory, namespace string) (cache.SharedIndexInformer, error) {

	logger := zerolog.Ctx(ctx)

	logger.Debug().Msgf("Starting cronjob informer\n")

	cronjobInformer := factory.Batch().V1().CronJobs().Informer()

	var handler cache.ResourceEventHandlerFuncs

	handler.AddFunc = func(obj interface{}) {
		cronjob := obj.(*batchv1.CronJob)
		logger.Debug().Msgf("ADD: CronJob Added to Store: %s\n", cronjob.GetName())
		_, ok := cronsMetaData.getCronsMonitorData(cronjob.Name)
		if ok {
			logger.Debug().Msgf("cronJob %s already exists in the crons informer data struct...\n", cronjob.Name)
		} else {
			cronsMetaData.addCronsMonitorData(cronjob.Name, NewCronsMonitorData(cronjob.Name, cronjob.Spec.Schedule, cronjob.Spec.JobTemplate.Spec.Completions))
		}
	}

	handler.DeleteFunc = func(obj interface{}) {
		cronjob := obj.(*batchv1.CronJob)
		logger.Debug().Msgf("DELETE: CronJob deleted from Store: %s\n", cronjob.GetName())
		_, ok := cronsMetaData.getCronsMonitorData(cronjob.Name)
		if ok {
			cronsMetaData.deleteCronsMonitorData(cronjob.Name)
			logger.Debug().Msgf("cronJob %s deleted from the crons informer data struct...\n", cronjob.Name)
		} else {
			logger.Debug().Msgf("cronJob %s not in the crons informer data struct...\n", cronjob.Name)
		}
	}

	// Check if cronjob monitoring is enabled
	if isTruthy(os.Getenv("SENTRY_K8S_MONITOR_CRONJOBS")) {
		logger.Info().Msgf("Add cronjob informer handlers for cronjob monitoring")
		cronjobInformer.AddEventHandler(handler)
	} else {
		logger.Info().Msgf("Cronjob monitoring is disabled")
	}

	return cronjobInformer, nil
}
