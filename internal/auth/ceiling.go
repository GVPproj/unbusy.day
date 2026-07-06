package auth

import (
	"sync"
	"time"
)

// sendCeiling is the global outbound-OTP circuit breaker: an in-process send
// count over a rolling window. Single-machine by design, like the rate limiter
// and broker.
type sendCeiling struct {
	mu     sync.Mutex
	max    int
	window time.Duration
	sends  []time.Time
}

func newSendCeiling(max int, window time.Duration) *sendCeiling {
	return &sendCeiling{max: max, window: window}
}

// allow records a send and reports whether it's under the ceiling.
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
