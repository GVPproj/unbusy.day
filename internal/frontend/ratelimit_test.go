// Rate-limit middleware tests. Small bursts so limits trip deterministically
// without sleeping.
package frontend

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func okHandler(ran *int) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*ran++
		w.WriteHeader(http.StatusOK)
	})
}

func postFrom(ip string) *http.Request {
	req := httptest.NewRequest(http.MethodPost, "/login/code", nil)
	req.RemoteAddr = ip + ":12345"
	return req
}

func TestRateLimitAllowsWithinBurst(t *testing.T) {
	rl := newRateLimiter(rateLimitConfig{perIPEvery: time.Second, perIPBurst: 3, globalEvery: time.Second, globalBurst: 100})
	ran := 0
	h := rl.Limit(okHandler(&ran))

	for i := 0; i < 3; i++ {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, postFrom("1.2.3.4"))
		if rec.Code != http.StatusOK {
			t.Fatalf("request %d: got %d, want 200", i, rec.Code)
		}
	}
	if ran != 3 {
		t.Fatalf("handler ran %d times, want 3", ran)
	}
}

func TestRateLimitRejectsOverIPBurst(t *testing.T) {
	rl := newRateLimiter(rateLimitConfig{perIPEvery: time.Hour, perIPBurst: 2, globalEvery: time.Second, globalBurst: 100})
	ran := 0
	h := rl.Limit(okHandler(&ran))

	var last int
	for i := 0; i < 3; i++ {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, postFrom("1.2.3.4"))
		last = rec.Code
	}
	if last != http.StatusTooManyRequests {
		t.Fatalf("3rd request: got %d, want 429", last)
	}
	if ran != 2 {
		t.Fatalf("handler ran %d times, want 2 (3rd blocked)", ran)
	}
}

func TestRateLimitIsolatesIPs(t *testing.T) {
	rl := newRateLimiter(rateLimitConfig{perIPEvery: time.Hour, perIPBurst: 1, globalEvery: time.Second, globalBurst: 100})
	ran := 0
	h := rl.Limit(okHandler(&ran))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, postFrom("1.1.1.1"))
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, postFrom("1.1.1.1"))
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("2nd from same IP: got %d, want 429", rec.Code)
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, postFrom("2.2.2.2"))
	if rec.Code != http.StatusOK {
		t.Fatalf("1st from fresh IP: got %d, want 200", rec.Code)
	}
}

// The global ceiling trips even when every source stays under its own per-IP
// limit (the spread-across-many-IPs attack).
func TestRateLimitGlobalCeiling(t *testing.T) {
	rl := newRateLimiter(rateLimitConfig{perIPEvery: time.Hour, perIPBurst: 5, globalEvery: time.Hour, globalBurst: 3})
	ran := 0
	h := rl.Limit(okHandler(&ran))

	// Fresh IP per request, so only the global bucket can stop them.
	codes := []int{}
	for _, ip := range []string{"10.0.0.1", "10.0.0.2", "10.0.0.3", "10.0.0.4"} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, postFrom(ip))
		codes = append(codes, rec.Code)
	}
	want := []int{200, 200, 200, 429}
	for i, c := range codes {
		if c != want[i] {
			t.Fatalf("request %d: got %d, want %d (all: %v)", i, c, want[i], codes)
		}
	}
}

// When trusted (behind Fly's proxy), the per-IP bucket keys on Fly-Client-IP,
// not the shared edge RemoteAddr — clients behind the proxy must not share a bucket.
func TestRateLimitTrustsFlyClientIP(t *testing.T) {
	rl := newRateLimiter(rateLimitConfig{perIPEvery: time.Hour, perIPBurst: 1, globalEvery: time.Second, globalBurst: 100, trustProxy: true})
	ran := 0
	h := rl.Limit(okHandler(&ran))

	send := func(clientIP string) int {
		req := postFrom("100.64.0.1") // shared Fly edge addr
		req.Header.Set("Fly-Client-IP", clientIP)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec.Code
	}
	if c := send("9.9.9.9"); c != http.StatusOK {
		t.Fatalf("client A 1st: got %d, want 200", c)
	}
	if c := send("9.9.9.9"); c != http.StatusTooManyRequests {
		t.Fatalf("client A 2nd: got %d, want 429", c)
	}
	if c := send("8.8.8.8"); c != http.StatusOK {
		t.Fatalf("client B 1st: got %d, want 200 (must not share A's bucket)", c)
	}
}

// When not trusted, a spoofed Fly-Client-IP can't mint fresh buckets.
func TestRateLimitIgnoresFlyClientIPWhenUntrusted(t *testing.T) {
	rl := newRateLimiter(rateLimitConfig{perIPEvery: time.Hour, perIPBurst: 1, globalEvery: time.Second, globalBurst: 100, trustProxy: false})
	ran := 0
	h := rl.Limit(okHandler(&ran))

	send := func(spoofed string) int {
		req := postFrom("1.2.3.4") // real RemoteAddr stays constant
		req.Header.Set("Fly-Client-IP", spoofed)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec.Code
	}
	if c := send("9.9.9.9"); c != http.StatusOK {
		t.Fatalf("1st: got %d, want 200", c)
	}
	if c := send("8.8.8.8"); c != http.StatusTooManyRequests {
		t.Fatalf("2nd spoofed: got %d, want 429", c)
	}
}

// sweep drops idle per-IP buckets so the map can't grow without bound.
func TestRateLimitSweepEvictsIdleIPs(t *testing.T) {
	rl := newRateLimiter(rateLimitConfig{perIPEvery: time.Hour, perIPBurst: 1, globalEvery: time.Second, globalBurst: 100})
	h := rl.Limit(okHandler(new(int)))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, postFrom("1.2.3.4"))
	if got := rl.numIPs(); got != 1 {
		t.Fatalf("after one request: %d buckets, want 1", got)
	}

	rl.expireAll()
	rl.sweep(time.Minute)
	if got := rl.numIPs(); got != 0 {
		t.Fatalf("after sweep: %d buckets, want 0", got)
	}
}
