package main

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	batchv1 "k8s.io/api/batch/v1"

	"k8s.io/client-go/informers"
	"k8s.io/client-go/tools/cache"
)

func startCronJobInformer(ctx context.Context, namespace string) (err error) {
	clientset, err := getClientsetFromContext(ctx)
	if err != nil {
		return err
	}

	factory := informers.NewSharedInformerFactoryWithOptions(
		clientset,
		5*time.Second,
		informers.WithNamespace(namespace),
	)

	// logger := zerolog.Ctx(ctx)
	// logger.Debug().Msgf("Factory %+v", factory)

	informer := factory.Batch().V1().CronJobs().Informer()

	informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			cronjob := obj.(*batchv1.CronJob)
			fmt.Printf("New CronJob Added to Store: %s\n", cronjob.GetName())
		},
		UpdateFunc: func(old, new interface{}) {
			cm := old.(*batchv1.CronJob)
			// fmt.Printf("Informer event: Event UPDATED %s/%s\n", cm.GetNamespace(), cm.GetName())
			cmNew := new.(*batchv1.CronJob)

			if cm.ResourceVersion != cmNew.ResourceVersion {
				// This is the true update, and not the "sync" event
				fmt.Printf("Informer event: Event TRUE-UPDATED %s/%s\n", cm.GetNamespace(), cm.GetName())
				fmt.Printf("Old:\n%#v\n", cm)
				// fmt.Printf("New:\n%#v\n\n", cmNew)

				empJSON, _ := json.MarshalIndent(cmNew, "", "  ")
				fmt.Printf("New:\n%s\n\n", string(empJSON))
			} else {
				fmt.Printf("Informer event: Event sync %s/%s\n", cm.GetNamespace(), cm.GetName())

			}
		},
		DeleteFunc: func(obj interface{}) {
			cm := obj.(*batchv1.CronJob)
			fmt.Printf("Informer event: Event DELETED %s/%s\n", cm.GetNamespace(), cm.GetName())
		},
	})

	ctx, _ = context.WithCancel(context.Background())

	factory.Start(ctx.Done())

	for informerType, ok := range factory.WaitForCacheSync(ctx.Done()) {
		if !ok {
			panic(fmt.Sprintf("Failed to sync cache for %v", informerType))
		}
	}
	fmt.Println("informer synced")

	return nil
}
