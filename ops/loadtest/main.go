// Command loadtest is the connection-ceiling drill: a concurrent-SSE ramp
// against the live origin. Each virtual client holds one GET /events over
// forced HTTP/1.1 (h2 would multiplex and hide the conn ceiling); at peak it
// fires a real layout and measures how completely fan-out reaches the fleet.
//
// Usage (run from repo root):
//
//	go run ./ops/loadtest -url https://hello-cards.fly.dev \
//	  -steps 250,500,1000,1500,2000 -hold 12s -peak-hold 45s
//
// Hit *.fly.dev directly — Cloudflare would pool origin conns and muddy the count.
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
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/starfederation/datastar-go/datastar"
)

type stats struct {
	attempted   atomic.Int64 // dials started
	established atomic.Int64 // got 200 + headers
	failed      atomic.Int64 // dial/handshake/non-200/early-EOF
	active      atomic.Int64 // currently held open
	keepalives  atomic.Int64 // :keepalive frames received (fleet-wide)
	events      atomic.Int64 // datastar-patch-elements frames received (fleet-wide)
	ttfbSumMs   atomic.Int64
	ttfbN       atomic.Int64
}

func main() {
	var (
		url       = flag.String("url", "https://hello-cards.fly.dev", "origin base URL")
		stepsCSV  = flag.String("steps", "250,500,1000,1500,2000", "cumulative concurrent targets")
		hold      = flag.Duration("hold", 12*time.Second, "hold time per step")
		peakHold  = flag.Duration("peak-hold", 45*time.Second, "hold at final step (cross a 25s keepalive boundary)")
		dialBurst = flag.Int("burst", 100, "max simultaneous in-flight dials when ramping")
		probeEach = flag.Bool("probe-each", false, "fire a fan-out probe at the end of every step (maps the ~1s sync SLO vs load)")
	)
	flag.Parse()

	steps := parseSteps(*stepsCSV)
	if len(steps) == 0 {
		fmt.Fprintln(os.Stderr, "no steps")
		os.Exit(1)
	}

	// DisableKeepAlives + an empty TLSNextProto map pins HTTP/1.1 so
	// 1 client == 1 origin connection.
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
				// Release the dial slot when the handshake resolves, NOT when
				// the stream closes — otherwise sem caps held connections
				// instead of pacing dials and the ramp wedges at -burst.
				var once sync.Once
				release := func() { once.Do(func() { <-sem }) }
				holdConn(rootCtx, client, *url, st, release)
			}()
			opened++
		}
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

// holdConn opens one SSE stream and reads it until ctx cancels or the stream
// errors — the unit of "one concurrent connection". Every early-return path
// must call release so a failed dial doesn't leak its slot.
func holdConn(ctx context.Context, client *http.Client, base string, st *stats, release func()) {
	st.attempted.Add(1)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/events", nil)
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
		case strings.HasPrefix(line, "event: "+string(datastar.EventTypePatchElements)):
			// The first frame after connect is the snapshot; fanoutProbe
			// baselines off a counter delta, so it never skews a probe.
			st.events.Add(1)
		}
		if ctx.Err() != nil {
			return
		}
	}
}

// fanoutProbe fires a real layout commit and measures time-to-first-event and
// time-to-90%-of-active across the held fleet — the load-bearing signal.
func fanoutProbe(ctx context.Context, client *http.Client, base string, st *stats) {
	active := st.active.Load()
	layout, err := currentLayout(ctx, client, base)
	if err != nil {
		fmt.Printf("[probe] skipped: read layout failed: %v\n", err)
		return
	}

	e0 := st.events.Load()
	t0 := time.Now()
	if err := postLayout(ctx, client, base, layout); err != nil {
		fmt.Printf("[probe] layout POST failed: %v\n", err)
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

// placement mirrors block.Placement's wire shape.
type placement struct {
	ID   string `json:"id"`
	Slot int    `json:"slot"`
	Span int    `json:"span"`
}

// blockRe pulls id/span/slot from the rendered page. Only blocks carry
// data-id; the day-grid slots don't, so they never enter the layout.
var blockRe = regexp.MustCompile(`data-id="([^"]+)" data-span="(\d+)" data-slot="(\d+)"`)

// currentLayout scrapes the authoritative layout from the page at / — there is
// no JSON read endpoint; the frontend is HTML over the wire.
func currentLayout(ctx context.Context, client *http.Client, base string) ([]placement, error) {
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, base+"/", nil)
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		io.Copy(io.Discard, resp.Body)
		return nil, fmt.Errorf("page status %d", resp.StatusCode)
	}
	html, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	matches := blockRe.FindAllSubmatch(html, -1)
	if len(matches) == 0 {
		return nil, fmt.Errorf("no block placements found in page")
	}
	layout := make([]placement, len(matches))
	for i, m := range matches {
		span, _ := strconv.Atoi(string(m[2]))
		slot, _ := strconv.Atoi(string(m[3]))
		layout[i] = placement{ID: string(m[1]), Slot: slot, Span: span}
	}
	return layout, nil
}

// postLayout re-submits the current layout verbatim: an identity layout still
// publishes on commit, exercising fan-out while always passing validation.
func postLayout(ctx context.Context, client *http.Client, base string, layout []placement) error {
	// Datastar reads a non-GET body as the signals JSON object directly.
	payload, _ := json.Marshal(map[string][]placement{"layout": layout})
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, base+"/blocks/layout", strings.NewReader(string(payload)))
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("layout status %d", resp.StatusCode)
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
