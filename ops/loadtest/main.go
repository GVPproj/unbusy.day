// Command loadtest is the M3c connection-ceiling drill (PRD §11 "connection
// -count bottleneck", §9 scale-out trigger): a modest concurrent-SSE ramp
// against the live origin to find the per-connection ceiling on shared-cpu-1x.
//
// It is black-box and client-side only — no access to the box is needed. Each
// virtual client opens one real EventSource-shaped GET /api/events. We force
// HTTP/1.1 with keep-alives disabled so every client maps to exactly ONE
// origin TCP connection (HTTP/2 would multiplex many streams over few conns
// and hide the FD/conn ceiling we are trying to measure).
//
// The ramp opens connections in steps, holds, and at peak fires a real reorder
// and measures how fast + how COMPLETELY the mutation fans out to every held
// subscriber — the degradation that actually triggers §9 scale-out is not
// "connections refused" but "fan-out stops reaching everyone".
//
// Usage (run from repo root):
//
//	go run ./ops/loadtest -url https://hello-cards.fly.dev \
//	  -steps 250,500,1000,1500,2000 -hold 12s -peak-hold 45s
//
// Hitting *.fly.dev directly (not the Cloudflare apex) keeps the measurement
// at the app: CF would pool/multiplex origin conns and muddy the count.
package main

import (
	"bufio"
	"context"
	"crypto/tls"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

type stats struct {
	attempted   atomic.Int64 // dials started
	established atomic.Int64 // got 200 + headers
	failed      atomic.Int64 // dial/handshake/non-200/early-EOF
	active      atomic.Int64 // currently held open
	keepalives  atomic.Int64 // :keepalive frames received (fleet-wide)
	events      atomic.Int64 // id:/data: mutation frames received (fleet-wide)
	ttfbSumMs   atomic.Int64
	ttfbN       atomic.Int64
}

func main() {
	var (
		url      = flag.String("url", "https://hello-cards.fly.dev", "origin base URL")
		stepsCSV = flag.String("steps", "250,500,1000,1500,2000", "cumulative concurrent targets")
		hold     = flag.Duration("hold", 12*time.Second, "hold time per step")
		peakHold = flag.Duration("peak-hold", 45*time.Second, "hold at final step (cross a 25s keepalive boundary)")
		dialBurst = flag.Int("burst", 100, "max simultaneous in-flight dials when ramping")
		probeEach = flag.Bool("probe-each", false, "fire a fan-out probe at the end of every step (maps the §2 ~1s SLO vs load)")
	)
	flag.Parse()

	steps := parseSteps(*stepsCSV)
	if len(steps) == 0 {
		fmt.Fprintln(os.Stderr, "no steps")
		os.Exit(1)
	}

	// One shared transport, but every request gets a fresh TCP conn:
	// DisableKeepAlives + an empty TLSNextProto map pins us to HTTP/1.1 so
	// 1 client == 1 connection at the origin.
	tr := &http.Transport{
		DisableKeepAlives:   true,
		MaxConnsPerHost:     0,
		TLSClientConfig:     &tls.Config{},
		TLSNextProto:        map[string]func(string, *tls.Conn) http.RoundTripper{}, // disable h2
		MaxIdleConns:        0,
		IdleConnTimeout:     0,
		TLSHandshakeTimeout: 15 * time.Second,
	}
	client := &http.Client{Transport: tr}

	st := &stats{}
	rootCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Live reporter.
	repDone := make(chan struct{})
	go reporter(rootCtx, st, repDone)

	var wg sync.WaitGroup
	sem := make(chan struct{}, *dialBurst)
	opened := 0

	for _, target := range steps {
		if rootCtx.Err() != nil {
			break
		}
		fmt.Printf("\n=== ramp → %d concurrent ===\n", target)
		for opened < target {
			if rootCtx.Err() != nil {
				break
			}
			sem <- struct{}{}
			wg.Add(1)
			go func() {
				defer wg.Done()
				// Release the dial slot once the handshake resolves (success
				// OR failure), NOT when the long-lived stream finally closes —
				// otherwise sem caps total held connections instead of pacing
				// concurrent dials, and the ramp wedges at -burst.
				var once sync.Once
				release := func() { once.Do(func() { <-sem }) }
				holdConn(rootCtx, client, *url, st, release)
			}()
			opened++
		}
		// Let the burst settle, then hold.
		settleAndHold(rootCtx, st, target, *hold)
		if *probeEach && rootCtx.Err() == nil {
			fanoutProbe(rootCtx, client, *url, st)
		}
	}

	// Peak: confirm held conns survive a keepalive cycle, then probe fan-out.
	if rootCtx.Err() == nil {
		fmt.Printf("\n=== peak hold %s (keepalive survival) ===\n", *peakHold)
		probeAt := time.Duration(float64(*peakHold) * 0.6)
		holdUntil := time.After(*peakHold)
		fired := false
		tick := time.NewTicker(probeAt)
		defer tick.Stop()
	peak:
		for {
			select {
			case <-rootCtx.Done():
				break peak
			case <-holdUntil:
				break peak
			case <-tick.C:
				if !fired {
					fired = true
					fanoutProbe(rootCtx, client, *url, st)
				}
			}
		}
	}

	fmt.Println("\n=== draining ===")
	stop() // cancel all conns
	wg.Wait()
	<-repDone
	finalReport(st)
}

// holdConn opens one SSE stream and reads it until ctx is cancelled or the
// stream errors. It is the unit of "one concurrent connection".
func holdConn(ctx context.Context, client *http.Client, base string, st *stats, release func()) {
	// release frees the caller's dial slot the moment the handshake resolves.
	// Guard every early-return path so a failed dial doesn't leak the slot.
	st.attempted.Add(1)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/api/events", nil)
	if err != nil {
		st.failed.Add(1)
		release()
		return
	}
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Cache-Control", "no-cache")

	t0 := time.Now()
	resp, err := client.Do(req)
	if err != nil {
		st.failed.Add(1)
		release()
		return
	}
	if resp.StatusCode != http.StatusOK {
		st.failed.Add(1)
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		release()
		return
	}
	st.ttfbSumMs.Add(time.Since(t0).Milliseconds())
	st.ttfbN.Add(1)
	st.established.Add(1)
	st.active.Add(1)
	release() // handshake done — free the dial slot; keep holding the stream
	defer st.active.Add(-1)
	defer resp.Body.Close()

	sc := bufio.NewScanner(resp.Body)
	sc.Buffer(make([]byte, 0, 8192), 1<<20)
	for sc.Scan() {
		line := sc.Text()
		switch {
		case strings.HasPrefix(line, ":keepalive"):
			st.keepalives.Add(1)
		case strings.HasPrefix(line, "id:"):
			st.events.Add(1)
		}
		if ctx.Err() != nil {
			return
		}
	}
}

// fanoutProbe fires a real reorder and measures how long until the held fleet
// observes it: time-to-first-event and time-to-90%-of-active. This is the
// load-bearing §9 signal — a healthy machine fans out to ~everyone fast.
func fanoutProbe(ctx context.Context, client *http.Client, base string, st *stats) {
	active := st.active.Load()
	order, err := currentOrder(ctx, client, base)
	if err != nil {
		fmt.Printf("[probe] skipped: read order failed: %v\n", err)
		return
	}
	rotated := append(order[1:], order[0]) // a rotation is always a valid permutation

	e0 := st.events.Load()
	t0 := time.Now()
	if err := postReorder(ctx, client, base, rotated); err != nil {
		fmt.Printf("[probe] reorder POST failed: %v\n", err)
		return
	}
	postLatency := time.Since(t0)

	var tFirst, t90 time.Duration
	target90 := e0 + int64(float64(active)*0.9)
	deadline := time.After(8 * time.Second)
	poll := time.NewTicker(20 * time.Millisecond)
	defer poll.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-deadline:
			got := st.events.Load() - e0
			fmt.Printf("[probe] active=%d post=%v first=%v reached=%d/%d (timeout before 90%%)\n",
				active, postLatency.Round(time.Millisecond), tFirst.Round(time.Millisecond), got, active)
			return
		case <-poll.C:
			got := st.events.Load()
			if tFirst == 0 && got > e0 {
				tFirst = time.Since(t0)
			}
			if got >= target90 {
				t90 = time.Since(t0)
				fmt.Printf("[probe] active=%d post=%v first=%v reach90%%=%v (reached %d/%d)\n",
					active, postLatency.Round(time.Millisecond), tFirst.Round(time.Millisecond),
					t90.Round(time.Millisecond), got-e0, active)
				return
			}
		}
	}
}

func currentOrder(ctx context.Context, client *http.Client, base string) ([]string, error) {
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, base+"/api/cards", nil)
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var body struct {
		Cards []struct {
			ID       string `json:"id"`
			Position int    `json:"position"`
		} `json:"cards"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, err
	}
	ids := make([]string, len(body.Cards))
	for i, c := range body.Cards {
		ids[i] = c.ID // already position-ordered by the API
	}
	return ids, nil
}

func postReorder(ctx context.Context, client *http.Client, base string, order []string) error {
	payload, _ := json.Marshal(map[string][]string{"order": order})
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, base+"/api/cards/reorder", strings.NewReader(string(payload)))
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("reorder status %d", resp.StatusCode)
	}
	return nil
}

func settleAndHold(ctx context.Context, st *stats, target int, hold time.Duration) {
	// Wait for the in-flight dials to resolve (active+failed ≈ attempted at this target).
	settleDeadline := time.After(20 * time.Second)
	for {
		if ctx.Err() != nil {
			return
		}
		if int(st.active.Load()+st.failed.Load()) >= target {
			break
		}
		select {
		case <-ctx.Done():
			return
		case <-settleDeadline:
			fmt.Printf("[warn] step %d did not fully settle (active=%d failed=%d)\n",
				target, st.active.Load(), st.failed.Load())
			goto held
		case <-time.After(200 * time.Millisecond):
		}
	}
held:
	select {
	case <-ctx.Done():
	case <-time.After(hold):
	}
}

func reporter(ctx context.Context, st *stats, done chan<- struct{}) {
	t := time.NewTicker(time.Second)
	defer t.Stop()
	var lastKA, lastEv int64
	for {
		select {
		case <-ctx.Done():
			close(done)
			return
		case <-t.C:
			ka := st.keepalives.Load()
			ev := st.events.Load()
			fmt.Printf("  active=%-5d est=%-5d fail=%-4d ka=%-6d(+%d) ev=%-6d(+%d) ttfb=%dms\n",
				st.active.Load(), st.established.Load(), st.failed.Load(),
				ka, ka-lastKA, ev, ev-lastEv, avgTTFB(st))
			lastKA, lastEv = ka, ev
		}
	}
}

func avgTTFB(st *stats) int64 {
	n := st.ttfbN.Load()
	if n == 0 {
		return 0
	}
	return st.ttfbSumMs.Load() / n
}

func finalReport(st *stats) {
	fmt.Printf("\n──────── ceiling drill summary ────────\n")
	fmt.Printf("  dials attempted : %d\n", st.attempted.Load())
	fmt.Printf("  established     : %d\n", st.established.Load())
	fmt.Printf("  failed          : %d\n", st.failed.Load())
	fmt.Printf("  peak concurrent : (see max active in stream above)\n")
	fmt.Printf("  keepalives recv : %d\n", st.keepalives.Load())
	fmt.Printf("  events recv     : %d\n", st.events.Load())
	fmt.Printf("  avg TTFB        : %dms\n", avgTTFB(st))
}

func parseSteps(csv string) []int {
	var out []int
	for _, p := range strings.Split(csv, ",") {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		n, err := strconv.Atoi(p)
		if err == nil && n > 0 {
			out = append(out, n)
		}
	}
	return out
}
