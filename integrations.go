package main

import (
	"github.com/getsentry/sentry-go"
	globalLogger "github.com/rs/zerolog/log"
)

type AgentIntegration interface {
	IsEnabled() bool
	Init() error
	IsInitialized() bool
	GetContext() (string, sentry.Context, error)
	GetTags() (map[string]string, error)
}

func runIntegrations() error {
	globalLogger.Info().Msg("Running integrations...")

	scope := sentry.CurrentHub().Scope()

	allIntegrations := []AgentIntegration{
		GetIntegrationGKE(),
	}

	for _, integration := range allIntegrations {
		if !integration.IsEnabled() {
			continue
		}

		if err := integration.Init(); err != nil {
			return err
		}

		// Process context
		contextName, context, err := integration.GetContext()
		if err != nil {
			return err
		}
		scope.SetContext(contextName, context)

		// Process tags
		tags, err := integration.GetTags()
		if err != nil {
			return err
		}
		for tagKey, tagValue := range tags {
			setTagIfNotEmpty(scope, tagKey, tagValue)
		}
	}
	return nil
}
