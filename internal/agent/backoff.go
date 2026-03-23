package agent

import "time"

const (
	defaultInitialBackoff = time.Second
	defaultMaxBackoff     = 30 * time.Second
)

// backoff implements a simple capped exponential backoff.
type backoff struct {
	current time.Duration
	initial time.Duration
	max     time.Duration
}

func newBackoff() *backoff {
	return &backoff{
		current: defaultInitialBackoff,
		initial: defaultInitialBackoff,
		max:     defaultMaxBackoff,
	}
}

// duration returns the current backoff duration after a failure
// and increases it for the next call (capped at max).
func (b *backoff) duration() time.Duration {
	d := b.current
	if b.current < b.max {
		b.current *= 2
		if b.current > b.max {
			b.current = b.max
		}
	}
	return d
}

// reset restores the backoff to its initial value after a success.
func (b *backoff) reset() {
	b.current = b.initial
}
