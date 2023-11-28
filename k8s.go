package main

import (
	"fmt"
	"os"
	"strings"

	globalLogger "github.com/rs/zerolog/log"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/validation"
	k8sVersion "k8s.io/apimachinery/pkg/version"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/rest"
)

var defaultNamespacesToWatch = []string{v1.NamespaceDefault}

const allNamespacesLabel = "__all__"

func getNamespacesToWatch() (watchAll bool, namespaces []string, err error) {
	watchNamespacesRaw := strings.TrimSpace(os.Getenv("SENTRY_K8S_WATCH_NAMESPACES"))

	// Nothing in the env variable => use the default value
	if watchNamespacesRaw == "" {
		return false, defaultNamespacesToWatch, nil
	}

	// Special label => watch all namespaces
	if watchNamespacesRaw == allNamespacesLabel {
		return true, []string{}, nil
	}

	rawNamespaces := strings.Split(watchNamespacesRaw, ",")
	namespaces = make([]string, 0, len(rawNamespaces))
	for _, rawNamespace := range rawNamespaces {
		namespace := strings.TrimSpace(rawNamespace)
		if namespace == "" {
			continue
		}
		errors := validation.IsValidLabelValue(namespace)
		if len(errors) != 0 {
			// Not a valid namespace name
			return false, []string{}, fmt.Errorf(errors[0])
		}
		namespaces = append(namespaces, namespace)
	}
	namespaces = removeDuplicates(namespaces)
	if len(namespaces) == 0 {
		return false, namespaces, fmt.Errorf("no namespaces specified")
	}

	return false, namespaces, nil
}

func getClusterVersion(config *rest.Config) (*k8sVersion.Info, error) {
	versionInfo := &k8sVersion.Info{}

	discoveryClient, err := discovery.NewDiscoveryClientForConfig(config)
	if err != nil {
		return versionInfo, err
	}
	globalLogger.Debug().Msgf("Fetching cluster version...")
	versionInfo, err = discoveryClient.ServerVersion()
	globalLogger.Debug().Msgf("Cluster version: %s", versionInfo)
	return versionInfo, err
}

func getObjectNameTag(object *v1.ObjectReference) string {
	if object.Kind == "" {
		return "object_name"
	} else {
		return fmt.Sprintf("%s_name", strings.ToLower(object.Kind))
	}
}
