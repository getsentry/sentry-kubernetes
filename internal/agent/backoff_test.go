package agent

import (
	"testing"
	"time"
)

func TestBackoffExponentialIncrease(t *testing.T) {
	b := newBackoff()

	expected := []time.Duration{
		1 * time.Second,
		2 * time.Second,
		4 * time.Second,
		8 * time.Second,
		16 * time.Second,
		30 * time.Second, // capped
		30 * time.Second, // stays at cap
	}

	for i, want := range expected {
		got := b.duration()
		if got != want {
			t.Errorf("call %d: got %v, want %v", i, got, want)
		}
	}
}

func TestBackoffReset(t *testing.T) {
	b := newBackoff()

	// Increase a few times
	b.duration()
	b.duration()
	b.duration()

	b.reset()

	got := b.duration()
	if got != time.Second {
		t.Errorf("after reset: got %v, want %v", got, time.Second)
	}
}

func TestBackoffCapsAtMax(t *testing.T) {
	b := newBackoff()

	// Call enough times to exceed max
	var last time.Duration
	for range 20 {
		last = b.duration()
	}

	if last != defaultMaxBackoff {
		t.Errorf("got %v, want %v", last, defaultMaxBackoff)
	}
}
