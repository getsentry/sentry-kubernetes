package main

import (
	"context"
	"encoding/json"
	"net/http"
	"os"

	"github.com/getsentry/sentry-go"
)

func runIntegrations() {
	gkeIntegrationEnabled := isTruthy(os.Getenv("SENTRY_K8S_INTEGRATION_GKE_ENABLED"))
	if gkeIntegrationEnabled {
		runGkeIntegration()
	}
}

func runGkeIntegration() {
	_, logger := getLoggerWithTag(context.Background(), "integration", "gke")
	logger.Info().Msg("Running GKE integration")

	scope := sentry.CurrentHub().Scope()

	// Instance metadata
	instanceMetadataUrl := "http://metadata.google.internal/computeMetadata/v1/instance/attributes/?recursive=true"
	resp, err := http.Get(instanceMetadataUrl)
	if err != nil {
		logger.Error().Msgf("Cannot fetch instance metadata: %w", err)
		return
	}
	defer resp.Body.Close()

	var instanceData map[string]string
	if err = json.NewDecoder(resp.Body).Decode(&instanceData); err != nil {
		logger.Error().Msgf("Cannot decode instance metadata: %w", err)
		return
	}

	clusterName := instanceData["cluster-name"]
	setTagIfNotEmpty(scope, "gke_cluster_name", clusterName)
	logger.Info().Msgf("Cluster name detected: %q", clusterName)
	clusterLocation := instanceData["cluster-location"]
	setTagIfNotEmpty(scope, "gke_cluster_location", clusterLocation)
	logger.Info().Msgf("Cluster location detected: %q", clusterLocation)

	// Project metadata
	projectMetadataUrl := "http://metadata.google.internal/computeMetadata/v1/project/?recursive=true"
	resp, err = http.Get(projectMetadataUrl)
	if err != nil {
		logger.Error().Msgf("Cannot fetch project metadata: %w", err)
		return
	}
	defer resp.Body.Close()

	var projectData map[string]string
	if err = json.NewDecoder(resp.Body).Decode(&projectData); err != nil {
		logger.Error().Msgf("Cannot decode project metadata: %w", err)
		return
	}

	projectName := projectData["projectId"]
	setTagIfNotEmpty(scope, "gke_project_name", projectName)
	logger.Info().Msgf("Project name detected: %q", projectName)

	scope.SetContext(
		"GKE Context",
		map[string]interface{}{
			"Cluster name":     clusterName,
			"Cluster location": clusterLocation,
			"GCP project":      projectName,
		},
	)
}
