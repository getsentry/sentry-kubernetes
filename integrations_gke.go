package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/getsentry/sentry-go"
)

const instanceMetadataUrl = "http://metadata.google.internal/computeMetadata/v1/instance/attributes/?recursive=true"
const projectMetadataUrl = "http://metadata.google.internal/computeMetadata/v1/project/?recursive=true"

type InstanceMetadata struct {
	ClusterName     string `json:"cluster-name"`
	ClusterLocation string `json:"cluster-location"`
}

type ProjectMetadata struct {
	ProjectId        string `json:"projectId"`
	NumericProjectId int    `json:"numbericProjectId"`
}

func readGoogleMetadata(url string, output interface{}) error {
	client := http.Client{}

	req, _ := http.NewRequest("GET", url, nil)
	req.Header = http.Header{
		"Metadata-Flavor": {"Google"},
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("cannot fetch metadata: %v", err)
	}
	defer resp.Body.Close()

	if err = json.NewDecoder(resp.Body).Decode(output); err != nil {
		return fmt.Errorf("cannot decode metadata: %v", err)
	}
	return nil
}

func runGkeIntegration() {
	_, logger := getLoggerWithTag(context.Background(), "integration", "gke")
	logger.Info().Msg("Running GKE integration")

	scope := sentry.CurrentHub().Scope()

	// Instance metadata
	var instanceMeta InstanceMetadata
	err := readGoogleMetadata(instanceMetadataUrl, &instanceMeta)
	if err != nil {
		logger.Error().Msgf("Error running GKE integration: %v", err)
		return
	}

	clusterName := instanceMeta.ClusterName
	setTagIfNotEmpty(scope, "gke_cluster_name", clusterName)
	clusterLocation := instanceMeta.ClusterLocation
	setTagIfNotEmpty(scope, "gke_cluster_location", clusterLocation)

	// Project metadata
	var projectMeta ProjectMetadata
	err = readGoogleMetadata(projectMetadataUrl, &projectMeta)
	if err != nil {
		logger.Error().Msgf("Error running GKE integration: %v", err)
		return
	}

	projectName := projectMeta.ProjectId
	setTagIfNotEmpty(scope, "gke_project_name", projectName)

	gkeContext := map[string]interface{}{
		"Cluster name":     clusterName,
		"Cluster location": clusterLocation,
		"GCP project":      projectName,
	}

	logger.Info().Msgf("GKE Context discovered: %v", gkeContext)

	scope.SetContext(
		"GKE Context",
		gkeContext,
	)
}