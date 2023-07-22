package main

import (
	"context"
	"fmt"

	"github.com/getsentry/sentry-go"
	v1 "k8s.io/api/core/v1"
)

func runEnhancers(ctx context.Context, objectRef *v1.ObjectReference, cachedObject interface{}, scope *sentry.Scope, sentryEvent *sentry.Event) {
	involvedObject := fmt.Sprintf("%s/%s", objectRef.Kind, objectRef.Name)
	ctx, logger := getLoggerWithTag(ctx, "object", involvedObject)

	var err error
	logger.Debug().Msgf("Running enhancers...")

	// First, run the basic (common) enhancer
	runCommonEnhancer(ctx, scope, sentryEvent)

	// Then, run kind-specific enhancers
	switch objectRef.Kind {
	case "Pod":
		err = runPodEnhancer(ctx, objectRef, cachedObject, scope, sentryEvent)
	}

	if err != nil {
		logger.Error().Msgf("Error running an enhancer: %v", err)
	}
}
