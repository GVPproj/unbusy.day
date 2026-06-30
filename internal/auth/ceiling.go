package auth

import (
	"sync"
	"time"
)

// sendCeiling is the global outbound-OTP backstop: an in-process count of sends
// over a rolling window. When the window's send count reaches the ceiling the
// breaker trips — further sends are skipped — protecting domain reputation and
// the bill from a burst no per-source rate limit can catch. Single-machine by
// design, same constraint as the rate limiter and broker.
type sendCeiling struct {
	mu     sync.Mutex
	max    int
	window time.Duration
	sends  []time.Time
}

func newSendCeiling(max int, window time.Duration) *sendCeiling {
	return &sendCeiling{max: max, window: window}
}

// allow records a send and returns true when under the ceiling for the rolling
// window; at or over the ceiling it records nothing and returns false (breaker
// tripped). Stale timestamps outside the window are evicted on each call.
func (c *sendCeiling) allow(now time.Time) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	cutoff := now.Add(-c.window)
	kept := c.sends[:0]
	for _, t := range c.sends {
		if t.After(cutoff) {
			kept = append(kept, t)
		}
	}
	c.sends = kept
	if len(c.sends) >= c.max {
		return false
	}
	c.sends = append(c.sends, now)
	return true
}
