package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"

	"github.com/getsentry/sentry-go"
	"github.com/rs/zerolog"
)

type IntegrationGKE struct {
	clusterLocation string
	clusterName     string
	projectName     string
	clusterUrl      string

	_initialized bool
}

func GetIntegrationGKE() *IntegrationGKE {
	return &instanceIntegrationGKE
}

var instanceIntegrationGKE = IntegrationGKE{_initialized: false}

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

func getGkeLogger() *zerolog.Logger {
	_, logger := getLoggerWithTag(context.Background(), "integration", "gke")
	return logger
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

func (igke *IntegrationGKE) IsEnabled() bool {
	return isTruthy(os.Getenv("SENTRY_K8S_INTEGRATION_GKE_ENABLED"))
}

func (igke *IntegrationGKE) IsInitialized() bool {
	return igke._initialized
}

func (igke *IntegrationGKE) Init() error {
	logger := getGkeLogger()
	logger.Info().Msg("Initializing GKE integration")

	// Instance metadata
	var instanceMeta InstanceMetadata
	err := readGoogleMetadata(instanceMetadataUrl, &instanceMeta)
	if err != nil {
		return fmt.Errorf("error initializing GKE integration: %v", err)
	}
	igke.clusterName = instanceMeta.ClusterName()
	igke.clusterLocation = instanceMeta.ClusterLocation()

	// Project metadata
	var projectMeta ProjectMetadata
	err = readGoogleMetadata(projectMetadataUrl, &projectMeta)
	if err != nil {
		return fmt.Errorf("error initializing GKE integration: %v", err)
	}
	igke.projectName = projectMeta.ProjectId()

	igke.clusterUrl = getClusterUrl(
		igke.clusterLocation, igke.clusterName, igke.projectName,
	)

	igke._initialized = true
	return nil
}

func (igke *IntegrationGKE) GetContext() (string, sentry.Context, error) {
	if !igke._initialized {
		return "", sentry.Context{}, fmt.Errorf("running GetContext on a non-initialized integration")
	}

	gkeContext := sentry.Context{
		"Cluster name":     igke.clusterName,
		"Cluster location": igke.clusterLocation,
		"GCP project":      igke.projectName,
	}
	if igke.clusterUrl != "" {
		gkeContext["Cluster URL"] = igke.clusterUrl
	}
	return "Google Kubernetes Engine", gkeContext, nil
}

func (igke *IntegrationGKE) GetTags() (map[string]string, error) {
	if !igke._initialized {
		return nil, fmt.Errorf("running GetTags on a non-initialized integration")
	}

	res := map[string]string{
		"gke_cluster_name":     igke.clusterName,
		"gke_cluster_location": igke.clusterLocation,
		"gke_project_name":     igke.projectName,
	}
	return res, nil
}

func (igke *IntegrationGKE) getLinkToPodLogs(podName string, namespace string) (string, error) {
	if !igke._initialized {
		return "", fmt.Errorf("the integration is not initialized")
	}

	projectName := igke.projectName
	clusterName := igke.clusterName
	clusterLocation := igke.clusterLocation

	if podName == "" || namespace == "" || projectName == "" || clusterName == "" || clusterLocation == "" {
		return "", nil
	}

	link := ("https://console.cloud.google.com/logs/query;query=" +
		"resource.type%%3D%%22k8s_container%%22%%0A" +
		fmt.Sprintf("resource.labels.project_id%%3D%%22%s%%22%%0A", projectName) +
		fmt.Sprintf("resource.labels.location%%3D%%22%s%%22%%0A", clusterLocation) +
		fmt.Sprintf("resource.labels.cluster_name%%3D%%22%s%%22%%0A", clusterName) +
		fmt.Sprintf("resource.labels.namespace_name%%3D%%22%s%%22%%0A", namespace) +
		fmt.Sprintf("resource.labels.pod_name%%3D%%22%s%%22%%0A", podName) +
		fmt.Sprintf(";duration=PT1H?project=%s", projectName))
	return link, nil
}

func addPodLogLinkToGKEContext(ctx context.Context, scope *sentry.Scope, podName string, namespace string) {
	logger := zerolog.Ctx(ctx)

	gkeIntegration := GetIntegrationGKE()
	if !gkeIntegration.IsEnabled() || !gkeIntegration.IsInitialized() {
		logger.Debug().Msgf("The GKE integration is not enabled or initialized, so not adding/modifying the context")
		return
	}

	logLink, err := gkeIntegration.getLinkToPodLogs(podName, namespace)
	if logLink != "" && err == nil {
		contextName, gkeContext, err := gkeIntegration.GetContext()
		if err != nil {
			logger.Debug().Msgf("Cannot get the context for GKE integration: %v", err)
			return
		}
		gkeContext["Pod Logs"] = logLink
		scope.SetContext(contextName, gkeContext)
	}
}
