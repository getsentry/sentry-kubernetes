package main

import (
	"time"

	"github.com/getsentry/sentry-go"
	"github.com/rs/zerolog/log"
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
	// Using SENTRY_DSN here
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

func main() {
	initSentrySDK()
	log.Info().Msg("Done.")

	sentry.CaptureMessage("It works!")
	defer sentry.Flush(time.Second)
}
