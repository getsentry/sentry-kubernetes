package main

import (
	"context"
	"fmt"
	"time"

	"github.com/getsentry/sentry-go"
	"github.com/rs/zerolog/log"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

func BeforeSend(event *sentry.Event, hint *sentry.EventHint) *sentry.Event {
	// Update SDK info
	event.Sdk.Name = "tonyo.sentry-kubernetes"
	event.Sdk.Version = version

	// Clear modules/packages
	event.Modules = map[string]string{}

	return event
}

func initSentrySDK() {
	err := sentry.Init(sentry.ClientOptions{
		Debug:            true,
		TracesSampleRate: 0.0,
		EnableTracing:    false,
		BeforeSend:       BeforeSend,
	})
	if err != nil {
		log.Fatal().Msgf("sentry.Init: %s", err)
	}

}

func watchEvents() (err error) {
	config, err := rest.InClusterConfig()
	if err != nil {
		return err
	}
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return err
	}
	opts := metav1.ListOptions{
		// FieldSelector: "involvedObject.kind=Pod",
		Watch: true,
	}
	watcher, err := clientset.CoreV1().Events("default").Watch(context.Background(), opts)

	if err != nil {
		return err
	}

	watchCh := watcher.ResultChan()
	defer watcher.Stop()

	for event := range watchCh {
		fmt.Println(event)
	}

	return nil
}

func main() {
	initSentrySDK()
	log.Info().Msg("Done.")

	sentry.CaptureMessage("It works!")
	defer sentry.Flush(time.Second)

	err := watchEvents()
	if err != nil {
		fmt.Println(err)
	}
}
