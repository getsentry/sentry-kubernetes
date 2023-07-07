package main

import (
	"os"
	"strings"

	globalLogger "github.com/rs/zerolog/log"
	v1 "k8s.io/api/core/v1"
)

// / Event Reason filter
var reasonFilterSet = map[string]struct{}{}
var defaultFilterReasons = []string{
	"DockerStart",
	"KubeletStart",
	"NodeSysctlChange",
	"ContainerdStart",
}

func prepareEventReasonFilter() {
	filterReasonsRaw := strings.TrimSpace(os.Getenv("SENTRY_K8S_FILTER_OUT_EVENT_REASONS"))
	var filterReasons []string
	if filterReasonsRaw == "" {
		filterReasons = defaultFilterReasons
	} else {
		filterReasons = strings.Split(filterReasonsRaw, ",")
	}
	for _, reason := range filterReasons {
		reason = strings.ToLower(strings.TrimSpace(reason))
		if reason != "" {
			reasonFilterSet[reason] = struct{}{}
		}
	}
	globalLogger.Debug().Msgf("Prepared the event reason filter: %v", reasonFilterSet)
}

// true -> the event should be dropped
func isFilteredByReason(event *v1.Event) bool {
	eventReason := strings.TrimSpace(strings.ToLower(event.Reason))
	if eventReason == "" {
		// Weird case, do not touch the event
		return false
	}
	_, found := reasonFilterSet[eventReason]
	return found
}

// / Event Source filter
var eventSourceFilterSet = map[string]struct{}{}
var defaultFilterEventSources = []string{}

func prepareEventSourceFilter() {
	filterEventSourcesRaw := strings.TrimSpace(os.Getenv("SENTRY_K8S_FILTER_OUT_EVENT_SOURCES"))
	var filterEventSources []string
	if filterEventSourcesRaw == "" {
		filterEventSources = defaultFilterEventSources
	} else {
		filterEventSources = strings.Split(filterEventSourcesRaw, ",")
	}
	for _, eventSource := range filterEventSources {
		eventSource = strings.ToLower(strings.TrimSpace(eventSource))
		if eventSource != "" {
			eventSourceFilterSet[eventSource] = struct{}{}
		}
	}
	globalLogger.Debug().Msgf("Prepared the event source filter: %v", eventSourceFilterSet)
}

// true -> the event should be dropped
func isFilteredByEventSource(event *v1.Event) bool {
	eventSource := strings.TrimSpace(strings.ToLower(event.Source.Component))
	if eventSource == "" {
		// Weird case, do not touch the event
		return false
	}
	_, found := eventSourceFilterSet[eventSource]
	return found
}

func prepareEventFilters() {
	prepareEventReasonFilter()
	prepareEventSourceFilter()
}
