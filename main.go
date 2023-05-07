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

func beforeSend(event *sentry.Event, hint *sentry.EventHint) *sentry.Event {
	// Update SDK info
	event.Sdk.Name = "tonyo.sentry-kubernetes"
	event.Sdk.Version = version

	// Clear modules/packages
	event.Modules = map[string]string{}

	return event
}

func initSentrySDK() {
	log.Debug().Msg("Initializing Sentry SDK")
	err := sentry.Init(sentry.ClientOptions{
		Debug:            true,
		TracesSampleRate: 0.0,
		EnableTracing:    false,
		BeforeSend:       beforeSend,
	})
	if err != nil {
		log.Fatal().Msgf("sentry.Init: %s", err)
	}
	log.Debug().Msg("Sentry SDK initialized")
}

func watchEvents() (err error) {
	log.Debug().Msg("Initializing cluster config...")
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
	log.Debug().Msg("Getting the event watcher...")

	ctx, cancelFunc := context.WithCancel(context.Background())
	defer cancelFunc()

	// Set "" to get events from all namespaces.
	// TODO: how to watch only for specific ones?
	watcher, err := clientset.CoreV1().Events("default").Watch(ctx, opts)

	if err != nil {
		return err
	}

	watchCh := watcher.ResultChan()
	defer watcher.Stop()

	log.Debug().Msg("Reading from the event channel...")
	for event := range watchCh {
		fmt.Println(event)
	}

	return nil
}

func main() {
	initSentrySDK()

	sentry.CaptureMessage("It works!")
	defer sentry.Flush(time.Second)

	err := watchEvents()
	if err != nil {
		fmt.Println("Error:", err)
	}
}
