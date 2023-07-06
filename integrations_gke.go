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
	// Allow both types of casing for compatibility
	ClusterName1     string `json:"cluster-name"`
	ClusterName2     string `json:"clusterName"`
	ClusterLocation1 string `json:"cluster-location"`
	ClusterLocation2 string `json:"clusterLocation"`
}

type ProjectMetadata struct {
	ProjectId1        string `json:"project-id"`
	ProjectId2        string `json:"projectId"`
	NumericProjectId1 int    `json:"numeric-project-id"`
	NumericProjectId2 int    `json:"numericProjectId"`
}

func (im *InstanceMetadata) ClusterName() string {
	if im.ClusterName1 != "" {
		return im.ClusterName1
	}
	return im.ClusterName2
}

func (im *InstanceMetadata) ClusterLocation() string {
	if im.ClusterLocation1 != "" {
		return im.ClusterLocation1
	}
	return im.ClusterLocation2
}

func (pm *ProjectMetadata) ProjectId() string {
	if pm.ProjectId1 != "" {
		return pm.ProjectId1
	}
	return pm.ProjectId2
}

func (pm *ProjectMetadata) NumericProjectId() int {
	if pm.NumericProjectId1 != 0 {
		return pm.NumericProjectId1
	}
	return pm.NumericProjectId2
}

func getClusterUrl(clusterLocation string, clusterName string, projectId string) string {
	if clusterLocation == "" || clusterName == "" || projectId == "" {
		return ""
	}
	return fmt.Sprintf(
		"https://console.cloud.google.com/kubernetes/clusters/details/%s/%s/details?project=%s",
		clusterLocation,
		clusterName,
		projectId,
	)
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

	clusterName := instanceMeta.ClusterName()
	setTagIfNotEmpty(scope, "gke_cluster_name", clusterName)
	clusterLocation := instanceMeta.ClusterLocation()
	setTagIfNotEmpty(scope, "gke_cluster_location", clusterLocation)

	// Project metadata
	var projectMeta ProjectMetadata
	err = readGoogleMetadata(projectMetadataUrl, &projectMeta)
	if err != nil {
		logger.Error().Msgf("Error running GKE integration: %v", err)
		return
	}

	projectName := projectMeta.ProjectId()
	setTagIfNotEmpty(scope, "gke_project_name", projectName)

	gkeContext := map[string]interface{}{
		"Cluster name":     clusterName,
		"Cluster location": clusterLocation,
		"GCP project":      projectName,
	}

	clusterUrl := getClusterUrl(clusterLocation, clusterName, projectName)
	if clusterUrl != "" {
		gkeContext["Cluster URL"] = clusterUrl
	}

	logger.Info().Msgf("GKE Context discovered: %v", gkeContext)

	scope.SetContext(
		"Google Kubernetes Engine",
		gkeContext,
	)
}
