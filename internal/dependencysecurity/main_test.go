package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func put(t *testing.T, root, name, data string) {
	t.Helper()
	file := filepath.Join(root, name)
	if err := os.MkdirAll(filepath.Dir(file), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, []byte(data), 0644); err != nil {
		t.Fatal(err)
	}
}
func hash(s string) string { return fmt.Sprintf("%x", sha256.Sum256([]byte(s))) }
func writeManifest(t *testing.T, root string, mods []module) {
	t.Helper()
	data, err := json.Marshal(manifest{Schema: 1, Modules: mods})
	if err != nil {
		t.Fatal(err)
	}
	put(t, root, vendor+"codemirror/manifest.json", string(data))
}
func fixture(t *testing.T) (string, []module) {
	t.Helper()
	root := t.TempDir()
	mods := []module{{Path: "modules/a.mjs", Package: "@cm/a", Version: "1.2.3", SHA256: hash("module")}}
	writeManifest(t, root, mods)
	put(t, root, vendor+"codemirror/modules/a.mjs", "module")
	put(t, root, vendor+"datastar/datastar-1.0.4.js", "bundle")
	put(t, root, vendor+"datastar/LICENSE.md", "license")
	put(t, root, vendor+"datastar/SHA256SUMS", hash("bundle")+"  datastar-1.0.4.js\n"+hash("license")+" *LICENSE.md\n")
	put(t, root, datastarTemplate, `<script type="module" src="/static/vendor/datastar/datastar-1.0.4.js"></script>`)
	return root, mods
}
func TestIntegrity(t *testing.T) {
	root, _ := fixture(t)
	var out bytes.Buffer
	if err := integrity(root, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "verified 1 module") || !strings.Contains(out.String(), "pinned datastar-1.0.4.js covered") {
		t.Fatal(out.String())
	}
}
func TestIntegrityRejects(t *testing.T) {
	tests := map[string]func(*testing.T, string, []module){
		"missing manifest": func(t *testing.T, r string, _ []module) {
			if err := os.Remove(filepath.Join(r, vendor, "codemirror/manifest.json")); err != nil {
				t.Fatal(err)
			}
		},
		"empty inventory":   func(t *testing.T, r string, _ []module) { writeManifest(t, r, nil) },
		"missing inventory": func(t *testing.T, r string, _ []module) { put(t, r, vendor+"codemirror/manifest.json", `{"schema":1}`) },
		"bad schema": func(t *testing.T, r string, _ []module) {
			put(t, r, vendor+"codemirror/manifest.json", `{"schema":2,"modules":[]}`)
		},
		"malformed manifest": func(t *testing.T, r string, _ []module) { put(t, r, vendor+"codemirror/manifest.json", `{`) },
		"trailing JSON":      func(t *testing.T, r string, _ []module) { put(t, r, vendor+"codemirror/manifest.json", `{} {}`) },
		"missing module":     func(t *testing.T, r string, m []module) { m[0].Path = "missing"; writeManifest(t, r, m) },
		"modified module":    func(t *testing.T, r string, _ []module) { put(t, r, vendor+"codemirror/modules/a.mjs", "changed") },
		"bad hash":           func(t *testing.T, r string, m []module) { m[0].SHA256 = "not a hash"; writeManifest(t, r, m) },
		"missing package":    func(t *testing.T, r string, m []module) { m[0].Package = ""; writeManifest(t, r, m) },
		"missing version":    func(t *testing.T, r string, m []module) { m[0].Version = ""; writeManifest(t, r, m) },
		"duplicate module":   func(t *testing.T, r string, m []module) { writeManifest(t, r, append(m, m[0])) },
		"missing sums": func(t *testing.T, r string, _ []module) {
			if err := os.Remove(filepath.Join(r, vendor, "datastar/SHA256SUMS")); err != nil {
				t.Fatal(err)
			}
		},
		"empty sums":     func(t *testing.T, r string, _ []module) { put(t, r, vendor+"datastar/SHA256SUMS", "\n") },
		"malformed sums": func(t *testing.T, r string, _ []module) { put(t, r, vendor+"datastar/SHA256SUMS", "bad\n") },
		"duplicate sums": func(t *testing.T, r string, _ []module) {
			line := hash("bundle") + "  datastar-1.0.4.js\n"
			put(t, r, vendor+"datastar/SHA256SUMS", line+line)
		},
		"unsafe sums": func(t *testing.T, r string, _ []module) {
			put(t, r, vendor+"datastar/SHA256SUMS", hash("bundle")+"  ../datastar/datastar-1.0.4.js\n")
		},
		"modified bundle":  func(t *testing.T, r string, _ []module) { put(t, r, vendor+"datastar/datastar-1.0.4.js", "changed") },
		"modified license": func(t *testing.T, r string, _ []module) { put(t, r, vendor+"datastar/LICENSE.md", "changed") },
		"uncovered pin": func(t *testing.T, r string, _ []module) {
			put(t, r, vendor+"datastar/SHA256SUMS", hash("license")+"  LICENSE.md\n")
		},
		"missing pin": func(t *testing.T, r string, _ []module) { put(t, r, datastarTemplate, "no script") },
		"directory":   func(t *testing.T, r string, m []module) { m[0].Path = "modules"; writeManifest(t, r, m) },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			root, mods := fixture(t)
			mutate(t, root, mods)
			if err := integrity(root, io.Discard); err == nil {
				t.Fatal("accepted invalid inventory")
			}
		})
	}
	for _, p := range []string{"", ".", "..", "../escape", "/absolute", "modules/../../escape", "modules/../a", "modules//a", "./a", `C:\a`, `modules\a`, "a\x00b"} {
		t.Run("path="+p, func(t *testing.T) {
			root, mods := fixture(t)
			mods[0].Path = p
			writeManifest(t, root, mods)
			if err := integrity(root, io.Discard); err == nil {
				t.Fatal("accepted unsafe path")
			}
		})
	}
}
func TestIntegrityRejectsSymlinks(t *testing.T) {
	for _, parent := range []bool{false, true} {
		t.Run(fmt.Sprint(parent), func(t *testing.T) {
			root, mods := fixture(t)
			target := t.TempDir()
			put(t, target, "a.mjs", "module")
			link := filepath.Join(root, vendor, "codemirror/link")
			if parent {
				mods[0].Path = "link/a.mjs"
			} else {
				target = filepath.Join(target, "a.mjs")
				mods[0].Path = "link"
			}
			if err := os.Symlink(target, link); err != nil {
				t.Fatal(err)
			}
			writeManifest(t, root, mods)
			if err := integrity(root, io.Discard); err == nil {
				t.Fatal("accepted symlink")
			}
		})
	}
}
func TestInventoryRejectsUnlistedModules(t *testing.T) {
	for _, name := range []string{"modules/unlisted.mjs", "unlisted.js"} {
		t.Run(name, func(t *testing.T) {
			root, _ := fixture(t)
			put(t, root, vendor+"codemirror/"+name, "untracked code")
			if _, err := inventory(root); err == nil || !strings.Contains(err.Error(), "missing from manifest") {
				t.Fatalf("unlisted module accepted: %v", err)
			}
		})
	}
}

func TestPackages(t *testing.T) {
	got := packages([]module{{Package: "b", Version: "2"}, {Package: "a", Version: "1"}, {Package: "a", Version: "1"}, {Package: "a", Version: "2"}, {Package: "node", Version: "builtin"}})
	want := []packageVersion{{"a", "1"}, {"a", "2"}, {"b", "2"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func testChecker(t *testing.T, handler http.HandlerFunc, out io.Writer) checker {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	return checker{client: server.Client(), osv: server.URL + "/osv", github: server.URL + "/github", token: "test-token", out: out}
}

const published = `{"ghsa_id":"GHSA-test","state":"published","published_at":"2026-01-02T03:04:05Z"}`

func TestAdvisories(t *testing.T) {
	for _, tt := range []struct {
		name, osv, github, want string
		fail                    bool
	}{
		{"clean", `{"results":[{}]}`, `[]`, "0 findings", false},
		{"osv finding", `{"results":[{"vulns":[{"id":"GHSA-npm"},{"id":"CVE-test"}]}]}`, `[]`, "@cm/a@1.2.3: GHSA-npm", true},
		{"github finding", `{"results":[{}]}`, `[` + published + `]`, "GHSA-test — manual applicability review required", true},
		{"both findings", `{"results":[{"vulns":[{"id":"CVE-test"}]}]}`, `[` + published + `]`, "manual applicability review required", true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			root, mods := fixture(t)
			duplicate := mods[0]
			duplicate.Path = "modules/duplicate.mjs"
			builtin := mods[0]
			builtin.Path = "modules/builtin.mjs"
			builtin.Version = "builtin"
			writeManifest(t, root, append(mods, duplicate, builtin))
			var out bytes.Buffer
			calls := 0
			c := testChecker(t, func(w http.ResponseWriter, r *http.Request) {
				calls++
				switch r.URL.Path {
				case "/osv":
					if r.Method != http.MethodPost || r.Header.Get("Authorization") != "" || r.Header.Get("Content-Type") != "application/json" {
						t.Errorf("bad OSV request: %v", r)
					}
					var q struct {
						Queries []struct {
							Package struct{ Ecosystem, Name string }
							Version string
						}
					}
					if err := json.NewDecoder(r.Body).Decode(&q); err != nil {
						t.Error(err)
					}
					if len(q.Queries) != 1 || q.Queries[0].Package.Ecosystem != "npm" || q.Queries[0].Package.Name != "@cm/a" || q.Queries[0].Version != "1.2.3" {
						t.Errorf("bad queries: %+v", q)
					}
					fmt.Fprint(w, tt.osv)
				case "/github":
					if r.Method != http.MethodGet || r.Header.Get("Authorization") != "Bearer test-token" || r.URL.Query().Get("state") != "published" || r.URL.Query().Get("per_page") != "100" {
						t.Errorf("bad GitHub request: %v", r)
					}
					fmt.Fprint(w, tt.github)
				default:
					t.Errorf("unexpected URL: %s", r.URL)
				}
			}, &out)
			err := c.advisories(root)
			if (err != nil) != tt.fail {
				t.Fatalf("error = %v", err)
			}
			if calls != 2 || !strings.Contains(out.String(), tt.want) {
				t.Fatalf("calls=%d output=%s", calls, &out)
			}
		})
	}
}

func TestOSVFailClosed(t *testing.T) {
	for _, body := range []string{`{`, `null`, `{}`, `{"results":null}`, `{"results":[]}`, `{"results":[{},{}]}`, `{"results":[null]}`, `{"results":[{"error":"bad"}]}`, `{"results":[{"vulns":null}]}`, `{"results":[{"vulns":{}}]}`, `{"results":[{"vulns":[{}]}]}`, `{"results":[{"vulns":[null]}]}`, `{"results":[{"next_page_token":"more"}]}`, `{"results":[{"next_page_token":null}]}`, `{"results":[{}],"error":"bad"}`, `{"results":[{}]} {}`} {
		t.Run(body, func(t *testing.T) {
			c := testChecker(t, func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, body) }, io.Discard)
			if err := c.osvAdvisories([]packageVersion{{"a", "1"}}); err == nil {
				t.Fatal("accepted malformed/incomplete response")
			}
		})
	}
}
func TestGitHubFailClosed(t *testing.T) {
	for _, body := range []string{`{`, `null`, `{}`, `[null]`, `[{}]`, `[{"ghsa_id":"GHSA-a"}]`, `[{"ghsa_id":"GHSA-a","state":"draft","published_at":"2026-01-02T03:04:05Z"}]`, `[{"ghsa_id":"GHSA-a","state":"published","published_at":"invalid"}]`, `[] []`} {
		t.Run(body, func(t *testing.T) {
			c := testChecker(t, func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, body) }, io.Discard)
			if err := c.datastarAdvisories(); err == nil {
				t.Fatal("accepted malformed response")
			}
		})
	}
}
func TestHTTPFailures(t *testing.T) {
	for _, status := range []int{301, 401, 403, 429, 500} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			root, _ := fixture(t)
			calls := 0
			c := testChecker(t, func(w http.ResponseWriter, r *http.Request) { calls++; w.WriteHeader(status); fmt.Fprint(w, `{}`) }, io.Discard)
			err := c.advisories(root)
			if err == nil || !strings.Contains(err.Error(), "OSV:") || !strings.Contains(err.Error(), "Datastar GitHub:") || calls != 2 {
				t.Fatalf("calls=%d err=%v", calls, err)
			}
		})
	}
}
func TestHTTPTimeout(t *testing.T) {
	release := make(chan struct{})
	defer close(release)
	c := testChecker(t, func(w http.ResponseWriter, r *http.Request) { <-release }, io.Discard)
	c.client.Timeout = 20 * time.Millisecond
	if err := c.osvAdvisories([]packageVersion{{"a", "1"}}); err == nil {
		t.Fatal("expected timeout")
	}
}
func TestOSVBatches(t *testing.T) {
	var sizes []int
	c := testChecker(t, func(w http.ResponseWriter, r *http.Request) {
		var q struct{ Queries []json.RawMessage }
		if err := json.NewDecoder(r.Body).Decode(&q); err != nil {
			t.Error(err)
		}
		sizes = append(sizes, len(q.Queries))
		results := make([]map[string]any, len(q.Queries))
		for i := range results {
			results[i] = map[string]any{}
		}
		if err := json.NewEncoder(w).Encode(map[string]any{"results": results}); err != nil {
			t.Error(err)
		}
	}, io.Discard)
	pkgs := make([]packageVersion, 2001)
	for i := range pkgs {
		pkgs[i] = packageVersion{fmt.Sprintf("p%d", i), "1"}
	}
	if err := c.osvAdvisories(pkgs); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(sizes, []int{1000, 1000, 1}) {
		t.Fatal(sizes)
	}
}
func TestGitHubPagination(t *testing.T) {
	var out bytes.Buffer
	calls := 0
	c := testChecker(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Query().Get("page") != fmt.Sprint(calls) {
			t.Errorf("wrong page: %s", r.URL)
		}
		if r.Header.Get("Authorization") != "" {
			t.Error("unexpected token")
		}
		if calls == 1 {
			// Never follow this untrusted target; use the same API's numbered pages.
			w.Header().Set("Link", `<https://untrusted.invalid/?page=2>; rel="next"`)
			fmt.Fprint(w, `[`+published+`]`)
		} else {
			fmt.Fprint(w, `[{"ghsa_id":"GHSA-page2","state":"published","published_at":"2026-01-02T03:04:05Z"}]`)
		}
	}, &out)
	c.token = ""
	if err := c.datastarAdvisories(); err == nil || !strings.Contains(err.Error(), "2 advisories") {
		t.Fatalf("error = %v", err)
	}
	if calls != 2 || !strings.Contains(out.String(), "GHSA-page2") {
		t.Fatalf("calls=%d output=%s", calls, &out)
	}
}
func TestGitHubFullPage(t *testing.T) {
	calls := 0
	c := testChecker(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls > 1 {
			fmt.Fprint(w, `[]`)
			return
		}
		rows := make([]map[string]string, 100)
		for i := range rows {
			rows[i] = map[string]string{"ghsa_id": fmt.Sprintf("GHSA-%d", i), "state": "published", "published_at": "2026-01-02T03:04:05Z"}
		}
		if err := json.NewEncoder(w).Encode(rows); err != nil {
			t.Error(err)
		}
	}, io.Discard)
	if err := c.datastarAdvisories(); err == nil || !strings.Contains(err.Error(), "100 advisories") {
		t.Fatalf("error = %v", err)
	}
	if calls != 2 {
		t.Fatal(calls)
	}
}
func TestAdvisoriesRejectEmptyNPMInventory(t *testing.T) {
	root, mods := fixture(t)
	mods[0].Version = "builtin"
	writeManifest(t, root, mods)
	c := checker{out: io.Discard}
	if err := c.advisories(root); err == nil {
		t.Fatal("accepted empty npm inventory")
	}
}
