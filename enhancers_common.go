package main

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/getsentry/sentry-go"
	"github.com/rs/zerolog"
	globalLogger "github.com/rs/zerolog/log"
	v1 "k8s.io/api/core/v1"
)

type commonMsgPattern struct {
	regex           *regexp.Regexp
	fingerprintKeys []string
}

// Common message patterns that should be grouped better
var patternsAll = []*commonMsgPattern{
	{
		regex:           regexp.MustCompile(`Memory cgroup out of memory: Killed process (?P<process_id>\d+) \((?P<process_name>\w+)\).*`),
		fingerprintKeys: []string{"process_name"},
	},
}

func checkCommonEnhancerPatterns() {
	globalLogger.Debug().Msgf("Checking common enhancer patterns: making sure that they are correct")

	for _, pat := range patternsAll {
		regex := pat.regex
		captureGroups := regex.SubexpNames()
		captureGroupMap := make(map[string]struct{}, len(captureGroups))

		// Build a set of capture group names
		for _, groupName := range captureGroups {
			captureGroupMap[groupName] = struct{}{}
		}

		// Check that the fingerprint keys exist in capture group
		for _, key := range pat.fingerprintKeys {
			_, found := captureGroupMap[key]
			if !found {
				globalLogger.Panic().Msgf("Invalid pattern: cannot find %s in pattern %q", key, regex.String())
			}
		}
	}
}

func matchSinglePattern(ctx context.Context, message string, pattern *commonMsgPattern) []string {
	pat := pattern.regex

	match := pat.FindStringSubmatch(message)

	if match == nil {
		// No match
		return nil
	}

	subMatchMap := make(map[string]string)

	// Build the mapping: group name -> match
	for i, name := range pat.SubexpNames() {
		if i == 0 {
			continue
		}
		subMatchMap[name] = match[i]
	}
	fmt.Println(subMatchMap)

	fingerprint := []string{pat.String()}
	for _, value := range pattern.fingerprintKeys {
		fingerprint = append(fingerprint, subMatchMap[value])
	}
	return fingerprint
}

func matchCommonPatterns(ctx context.Context, event *v1.Event, scope *sentry.Scope, sentryEvent *sentry.Event) error {
	logger := zerolog.Ctx(ctx)
	message := sentryEvent.Message

	logger.Trace().Msgf("Matching against message: %q", message)

	for _, pattern := range patternsAll {
		fingerprint := matchSinglePattern(ctx, message, pattern)
		if fingerprint != nil {
			logger.Trace().Msgf("Pattern match: %v, fingerprint: %v", pattern, fingerprint)
			scope.SetFingerprint(fingerprint)
			return nil
		}
	}
	return nil
}

func runCommonEnhancer(ctx context.Context, event *v1.Event, scope *sentry.Scope, sentryEvent *sentry.Event) error {
	logger := zerolog.Ctx(ctx)

	logger.Debug().Msgf("Running the common enhancer, event message: %q", sentryEvent.Message)

	// Remove the "combined from similar events" prefix
	combinedFromSimilarEventsPrefix := "(combined from similar events):"
	if strings.HasPrefix(sentryEvent.Message, combinedFromSimilarEventsPrefix) {
		newMessage := strings.TrimPrefix(sentryEvent.Message, combinedFromSimilarEventsPrefix)
		sentryEvent.Message = strings.TrimSpace(newMessage)
		scope.SetTag("combined_from_similar", "true")
	}

	// Match common message patterns
	matchCommonPatterns(ctx, event, scope, sentryEvent)
	return nil
}
