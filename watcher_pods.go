package main

import (
	"context"

	globalLogger "github.com/rs/zerolog/log"
	"k8s.io/client-go/rest"
)

func startPodWatchers(ctx context.Context, config *rest.Config, namespaces []string) {
	for _, namespace := range namespaces {
		globalLogger.Debug().Msgf("TODO: starting pod watcher for namespace: %s, ctx: %v", namespace, ctx)
	}
}
