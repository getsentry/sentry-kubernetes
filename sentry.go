package main

import (
	"github.com/getsentry/sentry-go"
	"github.com/rs/zerolog/log"
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
	log.Debug().Msg("Initializing Sentry SDK...")
	err := sentry.Init(sentry.ClientOptions{
		Debug:         true,
		EnableTracing: false,
		BeforeSend:    beforeSend,
		// Clear integration list
		Integrations: func([]sentry.Integration) []sentry.Integration { return []sentry.Integration{} },
	})
	if err != nil {
		log.Fatal().Msgf("sentry.Init: %s", err)
	}
	log.Debug().Msg("Sentry SDK initialized")
}
