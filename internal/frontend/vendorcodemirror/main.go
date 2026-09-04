// Command vendorcodemirror refreshes the browser-ready CodeMirror module graph.
package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

const (
	esmBase      = "https://esm.sh"
	registryBase = "https://registry.npmjs.org"
	vendorRoot   = "internal/frontend/static/vendor/codemirror"
)

// roots are the direct upgrade inputs. The manifest locks the complete deployed graph.
var roots = []string{
	"/@codemirror/view@6.43.11/es2022/view.mjs",
	"/@codemirror/state@6.7.2/es2022/state.mjs",
	"/@codemirror/commands@6.10.4/es2022/commands.mjs",
	"/@codemirror/language@6.12.4/es2022/language.mjs",
	"/@lezer/highlight@1.2.3/es2022/highlight.mjs",
	"/@codemirror/lang-markdown@6.5.2/es2022/lang-markdown.mjs",
}

var (
	rootImport       = regexp.MustCompile(`(?:from|import)\s*["'](/[^"']+)["']|import\(\s*["'](/[^"']+)["']`)
	sourceMapComment = regexp.MustCompile(`(?m)\n?//# sourceMappingURL=.*$`)
)

const (
	lezerNodeImport = `import __Process$ from "/node/process.mjs";`
	lezerDebugCode  = `var g=typeof __Process$<"u"&&__Process$.env&&/\bparse\b/.test(__Process$.env.LOG),A=null;`
)

type manifest struct {
	Schema  int      `json:"schema"`
	Roots   []string `json:"roots"`
	Modules []module `json:"modules"`
}

type module struct {
	Path         string `json:"path"`
	Package      string `json:"package"`
	Version      string `json:"version"`
	SourceURL    string `json:"sourceURL"`
	SourceSHA256 string `json:"sourceSHA256"`
	SHA256       string `json:"sha256"`
}

type sourceModule struct {
	raw     []byte
	body    []byte
	imports map[string]string
}

type npmMetadata struct {
	Dist struct {
		Tarball string `json:"tarball"`
	} `json:"dist"`
}

type generationConfig struct {
	roots        []string
	esmBase      string
	registryBase string
	destination  string
	client       *http.Client
	refresh      bool
}

type packageReference struct {
	packageName string
	version     string
	modulePath  string
}

func main() {
	refresh := flag.Bool("refresh", false, "resolve versions and intentionally update the content lock")
	flag.Parse()

	check(generate(generationConfig{
		roots:        roots,
		esmBase:      esmBase,
		registryBase: registryBase,
		destination:  vendorRoot,
		client:       &http.Client{Timeout: 30 * time.Second},
		refresh:      *refresh,
	}))
}

func generate(config generationConfig) error {
	manifestPath := filepath.Join(config.destination, "manifest.json")
	locked, err := loadManifest(manifestPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) && !config.refresh {
			return fmt.Errorf("content lock %s is missing; run with -refresh to create it", manifestPath)
		}
		if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("load content lock: %w", err)
		}
	}
	if !config.refresh {
		if err := validateRoots(config.roots, config.esmBase, locked); err != nil {
			return err
		}
	}

	versions := lockedVersions(locked)
	if config.refresh {
		versions, err = rootVersions(config.roots)
		if err != nil {
			return err
		}
	}
	expected := modulesBySourceURL(locked)
	sources, err := fetchGraph(config, versions, expected)
	if err != nil {
		return err
	}
	return writeGraph(config, sources, expected)
}

func loadManifest(name string) (manifest, error) {
	data, err := os.ReadFile(name)
	if err != nil {
		return manifest{}, err
	}
	var current manifest
	if err := json.Unmarshal(data, &current); err != nil {
		return manifest{}, err
	}
	if current.Schema != 1 {
		return manifest{}, fmt.Errorf("unsupported manifest schema %d", current.Schema)
	}
	return current, nil
}

func lockedVersions(current manifest) map[string]string {
	locked := make(map[string]string, len(current.Modules))
	for _, item := range current.Modules {
		locked[item.Package] = item.Version
	}
	return locked
}

func rootVersions(rootModules []string) (map[string]string, error) {
	versions := make(map[string]string, len(rootModules))
	for _, root := range rootModules {
		reference, err := parsePackageReference(root)
		if err != nil {
			return nil, fmt.Errorf("root %s: %w", root, err)
		}
		if selected := versions[reference.packageName]; selected != "" && selected != reference.version {
			return nil, fmt.Errorf("roots select both %s@%s and %s@%s", reference.packageName, selected, reference.packageName, reference.version)
		}
		versions[reference.packageName] = reference.version
	}
	return versions, nil
}

func modulesBySourceURL(current manifest) map[string]module {
	locked := make(map[string]module, len(current.Modules))
	for _, item := range current.Modules {
		locked[item.SourceURL] = item
	}
	return locked
}

func validateRoots(rootModules []string, baseURL string, current manifest) error {
	locked := modulesBySourceURL(current)
	expected := make([]string, 0, len(rootModules))
	for _, root := range rootModules {
		reference, err := parsePackageReference(root)
		if err != nil {
			return fmt.Errorf("root %s: %w", root, err)
		}
		sourceURL := strings.TrimRight(baseURL, "/") + root
		item, ok := locked[sourceURL]
		if !ok || item.Package != reference.packageName || item.Version != reference.version {
			return fmt.Errorf("root input %s is not recorded exactly in the generated manifest; run with -refresh", root)
		}
		expected = append(expected, sourceURL)
	}
	sort.Strings(expected)
	recorded := append([]string(nil), current.Roots...)
	sort.Strings(recorded)
	if strings.Join(expected, "\n") != strings.Join(recorded, "\n") {
		return fmt.Errorf("root inputs do not match the generated manifest; run with -refresh")
	}
	return nil
}

func fetchGraph(config generationConfig, locked map[string]string, expected map[string]module) (map[string]sourceModule, error) {
	queue := append([]string(nil), config.roots...)
	sources := make(map[string]sourceModule)
	baseURL := strings.TrimRight(config.esmBase, "/")
	for len(queue) > 0 {
		modulePath := queue[0]
		queue = queue[1:]
		if _, ok := sources[modulePath]; ok {
			continue
		}
		sourceURL := baseURL + modulePath
		raw, _, err := get(config.client, sourceURL)
		if err != nil {
			return nil, err
		}
		if !config.refresh {
			item, ok := expected[sourceURL]
			if !ok {
				return nil, fmt.Errorf("source %s is absent from the content lock; run with -refresh", sourceURL)
			}
			if actual := digest(raw); actual != item.SourceSHA256 {
				return nil, hashMismatch("source", sourceURL, item.SourceSHA256, actual)
			}
		}
		body, err := normalize(modulePath, raw)
		if err != nil {
			return nil, err
		}
		imports := make(map[string]string)
		for _, match := range rootImport.FindAllSubmatch(body, -1) {
			spec := string(match[1])
			if spec == "" {
				spec = string(match[2])
			}
			exact, err := resolve(config, spec, locked)
			if err != nil {
				return nil, fmt.Errorf("resolve %s: %w", spec, err)
			}
			imports[spec] = exact
			queue = append(queue, exact)
		}
		sources[modulePath] = sourceModule{raw: raw, body: body, imports: imports}
	}
	return sources, nil
}

func resolve(config generationConfig, spec string, locked map[string]string) (string, error) {
	if strings.Contains(spec, "/es2022/") {
		return strings.Split(spec, "?")[0], nil
	}
	requestPath := spec
	reference, err := parsePackageReference(spec)
	if err != nil {
		return "", err
	}
	selectedVersion := locked[reference.packageName]
	if selectedVersion != "" {
		requestPath = "/" + reference.packageName + "@" + selectedVersion + reference.modulePath + "?target=es2022"
	}
	_, header, err := get(config.client, strings.TrimRight(config.esmBase, "/")+requestPath)
	if err != nil {
		return "", err
	}
	exact := header.Get("x-esm-path")
	if exact == "" {
		return "", fmt.Errorf("response has no x-esm-path")
	}
	resolved, err := parsePackageReference(exact)
	if err != nil {
		return "", fmt.Errorf("invalid x-esm-path %q: %w", exact, err)
	}
	if resolved.packageName != reference.packageName {
		return "", fmt.Errorf("x-esm-path %q resolved unexpected package %s", exact, resolved.packageName)
	}
	if selectedVersion != "" && resolved.version != selectedVersion {
		return "", fmt.Errorf("x-esm-path %q has version %s, want %s", exact, resolved.version, selectedVersion)
	}
	locked[reference.packageName] = resolved.version
	return exact, nil
}

func parsePackageReference(reference string) (packageReference, error) {
	value := strings.TrimPrefix(strings.SplitN(reference, "?", 2)[0], "/")
	var separator int
	if strings.HasPrefix(value, "@") {
		scopeEnd := strings.Index(value, "/")
		if scopeEnd < 2 {
			return packageReference{}, fmt.Errorf("invalid package reference %q", reference)
		}
		relative := strings.Index(value[scopeEnd+1:], "@")
		if relative <= 0 {
			return packageReference{}, fmt.Errorf("invalid package reference %q", reference)
		}
		separator = scopeEnd + 1 + relative
	} else {
		separator = strings.Index(value, "@")
		if separator <= 0 {
			return packageReference{}, fmt.Errorf("invalid package reference %q", reference)
		}
	}

	versionEnd := strings.Index(value[separator+1:], "/")
	if versionEnd < 0 {
		versionEnd = len(value)
	} else {
		versionEnd += separator + 1
	}
	if separator+1 == versionEnd {
		return packageReference{}, fmt.Errorf("invalid package reference %q", reference)
	}
	return packageReference{
		packageName: value[:separator],
		version:     value[separator+1 : versionEnd],
		modulePath:  value[versionEnd:],
	}, nil
}

func normalize(modulePath string, body []byte) ([]byte, error) {
	reference, err := parsePackageReference(modulePath)
	if err != nil {
		return nil, fmt.Errorf("normalize %s: %w", modulePath, err)
	}
	text := sourceMapComment.ReplaceAllString(string(body), "\n")
	if reference.packageName == "@lezer/lr" {
		if count := strings.Count(text, lezerNodeImport); count != 1 {
			return nil, fmt.Errorf("normalize %s: expected one Lezer Node process import, found %d", modulePath, count)
		}
		if count := strings.Count(text, lezerDebugCode); count != 1 {
			return nil, fmt.Errorf("normalize %s: expected one Lezer parse-debug expression, found %d", modulePath, count)
		}
		text = strings.Replace(text, lezerNodeImport, "", 1)
		text = strings.Replace(text, lezerDebugCode, `var g=false,A=null;`, 1)
	}
	return []byte(text), nil
}

func writeGraph(config generationConfig, sources map[string]sourceModule, expected map[string]module) error {
	parent := filepath.Dir(config.destination)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return err
	}
	temporary, err := os.MkdirTemp(parent, ".codemirror-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(temporary)
	if err := os.MkdirAll(filepath.Join(temporary, "modules"), 0o755); err != nil {
		return err
	}

	paths := make([]string, 0, len(sources))
	for modulePath := range sources {
		paths = append(paths, modulePath)
	}
	sort.Strings(paths)
	baseURL := strings.TrimRight(config.esmBase, "/")
	rootURLs := make([]string, 0, len(config.roots))
	for _, root := range config.roots {
		rootURLs = append(rootURLs, baseURL+root)
	}
	sort.Strings(rootURLs)
	result := manifest{Schema: 1, Roots: rootURLs}
	licenses := make(map[string]string)
	seen := make(map[string]bool, len(paths))
	for _, modulePath := range paths {
		source := sources[modulePath]
		reference, err := parsePackageReference(modulePath)
		if err != nil {
			return err
		}
		licenseKey := reference.packageName + "@" + reference.version
		license := licenses[licenseKey]
		if license == "" {
			license, err = fetchLicense(config, reference.packageName, reference.version)
			if err != nil {
				return err
			}
			licenses[licenseKey] = license
		}
		text := string(source.body)
		for spec, exact := range source.imports {
			local, err := localModuleFilename(exact)
			if err != nil {
				return err
			}
			text = strings.ReplaceAll(text, `"`+spec+`"`, `"./`+local+`"`)
			text = strings.ReplaceAll(text, `'`+spec+`'`, `'./`+local+`'`)
		}
		text = "/*!\n" + strings.TrimSpace(license) + "\n*/\n" + text
		fileName, err := localModuleFilename(modulePath)
		if err != nil {
			return err
		}
		localPath := "modules/" + fileName
		data := []byte(text)
		sourceURL := baseURL + modulePath
		if !config.refresh {
			item, ok := expected[sourceURL]
			if !ok {
				return fmt.Errorf("module %s is absent from the content lock; run with -refresh", sourceURL)
			}
			if actual := digest(data); actual != item.SHA256 {
				return hashMismatch("module", sourceURL, item.SHA256, actual)
			}
		}
		if err := os.WriteFile(filepath.Join(temporary, filepath.FromSlash(localPath)), data, 0o644); err != nil {
			return err
		}
		seen[sourceURL] = true
		result.Modules = append(result.Modules, module{
			Path:         localPath,
			Package:      reference.packageName,
			Version:      reference.version,
			SourceURL:    sourceURL,
			SourceSHA256: digest(source.raw),
			SHA256:       digest(data),
		})
	}
	if !config.refresh {
		for sourceURL := range expected {
			if !seen[sourceURL] {
				return fmt.Errorf("locked module %s is absent from the generated graph; run with -refresh", sourceURL)
			}
		}
	}
	if err := validateRoots(config.roots, config.esmBase, result); err != nil {
		return err
	}

	manifestData, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return err
	}
	manifestData = append(manifestData, '\n')
	if err := os.WriteFile(filepath.Join(temporary, "manifest.json"), manifestData, 0o644); err != nil {
		return err
	}
	return replaceDirectory(temporary, config.destination)
}

func fetchLicense(config generationConfig, packageName, version string) (string, error) {
	metadataURL := strings.TrimRight(config.registryBase, "/") + "/" + url.PathEscape(packageName) + "/" + version
	data, _, err := get(config.client, metadataURL)
	if err != nil {
		return "", err
	}
	var metadata npmMetadata
	if err := json.Unmarshal(data, &metadata); err != nil {
		return "", err
	}
	if metadata.Dist.Tarball == "" {
		return "", fmt.Errorf("%s@%s metadata has no tarball", packageName, version)
	}
	archive, _, err := get(config.client, metadata.Dist.Tarball)
	if err != nil {
		return "", err
	}
	gz, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		return "", err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", err
		}
		name := strings.ToLower(path.Base(header.Name))
		if name == "license" || name == "license.md" || name == "license.txt" {
			license, err := io.ReadAll(tr)
			return string(license), err
		}
	}
	return "", fmt.Errorf("%s@%s has no license file", packageName, version)
}

func localModuleFilename(modulePath string) (string, error) {
	reference, err := parsePackageReference(modulePath)
	if err != nil {
		return "", err
	}
	const target = "/es2022/"
	if !strings.HasPrefix(reference.modulePath, target) || len(reference.modulePath) == len(target) {
		return "", fmt.Errorf("invalid module path %q", modulePath)
	}
	moduleName := strings.TrimPrefix(reference.modulePath, target)
	return strings.ReplaceAll(reference.packageName, "/", "__") + "__" + strings.ReplaceAll(moduleName, "/", "__"), nil
}

func replaceDirectory(temporary, destination string) error {
	if _, err := os.Stat(destination); errors.Is(err, os.ErrNotExist) {
		return os.Rename(temporary, destination)
	} else if err != nil {
		return err
	}
	parent := filepath.Dir(destination)
	backup, err := os.MkdirTemp(parent, ".codemirror-backup-")
	if err != nil {
		return err
	}
	if err := os.Remove(backup); err != nil {
		return err
	}
	defer os.RemoveAll(backup)
	if err := os.Rename(destination, backup); err != nil {
		return err
	}
	if err := os.Rename(temporary, destination); err != nil {
		if restoreErr := os.Rename(backup, destination); restoreErr != nil {
			return fmt.Errorf("replace vendor directory: %w (restore failed: %v)", err, restoreErr)
		}
		return err
	}
	_ = os.RemoveAll(backup)
	return nil
}

func hashMismatch(kind, sourceURL, expected, actual string) error {
	return fmt.Errorf("%s SHA-256 mismatch for %s: expected %s, actual %s; use -refresh to accept intentional changes", kind, sourceURL, expected, actual)
}

func get(client *http.Client, target string) ([]byte, http.Header, error) {
	response, err := client.Get(target)
	if err != nil {
		return nil, nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, nil, fmt.Errorf("GET %s: %s", target, response.Status)
	}
	data, err := io.ReadAll(response.Body)
	return data, response.Header, err
}

func digest(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func check(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "vendorcodemirror:", err)
		os.Exit(1)
	}
}
