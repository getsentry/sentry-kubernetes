package main

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/getsentry/sentry-go"
	"github.com/rs/zerolog/log"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	jsonserializer "k8s.io/apimachinery/pkg/runtime/serializer/json"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
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
		Debug:         true,
		EnableTracing: false,
		BeforeSend:    beforeSend,
	})
	if err != nil {
		log.Fatal().Msgf("sentry.Init: %s", err)
	}
	log.Debug().Msg("Sentry SDK initialized")
}

func watchEventsInNamespace(config *rest.Config, namespace string) (err error) {
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
	watcher, err := clientset.CoreV1().Events(namespace).Watch(ctx, opts)

	// FIXME: Watch() currently returns also all recent events.
	// Should we ignore events that happened in the past?

	if err != nil {
		return err
	}

	watchCh := watcher.ResultChan()
	defer watcher.Stop()

	log.Debug().Msg("Reading from the event channel...")
	for event := range watchCh {
		eventObjectRaw := event.Object
		eventType := string(event.Type)
		objectKind := eventObjectRaw.GetObjectKind()

		eventObject, ok := eventObjectRaw.(*v1.Event)
		if !ok {
			log.Warn().Msgf("Skipping an event of eventType '%s', kind '%v'", eventType, objectKind)
			continue
		}
		// log.Info().Str("type", eventType).Msgf("%#v", eventObject)

		if eventObject.Type == v1.EventTypeNormal {
			log.Debug().Msgf("Skipping an event of type Normal")
			continue
		}

		fmt.Printf("Kind: %#v\n", objectKind)
		fmt.Printf("EventObject: %#v\n", eventObject)
		fmt.Printf("Event type: %#v\n", eventObject.Type)
		fmt.Println()

		involvedObject := eventObject.InvolvedObject

		sentry.WithScope(func(scope *sentry.Scope) {
			// TODO: use SetTags
			scope.SetTag("eventType", eventObject.Type)
			scope.SetTag("objectName", involvedObject.Name)
			scope.SetTag("objectNamespace", involvedObject.Namespace)
			scope.SetTag("objectKind", involvedObject.Kind)
			scope.SetTag("objectUID", string(involvedObject.UID))

			encoder := jsonserializer.NewSerializerWithOptions(
				nil, // jsonserializer.MetaFactory
				nil, // runtime.ObjectCreater
				nil, // runtime.ObjectTyper
				jsonserializer.SerializerOptions{
					Yaml:   false,
					Pretty: true,
					Strict: false,
				},
			)

			// Runtime.Encode() is just a helper function to invoke Encoder.Encode()
			encodedEvent, err := runtime.Encode(encoder, eventObject)
			if err != nil {
				log.Error().Msgf("Error while serializing event: %s", err.Error())
			}
			scope.SetExtra("kubeEvent", string(encodedEvent))
			fmt.Println(string(encodedEvent))

			sentryEvent := sentry.Event{Message: eventObject.Message, Level: sentry.LevelError}
			sentry.CaptureEvent(&sentryEvent)
		})

	}

	return nil
}

func getConfig(useInClusterConfig bool) (*rest.Config, error) {
	var config *rest.Config
	var err error

	if useInClusterConfig {
		log.Debug().Msg("Initializing in-cluster config...")

		config, err = rest.InClusterConfig()
		if err != nil {
			return nil, err
		}
	} else {
		log.Debug().Msg("Initializing out-of-cluster config...")

		var kubeconfig string
		if home := homedir.HomeDir(); home != "" {
			// FIXME: make this configurable
			kubeconfig = filepath.Join(home, ".kube", "config")
		} else {
			return nil, fmt.Errorf("Cannot find the default kubeconfig")
		}

		log.Debug().Msgf("Kubeconfig path: %s", kubeconfig)
		config, err = clientcmd.BuildConfigFromFlags("", kubeconfig)
		if err != nil {
			return nil, err
		}
	}
	return config, nil
}

func main() {
	initSentrySDK()
	defer sentry.Flush(time.Second)

	// FIXME: make this configurable
	useInClusterConfig := false
	// FIXME: make this configurable
	namespace := "default"

	config, err := getConfig(useInClusterConfig)
	if err != nil {
		log.Fatal().Msgf("Config init error: %s", err)
	}

	err = watchEventsInNamespace(config, namespace)
	if err != nil {
		log.Fatal().Msgf("Watch error: %s", err)
	}
}
