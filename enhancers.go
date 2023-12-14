package main

import (
	"context"
	"fmt"

	"github.com/getsentry/sentry-go"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func runEnhancers(ctx context.Context, kind string, cachedObject metav1.Object, scope *sentry.Scope, sentryEvent *sentry.Event) {
	involvedObject := fmt.Sprintf("%s/%s", kind, cachedObject.GetName())
	ctx, logger := getLoggerWithTag(ctx, "object", involvedObject)

	var err error
	logger.Debug().Msgf("Running enhancers...")

	// First, run the basic (common) enhancer
	runCommonEnhancer(ctx, scope, sentryEvent)

	// Then, run kind-specific enhancers
	// The Pod enhancer will call corresponding enhancers depending on its root owners
	switch kind {
	case "Pod":
		podObj, ok := cachedObject.(*v1.Pod)
		if !ok {
			return
		}
		err = runPodEnhancer(ctx, podObj, scope, sentryEvent)
	}
	if err != nil {
		logger.Error().Msgf("Error running an enhancer: %v", err)
	}
}
