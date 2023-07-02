package main

import (
	"os"

	globalLogger "github.com/rs/zerolog/log"
)

func runIntegrations() {
	globalLogger.Info().Msg("Running integrations...")
	gkeIntegrationEnabled := isTruthy(os.Getenv("SENTRY_K8S_INTEGRATION_GKE_ENABLED"))
	if gkeIntegrationEnabled {
		runGkeIntegration()
	}
}
