// Command dependencysecurity checks the committed browser dependency inventory.
package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

const vendor = "internal/frontend/static/vendor/"
const datastarTemplate = "internal/frontend/layouts/datastar.templ"

type module struct {
	Path    string `json:"path"`
	Package string `json:"package"`
	Version string `json:"version"`
	SHA256  string `json:"sha256"`
}
type manifest struct {
	Schema  int      `json:"schema"`
	Modules []module `json:"modules"`
}

func main() {
	mode := flag.String("mode", "integrity", "integrity (offline) or advisories (OSV and GitHub)")
	flag.Parse()
	var err error
	if flag.NArg() != 0 {
		err = errors.New("unexpected positional arguments; run from the repository root")
	} else {
		switch *mode {
		case "integrity":
			err = integrity(".", os.Stdout)
		case "advisories":
			c := checker{client: &http.Client{Timeout: 30 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return errors.New("unexpected API redirect") }}, osv: "https://api.osv.dev/v1/querybatch", github: "https://api.github.com/repos/starfederation/datastar/security-advisories", token: os.Getenv("GITHUB_TOKEN"), out: os.Stdout}
			err = c.advisories(".")
		default:
			err = fmt.Errorf("unknown mode %q", *mode)
		}
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "dependencysecurity:", err)
		os.Exit(1)
	}
}

func decode(data []byte, dst any) error {
	d := json.NewDecoder(bytes.NewReader(data))
	if err := d.Decode(dst); err != nil {
		return err
	}
	if err := d.Decode(new(any)); err != io.EOF {
		return errors.New("expected exactly one JSON value")
	}
	return nil
}

func inventory(root string) ([]module, error) {
	data, err := os.ReadFile(filepath.Join(root, vendor, "codemirror/manifest.json"))
	if err != nil {
		return nil, err
	}
	var m manifest
	if err := decode(data, &m); err != nil {
		return nil, fmt.Errorf("CodeMirror manifest: %w", err)
	}
	if m.Schema != 1 || len(m.Modules) == 0 {
		return nil, errors.New("CodeMirror manifest: require schema 1 and nonempty modules")
	}
	seen := map[string]bool{}
	for _, mod := range m.Modules {
		if !safePath(mod.Path) || seen[mod.Path] {
			return nil, fmt.Errorf("invalid or duplicate module path %q", mod.Path)
		}
		if !validHash(mod.SHA256) || strings.TrimSpace(mod.Package) == "" || strings.TrimSpace(mod.Version) == "" {
			return nil, fmt.Errorf("incomplete module %q", mod.Path)
		}
		seen[mod.Path] = true
	}
	base := filepath.Join(root, vendor, "codemirror")
	if err := filepath.WalkDir(base, func(name string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("symlink in inventory: %s", name)
		}
		if entry.IsDir() || (filepath.Ext(name) != ".mjs" && filepath.Ext(name) != ".js") {
			return nil
		}
		rel, err := filepath.Rel(base, name)
		if err != nil {
			return err
		}
		if !seen[filepath.ToSlash(rel)] {
			return fmt.Errorf("CodeMirror module missing from manifest: %s", rel)
		}
		return nil
	}); err != nil {
		return nil, err
	}
	return m.Modules, nil
}

func safePath(p string) bool {
	return p != "." && p != "" && path.Clean(p) == p && !strings.ContainsAny(p, "\\:\x00") && !path.IsAbs(p) && p != ".." && !strings.HasPrefix(p, "../")
}
func validHash(s string) bool {
	b, err := hex.DecodeString(s)
	return err == nil && len(b) == sha256.Size
}

func verifyFile(base, name, want string) error {
	if !safePath(name) || !validHash(want) {
		return fmt.Errorf("invalid checksum entry %q", name)
	}
	// Reject symlinks, including parent directories, rather than hashing outside the inventory.
	file := base
	for _, part := range strings.Split(name, "/") {
		file = filepath.Join(file, part)
		info, err := os.Lstat(file)
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("symlink in inventory: %s", file)
		}
	}
	info, err := os.Stat(file)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("not a regular file: %s", file)
	}
	data, err := os.ReadFile(file)
	if err != nil {
		return err
	}
	got := fmt.Sprintf("%x", sha256.Sum256(data))
	if !strings.EqualFold(got, want) {
		return fmt.Errorf("SHA256 mismatch: %s (got %s, want %s)", file, got, want)
	}
	return nil
}

var pinnedBundle = regexp.MustCompile(`src\s*=\s*["'](/static/vendor/datastar/[^"']+)["']`)

func integrity(root string, out io.Writer) error {
	mods, err := inventory(root)
	if err != nil {
		return err
	}
	for _, mod := range mods {
		if err := verifyFile(filepath.Join(root, vendor, "codemirror"), mod.Path, mod.SHA256); err != nil {
			return err
		}
	}
	fmt.Fprintf(out, "CodeMirror: verified %d module SHA256 checksums\n", len(mods))
	base := filepath.Join(root, vendor, "datastar")
	data, err := os.ReadFile(filepath.Join(base, "SHA256SUMS"))
	if err != nil {
		return err
	}
	sums := map[string]bool{}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		if len(line) < 67 || line[64] != ' ' || (line[65] != ' ' && line[65] != '*') {
			return errors.New("malformed Datastar SHA256SUMS entry")
		}
		name := line[66:]
		if sums[name] {
			return fmt.Errorf("duplicate Datastar checksum: %s", name)
		}
		if err := verifyFile(base, name, line[:64]); err != nil {
			return err
		}
		sums[name] = true
	}
	if len(sums) == 0 {
		return errors.New("empty Datastar checksum inventory")
	}
	template, err := os.ReadFile(filepath.Join(root, datastarTemplate))
	if err != nil {
		return err
	}
	pins := pinnedBundle.FindAllSubmatch(template, -1)
	if len(pins) != 1 {
		return errors.New("expected exactly one pinned Datastar bundle in datastar.templ")
	}
	pin := strings.TrimPrefix(string(pins[0][1]), "/static/vendor/datastar/")
	if !strings.HasPrefix(pin, "datastar-") || !strings.HasSuffix(pin, ".js") || !sums[pin] {
		return fmt.Errorf("pinned Datastar bundle %q not covered by SHA256SUMS", pin)
	}
	fmt.Fprintf(out, "Datastar: verified %d SHA256 checksums; pinned %s covered\n", len(sums), pin)
	return nil
}

type packageVersion struct{ Name, Version string }

func packages(mods []module) []packageVersion {
	seen := map[packageVersion]bool{}
	for _, m := range mods {
		if m.Version != "builtin" {
			seen[packageVersion{m.Package, m.Version}] = true
		}
	}
	result := make([]packageVersion, 0, len(seen))
	for p := range seen {
		result = append(result, p)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Name == result[j].Name {
			return result[i].Version < result[j].Version
		}
		return result[i].Name < result[j].Name
	})
	return result
}

type checker struct {
	client             *http.Client
	osv, github, token string
	out                io.Writer
}

func (c checker) request(method, url string, body []byte, github bool, dst any) (http.Header, error) {
	req, err := http.NewRequest(method, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "unbusy-dependencysecurity")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if github {
		req.Header.Set("Accept", "application/vnd.github+json")
		req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
		if c.token != "" {
			req.Header.Set("Authorization", "Bearer "+c.token)
		}
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s: HTTP %d", url, resp.StatusCode)
	}
	const limit = 32 << 20
	data, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if len(data) > limit {
		return nil, fmt.Errorf("%s: response too large", url)
	}
	if err := decode(data, dst); err != nil {
		return nil, fmt.Errorf("%s: malformed response: %w", url, err)
	}
	return resp.Header, nil
}

func (c checker) advisories(root string) error {
	mods, err := inventory(root)
	if err != nil {
		return err
	}
	pkgs := packages(mods)
	if len(pkgs) == 0 {
		return errors.New("no npm packages in CodeMirror inventory")
	}
	// Run both sources even when one finds vulnerabilities or is unavailable.
	return errors.Join(c.osvAdvisories(pkgs), c.datastarAdvisories())
}

func (c checker) osvAdvisories(pkgs []packageVersion) error {
	findings := 0
	for start := 0; start < len(pkgs); start += 1000 {
		batch := pkgs[start:min(start+1000, len(pkgs))]
		queries := make([]any, 0, len(batch))
		for _, p := range batch {
			queries = append(queries, map[string]any{"package": map[string]string{"ecosystem": "npm", "name": p.Name}, "version": p.Version})
		}
		body, err := json.Marshal(map[string]any{"queries": queries})
		if err != nil {
			return err
		}
		var response struct {
			Results []map[string]json.RawMessage `json:"results"`
			Error   json.RawMessage              `json:"error"`
		}
		if _, err := c.request(http.MethodPost, c.osv, body, false, &response); err != nil {
			return fmt.Errorf("OSV: %w", err)
		}
		if response.Error != nil || len(response.Results) != len(batch) {
			return errors.New("OSV: error or missing/mismatched results")
		}
		for i, result := range response.Results {
			if result == nil {
				return errors.New("OSV: null result")
			}
			for key := range result {
				if key != "vulns" && key != "next_page_token" {
					return fmt.Errorf("OSV: unexpected result field %q", key)
				}
			}
			if raw, ok := result["next_page_token"]; ok {
				var token string
				if err := json.Unmarshal(raw, &token); err != nil || token != "" || string(raw) == "null" {
					return errors.New("OSV: incomplete paginated result; manual review required")
				}
			}
			if raw, ok := result["vulns"]; ok {
				var vulns []struct {
					ID string `json:"id"`
				}
				if err := json.Unmarshal(raw, &vulns); err != nil || vulns == nil {
					return errors.New("OSV: malformed vulnerabilities")
				}
				for _, v := range vulns {
					if strings.TrimSpace(v.ID) == "" {
						return errors.New("OSV: vulnerability missing ID")
					}
					fmt.Fprintf(c.out, "OSV: %s@%s: %s\n", batch[i].Name, batch[i].Version, v.ID)
					findings++
				}
			}
		}
	}
	fmt.Fprintf(c.out, "OSV: checked %d unique npm package/version pairs; %d findings\n", len(pkgs), findings)
	if findings != 0 {
		return fmt.Errorf("OSV: %d vulnerability findings", findings)
	}
	return nil
}

func (c checker) datastarAdvisories() error {
	findings := 0
	seen := map[string]bool{}
	for page := 1; page <= 10000; page++ {
		var advisories []struct {
			ID        string `json:"ghsa_id"`
			State     string `json:"state"`
			Published string `json:"published_at"`
		}
		url := fmt.Sprintf("%s?state=published&per_page=100&page=%d", c.github, page)
		headers, err := c.request(http.MethodGet, url, nil, true, &advisories)
		if err != nil {
			return fmt.Errorf("Datastar GitHub: %w", err)
		}
		if advisories == nil {
			return errors.New("Datastar GitHub: expected advisory array, not null")
		}
		for _, a := range advisories {
			if strings.TrimSpace(a.ID) == "" || a.State != "published" {
				return errors.New("Datastar GitHub: malformed published advisory")
			}
			if _, err := time.Parse(time.RFC3339, a.Published); err != nil {
				return errors.New("Datastar GitHub: missing/invalid published_at")
			}
			if seen[a.ID] {
				return errors.New("Datastar GitHub: duplicate advisory across pages")
			}
			seen[a.ID] = true
			fmt.Fprintf(c.out, "Datastar: %s — manual applicability review required (no version-range inference)\n", a.ID)
			findings++
		}
		// Numbered pages avoid following an arbitrary Link URL with the GitHub token.
		if len(advisories) < 100 && !strings.Contains(headers.Get("Link"), `rel="next"`) {
			fmt.Fprintf(c.out, "Datastar GitHub: %d published repository advisories\n", findings)
			if findings != 0 {
				return fmt.Errorf("Datastar: %d advisories require manual applicability review", findings)
			}
			return nil
		}
	}
	return errors.New("Datastar GitHub: pagination limit exceeded")
}
