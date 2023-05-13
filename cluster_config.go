package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/rs/zerolog/log"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
)

const typeInCluster = "in-cluster"
const typeOutCluster = "out-cluster"

func getClusterConfig() (*rest.Config, error) {
	var config *rest.Config
	var err error

	configType := strings.ToLower(os.Getenv("SENTRY_K8S_CLUSTER_CONFIG_TYPE"))

	if configType != "" &&
		configType != typeInCluster &&
		configType != typeOutCluster {
		log.Fatal().Msgf(
			"Infalid cluster configuration type provided in SENTRY_K8S_CLUSTER_CONFIG_TYPE: %s",
			configType,
		)
	}

	autoConfig := configType == ""
	if autoConfig {
		log.Info().Msg("Auto-detecting cluster configuration...")
	}

	if autoConfig || configType == typeInCluster {
		log.Debug().Msg("Initializing in-cluster config...")

		config, err = rest.InClusterConfig()
		if err != nil {
			if autoConfig {
				log.Warn().Msgf("Could not initialize in-cluster config")
			} else {
				return nil, err
			}
		}
	}

	if autoConfig || configType == typeOutCluster {
		log.Debug().Msg("Initializing out-of-cluster config...")

		kubeconfig := os.Getenv("SENTRY_K8S_KUBECONFIG_PATH")

		if kubeconfig == "" {
			log.Debug().Msg("Trying to read kubeconfig from home directory...")

			if home := homedir.HomeDir(); home != "" {
				kubeconfig = filepath.Join(home, ".kube", "config")
			} else {
				return nil, fmt.Errorf("cannot find the default kubeconfig")
			}
		}

		log.Debug().Msgf("Kubeconfig path: %s", kubeconfig)
		config, err = clientcmd.BuildConfigFromFlags("", kubeconfig)
		if err != nil {
			return nil, err
		}
	}

	if config == nil {
		return nil, fmt.Errorf("cannot initialize cluster config")
	}

	return config, nil
}
