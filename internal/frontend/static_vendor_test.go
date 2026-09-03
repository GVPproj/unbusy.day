package frontend

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io/fs"
	"path"
	"regexp"
	"strings"
	"testing"
)

const codeMirrorVendorRoot = "static/vendor/codemirror"

type codeMirrorManifest struct {
	Schema  int                `json:"schema"`
	Modules []codeMirrorModule `json:"modules"`
}

type codeMirrorModule struct {
	Path         string `json:"path"`
	Package      string `json:"package"`
	Version      string `json:"version"`
	SourceURL    string `json:"sourceURL"`
	SourceSHA256 string `json:"sourceSHA256"`
	SHA256       string `json:"sha256"`
}

var localModuleReference = regexp.MustCompile(`["']\./([^"']+\.mjs)["']`)

func TestVendoredCodeMirrorManifestMatchesEmbeddedGraph(t *testing.T) {
	manifestBytes, err := staticFS.ReadFile(codeMirrorVendorRoot + "/manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	var manifest codeMirrorManifest
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		t.Fatalf("parse manifest: %v", err)
	}
	if manifest.Schema != 1 {
		t.Fatalf("schema: want 1, got %d", manifest.Schema)
	}

	listed := make(map[string]bool, len(manifest.Modules))
	versions := make(map[string]string)
	for _, module := range manifest.Modules {
		if listed[module.Path] {
			t.Fatalf("duplicate manifest path %q", module.Path)
		}
		listed[module.Path] = true
		if module.Package == "" || module.Version == "" || module.SourceURL == "" {
			t.Errorf("%s: package, version, and sourceURL are required", module.Path)
		}
		if !strings.HasPrefix(module.Path, "modules/") || path.Clean(module.Path) != module.Path {
			t.Errorf("invalid module path %q", module.Path)
		}
		if !strings.HasPrefix(module.SourceURL, "https://esm.sh/") {
			t.Errorf("%s: unexpected source URL %q", module.Path, module.SourceURL)
		}
		if sourceHash, err := hex.DecodeString(module.SourceSHA256); err != nil || len(sourceHash) != sha256.Size {
			t.Errorf("%s: invalid source SHA-256", module.Path)
		}
		if module.Version != "builtin" && !strings.Contains(module.SourceURL, "@"+module.Version+"/") {
			t.Errorf("%s: source URL does not contain version %s", module.Path, module.Version)
		}
		if old, ok := versions[module.Package]; ok && old != module.Version {
			t.Errorf("%s resolves to both %s and %s", module.Package, old, module.Version)
		}
		versions[module.Package] = module.Version

		data, err := staticFS.ReadFile(codeMirrorVendorRoot + "/" + module.Path)
		if err != nil {
			t.Errorf("%s: %v", module.Path, err)
			continue
		}
		sum := sha256.Sum256(data)
		if got := hex.EncodeToString(sum[:]); got != module.SHA256 {
			t.Errorf("%s: SHA-256 got %s, want %s", module.Path, got, module.SHA256)
		}
		text := string(data)
		if strings.Contains(text, "esm.sh/") {
			t.Errorf("%s still imports esm.sh", module.Path)
		}
		if strings.Contains(text, "import(") {
			t.Errorf("%s contains an unverified dynamic import", module.Path)
		}
		for _, match := range localModuleReference.FindAllStringSubmatch(text, -1) {
			dependency := path.Join(path.Dir(module.Path), match[1])
			if !listed[dependency] {
				// The complete manifest is known only after this loop.
				if _, err := staticFS.ReadFile(codeMirrorVendorRoot + "/" + dependency); err != nil {
					t.Errorf("%s references missing module %s", module.Path, dependency)
				}
			}
		}
	}

	err = fs.WalkDir(staticFS, codeMirrorVendorRoot+"/modules", func(name string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		rel := strings.TrimPrefix(name, codeMirrorVendorRoot+"/")
		if !listed[rel] {
			t.Errorf("unlisted vendored module %s", rel)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestJotpadCodeMirrorImportsOnlyLocalMarkdownModules(t *testing.T) {
	data, err := staticFS.ReadFile("static/js/jot/cm.js")
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if strings.Contains(text, "esm.sh") {
		t.Error("cm.js must not import esm.sh")
	}
	if strings.Contains(text, "language-data") || strings.Contains(text, "codeLanguages") {
		t.Error("the Jotpad must not load fenced-code language packages")
	}
	if !strings.Contains(text, "/static/vendor/codemirror/") {
		t.Error("cm.js does not import the vendored CodeMirror graph")
	}
}
