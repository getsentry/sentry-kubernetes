package agent

import (
	"testing"

	v1 "k8s.io/api/core/v1"
)

func TestGetNamespacesToWatchDefault(t *testing.T) {
	t.Setenv("SENTRY_K8S_WATCH_NAMESPACES", "")

	watchAll, namespaces, err := getNamespacesToWatch()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if watchAll {
		t.Error("expected watchAll to be false")
	}
	if len(namespaces) != 1 || namespaces[0] != "default" {
		t.Errorf("expected [default], got %v", namespaces)
	}
}

func TestGetNamespacesToWatchAll(t *testing.T) {
	t.Setenv("SENTRY_K8S_WATCH_NAMESPACES", "__all__")

	watchAll, _, err := getNamespacesToWatch()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !watchAll {
		t.Error("expected watchAll to be true")
	}
}

func TestGetNamespacesToWatchCustom(t *testing.T) {
	t.Setenv("SENTRY_K8S_WATCH_NAMESPACES", "kube-system, monitoring, default")

	watchAll, namespaces, err := getNamespacesToWatch()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if watchAll {
		t.Error("expected watchAll to be false")
	}
	expected := []string{"kube-system", "monitoring", "default"}
	if len(namespaces) != len(expected) {
		t.Fatalf("expected %d namespaces, got %d", len(expected), len(namespaces))
	}
	for i, ns := range expected {
		if namespaces[i] != ns {
			t.Errorf("namespace[%d] = %q, want %q", i, namespaces[i], ns)
		}
	}
}

func TestGetNamespacesToWatchDeduplicates(t *testing.T) {
	t.Setenv("SENTRY_K8S_WATCH_NAMESPACES", "default, default, kube-system")

	_, namespaces, err := getNamespacesToWatch()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(namespaces) != 2 {
		t.Errorf("expected 2 namespaces after dedup, got %d: %v", len(namespaces), namespaces)
	}
}

func TestGetNamespacesToWatchInvalidName(t *testing.T) {
	t.Setenv("SENTRY_K8S_WATCH_NAMESPACES", "INVALID_NAMESPACE!")

	_, _, err := getNamespacesToWatch()
	if err == nil {
		t.Error("expected error for invalid namespace name")
	}
}

func TestGetObjectNameTag(t *testing.T) {
	tests := []struct {
		kind string
		want string
	}{
		{"Pod", "pod_name"},
		{"CronJob", "cronjob_name"},
		{"", "object_name"},
	}

	for _, tt := range tests {
		t.Run(tt.kind, func(t *testing.T) {
			ref := &v1.ObjectReference{Kind: tt.kind}
			if got := getObjectNameTag(ref); got != tt.want {
				t.Errorf("getObjectNameTag(%q) = %q, want %q", tt.kind, got, tt.want)
			}
		})
	}
}
