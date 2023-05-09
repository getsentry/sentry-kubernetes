package main

import (
	"fmt"
	"path/filepath"

	"github.com/rs/zerolog/log"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
)

func getClusterConfig(useInClusterConfig bool) (*rest.Config, error) {
	var config *rest.Config
	var err error

	if useInClusterConfig {
		log.Debug().Msg("Initializing in-cluster config...")

		config, err = rest.InClusterConfig()
		if err != nil {
			return nil, err
		}
	} else {
		log.Debug().Msg("Initializing out-of-cluster config...")

		var kubeconfig string
		if home := homedir.HomeDir(); home != "" {
			// FIXME: make this configurable
			kubeconfig = filepath.Join(home, ".kube", "config")
		} else {
			return nil, fmt.Errorf("Cannot find the default kubeconfig")
		}

		log.Debug().Msgf("Kubeconfig path: %s", kubeconfig)
		config, err = clientcmd.BuildConfigFromFlags("", kubeconfig)
		if err != nil {
			return nil, err
		}
	}
	return config, nil
}
