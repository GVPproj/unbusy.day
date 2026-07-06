package frontend

import (
	"net"
	"net/http"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

type rateLimitConfig struct {
	perIPEvery  time.Duration // one token per this interval, per source IP
	perIPBurst  int
	globalEvery time.Duration // process-wide ceiling regardless of IP spread
	globalBurst int
	trustProxy  bool // honor Fly-Client-IP (only behind Fly's proxy)
}

type ipEntry struct {
	lim      *rate.Limiter
	lastSeen time.Time
}

// LoginRateLimiter bounds the pre-auth send path on POST /login/code: a
// per-source-IP bucket plus a global cross-IP ceiling. In-process by design,
// like the broker.
type LoginRateLimiter struct {
	cfg    rateLimitConfig
	global *rate.Limiter

	mu  sync.Mutex
	ips map[string]*ipEntry
}

// NewLoginRateLimiter builds the production limiter and starts its idle-bucket
// sweeper. trustProxy must be true only behind Fly's proxy.
func NewLoginRateLimiter(trustProxy bool) *LoginRateLimiter {
	l := newRateLimiter(rateLimitConfig{
		perIPEvery:  6 * time.Second,
		perIPBurst:  5,
		globalEvery: 6 * time.Second,
		globalBurst: 20,
		trustProxy:  trustProxy,
	})
	go func() {
		for range time.Tick(10 * time.Minute) {
			l.sweep(15 * time.Minute)
		}
	}()
	return l
}

func newRateLimiter(cfg rateLimitConfig) *LoginRateLimiter {
	return &LoginRateLimiter{
		cfg:    cfg,
		global: rate.NewLimiter(rate.Every(cfg.globalEvery), cfg.globalBurst),
		ips:    make(map[string]*ipEntry),
	}
}

func (l *LoginRateLimiter) limiterFor(ip string) *rate.Limiter {
	l.mu.Lock()
	defer l.mu.Unlock()
	e, ok := l.ips[ip]
	if !ok {
		e = &ipEntry{lim: rate.NewLimiter(rate.Every(l.cfg.perIPEvery), l.cfg.perIPBurst)}
		l.ips[ip] = e
	}
	e.lastSeen = time.Now()
	return e.lim
}

// sweep drops buckets untouched for longer than maxIdle. A long-idle bucket is
// at full tokens, so evicting it is indistinguishable from a fresh one.
func (l *LoginRateLimiter) sweep(maxIdle time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	cutoff := time.Now().Add(-maxIdle)
	for ip, e := range l.ips {
		if e.lastSeen.Before(cutoff) {
			delete(l.ips, ip)
		}
	}
}

func (l *LoginRateLimiter) numIPs() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.ips)
}

// expireAll backdates every bucket so a subsequent sweep evicts it (test seam).
func (l *LoginRateLimiter) expireAll() {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, e := range l.ips {
		e.lastSeen = time.Time{}
	}
}

// Limit wraps next, rejecting requests over the per-IP or global rate with a
// bare 429.
func (l *LoginRateLimiter) Limit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := clientIP(r, l.cfg.trustProxy)
		if !l.limiterFor(ip).Allow() || !l.global.Allow() {
			http.Error(w, "too many requests", http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// clientIP trusts Fly-Client-IP only behind Fly's proxy — otherwise an
// attacker could spoof the header to dodge the limit.
func clientIP(r *http.Request, trustProxy bool) string {
	if trustProxy {
		if ip := r.Header.Get("Fly-Client-IP"); ip != "" {
			return ip
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
