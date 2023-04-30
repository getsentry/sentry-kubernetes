package main

import (
	"github.com/getsentry/sentry-go"
	"github.com/rs/zerolog/log"
)

func BeforeSend(event *sentry.Event, hint *sentry.EventHint) *sentry.Event {
	// Update SDK info
	event.Sdk.Name = "tonyo.sentry-kubernetes"
	event.Sdk.Version = "FIXME"

	// Clear modules/packages
	event.Modules = map[string]string{}

	return event
}

func initSentrySDK() {
	// Using SENTRY_DSN here
	err := sentry.Init(sentry.ClientOptions{
		// Enable printing of SDK debug messages.
		// Useful when getting started or trying to figure something out.
		Debug:            true,
		TracesSampleRate: 0.0,
		BeforeSend:       BeforeSend,
	})
	if err != nil {
		log.Fatal().Msgf("sentry.Init: %s", err)
	}

}

func main() {
	initSentrySDK()
	log.Info().Msg("Done.")
}
