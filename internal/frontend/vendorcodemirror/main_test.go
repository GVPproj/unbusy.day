package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGenerateDiscoversAndRewritesImports(t *testing.T) {
	fixture := newRegistryFixture(t)
	fixture.modules["/root@1.0.0/es2022/root.mjs"] = `import {value} from "/dep@^1.0.0";
export const lazy = import('/dynamic@~2.0.0/nested');
//# sourceMappingURL=root.mjs.map`
	fixture.resolutions["/dep@^1.0.0"] = "/dep@1.2.0/es2022/dep.mjs"
	fixture.resolutions["/dynamic@~2.0.0/nested"] = "/dynamic@2.1.0/es2022/nested.mjs"
	fixture.resolutions["/root@1.0.0"] = "/root@1.0.0/es2022/root.mjs"
	fixture.modules["/dep@1.2.0/es2022/dep.mjs"] = `import "/root@^1.0.0"; export const value = 1;`
	fixture.modules["/dynamic@2.1.0/es2022/nested.mjs"] = `export default 2;`
	fixture.licenses["root@1.0.0"] = "ROOT LICENSE"
	fixture.licenses["dep@1.2.0"] = "DEP LICENSE"
	fixture.licenses["dynamic@2.1.0"] = "DYNAMIC LICENSE"

	destination := filepath.Join(t.TempDir(), "vendor")
	if err := generate(fixture.config(destination, []string{"/root@1.0.0/es2022/root.mjs"}, true)); err != nil {
		t.Fatal(err)
	}

	root := readGenerated(t, destination, "modules/root__root.mjs")
	if !strings.Contains(root, `from "./dep__dep.mjs"`) {
		t.Errorf("static import was not rewritten:\n%s", root)
	}
	if !strings.Contains(root, `import('./dynamic__nested.mjs')`) {
		t.Errorf("dynamic import was not rewritten:\n%s", root)
	}
	if strings.Contains(root, "sourceMappingURL") {
		t.Errorf("source-map reference remains:\n%s", root)
	}
	if !strings.Contains(root, "ROOT LICENSE") {
		t.Errorf("license is missing:\n%s", root)
	}

	locked := readManifest(t, destination)
	if len(locked.Modules) != 3 {
		t.Fatalf("manifest modules: want 3, got %d", len(locked.Modules))
	}
	for _, item := range locked.Modules {
		data, err := os.ReadFile(filepath.Join(destination, filepath.FromSlash(item.Path)))
		if err != nil {
			t.Fatal(err)
		}
		if got := testDigest(data); got != item.SHA256 {
			t.Errorf("%s hash: got %s, want %s", item.Path, got, item.SHA256)
		}
	}
}

func TestGenerateContentLockRejectsSourceChangeAndRefreshAcceptsIt(t *testing.T) {
	fixture := newRegistryFixture(t)
	modulePath := "/root@1.0.0/es2022/root.mjs"
	fixture.modules[modulePath] = "export const value = 1;"
	fixture.licenses["root@1.0.0"] = "LICENSE"
	destination := filepath.Join(t.TempDir(), "vendor")

	if err := generate(fixture.config(destination, []string{modulePath}, true)); err != nil {
		t.Fatal(err)
	}
	before := snapshotDir(t, destination)
	fixture.modules[modulePath] = "export const value = 2;"

	err := generate(fixture.config(destination, []string{modulePath}, false))
	if err == nil {
		t.Fatal("default generation accepted changed source")
	}
	if !strings.Contains(err.Error(), fixture.server.URL+modulePath) ||
		!strings.Contains(err.Error(), "source SHA-256") ||
		!strings.Contains(err.Error(), "expected") || !strings.Contains(err.Error(), "actual") {
		t.Fatalf("unclear content-lock error: %v", err)
	}
	if after := snapshotDir(t, destination); after != before {
		t.Fatal("failed generation modified the existing vendor directory")
	}

	if err := generate(fixture.config(destination, []string{modulePath}, true)); err != nil {
		t.Fatal(err)
	}
	if after := snapshotDir(t, destination); after == before {
		t.Fatal("refresh did not update generated output")
	}
	locked := readManifest(t, destination)
	if got := locked.Modules[0].SourceSHA256; got != testDigest([]byte(fixture.modules[modulePath])) {
		t.Errorf("source hash: got %s", got)
	}
}

func TestGenerateContentLockRejectsChangedFinalModule(t *testing.T) {
	fixture := newRegistryFixture(t)
	modulePath := "/root@1.0.0/es2022/root.mjs"
	fixture.modules[modulePath] = "export const value = 1;"
	fixture.licenses["root@1.0.0"] = "ORIGINAL LICENSE"
	destination := filepath.Join(t.TempDir(), "vendor")
	if err := generate(fixture.config(destination, []string{modulePath}, true)); err != nil {
		t.Fatal(err)
	}
	before := snapshotDir(t, destination)
	fixture.licenses["root@1.0.0"] = "CHANGED LICENSE"

	err := generate(fixture.config(destination, []string{modulePath}, false))
	if err == nil {
		t.Fatal("default generation accepted changed final module")
	}
	if !strings.Contains(err.Error(), fixture.server.URL+modulePath) ||
		!strings.Contains(err.Error(), "module SHA-256") ||
		!strings.Contains(err.Error(), "expected") || !strings.Contains(err.Error(), "actual") {
		t.Fatalf("unclear content-lock error: %v", err)
	}
	if after := snapshotDir(t, destination); after != before {
		t.Fatal("failed generation modified the existing vendor directory")
	}
}

func TestGenerateRequiresRefreshToBootstrap(t *testing.T) {
	fixture := newRegistryFixture(t)
	destination := filepath.Join(t.TempDir(), "vendor")
	err := generate(fixture.config(destination, []string{"/root@1.0.0/es2022/root.mjs"}, false))
	if err == nil || !strings.Contains(err.Error(), "-refresh") {
		t.Fatalf("error: want missing-lock refresh guidance, got %v", err)
	}
}

func TestGenerateRemovesLezerNodeDebugCode(t *testing.T) {
	fixture := newRegistryFixture(t)
	modulePath := "/@lezer/lr@1.4.10/es2022/lr.mjs"
	fixture.modules[modulePath] = `import __Process$ from "/node/process.mjs";
var g=typeof __Process$<"u"&&__Process$.env&&/\bparse\b/.test(__Process$.env.LOG),A=null;
export {g};`
	fixture.licenses["@lezer/lr@1.4.10"] = "LEZER LICENSE"
	destination := filepath.Join(t.TempDir(), "vendor")

	if err := generate(fixture.config(destination, []string{modulePath}, true)); err != nil {
		t.Fatal(err)
	}
	body := readGenerated(t, destination, "modules/@lezer__lr__lr.mjs")
	if strings.Contains(body, "node/process") || strings.Contains(body, "__Process$") {
		t.Fatalf("Node debug code remains:\n%s", body)
	}
	if !strings.Contains(body, "var g=false,A=null;") {
		t.Fatalf("parse-debug expression was not disabled:\n%s", body)
	}
}

func TestGenerateRejectsChangedLezerPatternsWithoutReplacingDestination(t *testing.T) {
	const (
		modulePath = "/@lezer/lr@1.4.10/es2022/lr.mjs"
		nodeImport = `import __Process$ from "/node/process.mjs";`
		debugCode  = `var g=typeof __Process$<"u"&&__Process$.env&&/\bparse\b/.test(__Process$.env.LOG),A=null;`
	)
	tests := []struct {
		name string
		body string
		want string
	}{
		{"node import", debugCode, "Node process import"},
		{"parse debug expression", nodeImport, "parse-debug expression"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fixture := newRegistryFixture(t)
			fixture.modules[modulePath] = tt.body
			fixture.licenses["@lezer/lr@1.4.10"] = "LEZER LICENSE"
			destination := filepath.Join(t.TempDir(), "vendor")
			if err := os.MkdirAll(destination, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(destination, "sentinel"), []byte("keep"), 0o644); err != nil {
				t.Fatal(err)
			}

			err := generate(fixture.config(destination, []string{modulePath}, true))
			if err == nil || !strings.Contains(err.Error(), tt.want) || !strings.Contains(err.Error(), modulePath) {
				t.Fatalf("error: want clear %s failure, got %v", tt.want, err)
			}
			if got, err := os.ReadFile(filepath.Join(destination, "sentinel")); err != nil || string(got) != "keep" {
				t.Fatalf("existing destination changed: %q, %v", got, err)
			}
		})
	}
}

func TestRootInputsMatchCommittedManifest(t *testing.T) {
	locked, err := loadManifest(filepath.Join("..", "static", "vendor", "codemirror", "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := validateRoots(roots, esmBase, locked); err != nil {
		t.Error(err)
	}
}

func TestValidateRootsRejectsUnrecordedInput(t *testing.T) {
	locked := manifest{
		Roots: []string{"https://esm.test/root@1.0.0/es2022/root.mjs"},
		Modules: []module{{
			Package: "root", Version: "1.0.0", SourceURL: "https://esm.test/root@1.0.0/es2022/root.mjs",
		}},
	}
	err := validateRoots([]string{"/root@2.0.0/es2022/root.mjs"}, "https://esm.test", locked)
	if err == nil || !strings.Contains(err.Error(), "/root@2.0.0/es2022/root.mjs") {
		t.Fatalf("expected root mismatch, got %v", err)
	}
}

func TestValidateRootsRejectsChangingInputToLockedTransitiveModule(t *testing.T) {
	locked := manifest{
		Roots: []string{"https://esm.test/root@1.0.0/es2022/root.mjs"},
		Modules: []module{
			{Package: "root", Version: "1.0.0", SourceURL: "https://esm.test/root@1.0.0/es2022/root.mjs"},
			{Package: "dep", Version: "1.0.0", SourceURL: "https://esm.test/dep@1.0.0/es2022/dep.mjs"},
		},
	}
	err := validateRoots([]string{"/dep@1.0.0/es2022/dep.mjs"}, "https://esm.test", locked)
	if err == nil || !strings.Contains(err.Error(), "do not match") {
		t.Fatalf("expected root-set mismatch, got %v", err)
	}
}

func TestReplaceDirectoryRestoresDestinationWhenInstallFails(t *testing.T) {
	parent := t.TempDir()
	destination := filepath.Join(parent, "vendor")
	if err := os.Mkdir(destination, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(destination, "sentinel"), []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}

	err := replaceDirectory(filepath.Join(parent, "missing-staging-directory"), destination)
	if err == nil {
		t.Fatal("replacement unexpectedly succeeded")
	}
	if got, readErr := os.ReadFile(filepath.Join(destination, "sentinel")); readErr != nil || string(got) != "keep" {
		t.Fatalf("destination was not restored: %q, %v", got, readErr)
	}
}

func TestParsePackageReference(t *testing.T) {
	tests := []struct {
		reference string
		pkg       string
		version   string
		module    string
	}{
		{"/crelt@1.0.7", "crelt", "1.0.7", ""},
		{"/crelt@1.0.7/es2022/crelt.mjs", "crelt", "1.0.7", "/es2022/crelt.mjs"},
		{"/@codemirror/view@6.43.11", "@codemirror/view", "6.43.11", ""},
		{"/@codemirror/lang-markdown@6.5.2/es2022/lang-markdown.mjs?target=es2022", "@codemirror/lang-markdown", "6.5.2", "/es2022/lang-markdown.mjs"},
		{"/@scope/pkg@^1.2.0/nested/module?dev", "@scope/pkg", "^1.2.0", "/nested/module"},
	}
	for _, tt := range tests {
		t.Run(tt.reference, func(t *testing.T) {
			got, err := parsePackageReference(tt.reference)
			if err != nil {
				t.Fatal(err)
			}
			if got.packageName != tt.pkg || got.version != tt.version || got.modulePath != tt.module {
				t.Fatalf("got %#v, want package=%q version=%q module=%q", got, tt.pkg, tt.version, tt.module)
			}
		})
	}

	for _, malformed := range []string{"", "/missing-version", "/@scope/pkg", "/@scope@1.0.0", "/pkg@", "/@scope/pkg@/file"} {
		t.Run("malformed "+malformed, func(t *testing.T) {
			if _, err := parsePackageReference(malformed); err == nil {
				t.Fatalf("accepted malformed reference %q", malformed)
			}
		})
	}
}

type registryFixture struct {
	t           *testing.T
	server      *httptest.Server
	modules     map[string]string
	resolutions map[string]string
	licenses    map[string]string
}

func newRegistryFixture(t *testing.T) *registryFixture {
	t.Helper()
	fixture := &registryFixture{
		t:           t,
		modules:     make(map[string]string),
		resolutions: make(map[string]string),
		licenses:    make(map[string]string),
	}
	fixture.server = httptest.NewServer(http.HandlerFunc(fixture.serveHTTP))
	t.Cleanup(fixture.server.Close)
	return fixture
}

func (f *registryFixture) config(destination string, rootModules []string, refresh bool) generationConfig {
	return generationConfig{
		roots:        rootModules,
		esmBase:      f.server.URL,
		registryBase: f.server.URL + "/registry",
		destination:  destination,
		client:       f.server.Client(),
		refresh:      refresh,
	}
}

func (f *registryFixture) serveHTTP(w http.ResponseWriter, r *http.Request) {
	if body, ok := f.modules[r.URL.Path]; ok {
		_, _ = w.Write([]byte(body))
		return
	}
	if exact, ok := f.resolutions[r.URL.Path]; ok {
		w.Header().Set("x-esm-path", exact)
		_, _ = w.Write([]byte("resolved"))
		return
	}
	if strings.HasPrefix(r.URL.Path, "/registry/") {
		metadataPath := strings.TrimPrefix(r.URL.Path, "/registry/")
		separator := strings.LastIndex(metadataPath, "/")
		if separator < 0 {
			http.NotFound(w, r)
			return
		}
		pkgVersion := metadataPath[:separator] + "@" + metadataPath[separator+1:]
		if _, ok := f.licenses[pkgVersion]; !ok {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"dist": map[string]string{"tarball": f.server.URL + "/tarball/" + pkgVersion},
		})
		return
	}
	if strings.HasPrefix(r.URL.Path, "/tarball/") {
		pkgVersion := strings.TrimPrefix(r.URL.Path, "/tarball/")
		license, ok := f.licenses[pkgVersion]
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/gzip")
		_, _ = w.Write(licenseArchive(f.t, license))
		return
	}
	http.Error(w, fmt.Sprintf("unhandled fixture URL %s", r.URL.String()), http.StatusNotFound)
}

func licenseArchive(t *testing.T, license string) []byte {
	t.Helper()
	var result bytes.Buffer
	gz := gzip.NewWriter(&result)
	tarWriter := tar.NewWriter(gz)
	if err := tarWriter.WriteHeader(&tar.Header{Name: "package/LICENSE", Mode: 0o644, Size: int64(len(license))}); err != nil {
		t.Fatal(err)
	}
	if _, err := tarWriter.Write([]byte(license)); err != nil {
		t.Fatal(err)
	}
	if err := tarWriter.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return result.Bytes()
}

func readGenerated(t *testing.T, destination, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(destination, filepath.FromSlash(name)))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func readManifest(t *testing.T, destination string) manifest {
	t.Helper()
	locked, err := loadManifest(filepath.Join(destination, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	return locked
}

func snapshotDir(t *testing.T, root string) string {
	t.Helper()
	var snapshot strings.Builder
	err := filepath.WalkDir(root, func(name string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, name)
		if err != nil {
			return err
		}
		data, err := os.ReadFile(name)
		if err != nil {
			return err
		}
		fmt.Fprintf(&snapshot, "%s:%s\n", filepath.ToSlash(rel), testDigest(data))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return snapshot.String()
}

func testDigest(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
