package agent

import (
	"strings"
	"testing"

	v1 "k8s.io/api/core/v1"
)

func TestPrepareEventReasonFilterDefaults(t *testing.T) {
	// Reset global state
	reasonFilterSet = map[string]struct{}{}

	t.Setenv("SENTRY_K8S_FILTER_OUT_EVENT_REASONS", "")
	prepareEventReasonFilter()

	// Check that default reasons are loaded (stored lowercased)
	for _, reason := range defaultFilterReasons {
		lowered := strings.ToLower(reason)
		if _, found := reasonFilterSet[lowered]; !found {
			t.Errorf("expected default reason %q in filter set", lowered)
		}
	}
}

func TestPrepareEventReasonFilterCustom(t *testing.T) {
	reasonFilterSet = map[string]struct{}{}

	t.Setenv("SENTRY_K8S_FILTER_OUT_EVENT_REASONS", "MyReason, OtherReason")
	prepareEventReasonFilter()

	if _, found := reasonFilterSet["myreason"]; !found {
		t.Error("expected 'myreason' in filter set")
	}
	if _, found := reasonFilterSet["otherreason"]; !found {
		t.Error("expected 'otherreason' in filter set")
	}
	// Default reasons should NOT be present
	if _, found := reasonFilterSet["dockerstart"]; found {
		t.Error("default reason should not be present when custom reasons are set")
	}
}

func TestIsFilteredByReason(t *testing.T) {
	reasonFilterSet = map[string]struct{}{
		"dockerstart": {},
	}

	tests := []struct {
		name     string
		reason   string
		filtered bool
	}{
		{"matching reason", "DockerStart", true},
		{"non-matching reason", "OOMKilled", false},
		{"empty reason", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			event := &v1.Event{Reason: tt.reason}
			if got := isFilteredByReason(event); got != tt.filtered {
				t.Errorf("isFilteredByReason(%q) = %v, want %v", tt.reason, got, tt.filtered)
			}
		})
	}
}

func TestPrepareEventSourceFilterCustom(t *testing.T) {
	eventSourceFilterSet = map[string]struct{}{}

	t.Setenv("SENTRY_K8S_FILTER_OUT_EVENT_SOURCES", "kubelet, scheduler")
	prepareEventSourceFilter()

	if _, found := eventSourceFilterSet["kubelet"]; !found {
		t.Error("expected 'kubelet' in filter set")
	}
	if _, found := eventSourceFilterSet["scheduler"]; !found {
		t.Error("expected 'scheduler' in filter set")
	}
}

func TestIsFilteredByEventSource(t *testing.T) {
	eventSourceFilterSet = map[string]struct{}{
		"kubelet": {},
	}

	tests := []struct {
		name      string
		component string
		filtered  bool
	}{
		{"matching source", "kubelet", true},
		{"non-matching source", "scheduler", false},
		{"empty source", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			event := &v1.Event{Source: v1.EventSource{Component: tt.component}}
			if got := isFilteredByEventSource(event); got != tt.filtered {
				t.Errorf("isFilteredByEventSource(%q) = %v, want %v", tt.component, got, tt.filtered)
			}
		})
	}
}
