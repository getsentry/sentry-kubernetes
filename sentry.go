package main

import (
	"context"
	"os"
	"strings"

	"github.com/getsentry/sentry-go"
	globalLogger "github.com/rs/zerolog/log"
	v1 "k8s.io/api/core/v1"
	"k8s.io/client-go/rest"
)

func beforeSend(event *sentry.Event, hint *sentry.EventHint) *sentry.Event {
	// Update SDK info
	event.Sdk.Name = "tonyo.sentry-kubernetes"
	event.Sdk.Version = version

	// Clear modules/packages
	event.Modules = map[string]string{}

	// We don't need these for now
	event.Release = ""
	event.ServerName = ""

	return event
}

func initSentrySDK() {
	globalLogger.Debug().Msg("Initializing Sentry SDK...")
	err := sentry.Init(sentry.ClientOptions{
		Debug:         true,
		EnableTracing: false,
		BeforeSend:    beforeSend,
		// Clear integration list
		Integrations: func([]sentry.Integration) []sentry.Integration { return []sentry.Integration{} },
	})
	if err != nil {
		globalLogger.Fatal().Msgf("sentry.Init: %s", err)
	}

	if sentry.CurrentHub().Client().Options().Dsn == "" {
		globalLogger.Warn().Msg("No Sentry DSN specified, events will not be sent.")
	}

	globalLogger.Debug().Msg("Sentry SDK initialized")
}

func setKubernetesSentryContext(config *rest.Config) {
	kubernetesContext := map[string]interface{}{
		"API endpoint": config.Host,
	}

	// Get cluster version via API
	clusterVersion, err := getClusterVersion(config)
	if err == nil {
		kubernetesContext["Server version"] = clusterVersion.String()
	} else {
		globalLogger.Error().Msgf("Error while getting cluster version: %s", err)
	}

	sentry.CurrentHub().Scope().SetContext(
		"Kubernetes",
		kubernetesContext,
	)
}

func setGlobalSentryTags() {
	for _, e := range os.Environ() {
		pair := strings.SplitN(e, "=", 2)
		if len(pair) != 2 {
			continue
		}
		key, value := strings.TrimSpace(pair[0]), strings.TrimSpace(pair[1])
		tagPrefix := "SENTRY_K8S_GLOBAL_TAG_"
		if strings.HasPrefix(key, tagPrefix) {
			tagKey := strings.TrimPrefix(key, tagPrefix)
			globalLogger.Info().Msgf("Global tag detected: %s=%s", tagKey, value)
			sentry.CurrentHub().Scope().SetTag(tagKey, value)
		}
	}
}

func buildSentryEvent(ctx context.Context, event *v1.Event, scope *sentry.Scope) *sentry.Event {
	sentryEvent := &sentry.Event{Message: event.Message, Level: sentry.LevelError}
	runEnhancers(ctx, event, scope, sentryEvent)
	return sentryEvent
}

func setTagIfNotEmpty(scope *sentry.Scope, key string, value string) {
	if key != "" && value != "" {
		scope.SetTag(key, value)
	}
}
