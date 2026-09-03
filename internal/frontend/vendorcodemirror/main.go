// Command vendorcodemirror refreshes the browser-ready CodeMirror module graph.
package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
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
	esmBase    = "https://esm.sh"
	vendorRoot = "internal/frontend/static/vendor/codemirror"
)

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

type manifest struct {
	Schema  int      `json:"schema"`
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

func main() {
	refresh := flag.Bool("refresh", false, "resolve transitive ranges again instead of using the manifest lock")
	flag.Parse()

	client := &http.Client{Timeout: 30 * time.Second}
	locked, err := lockedVersions(vendorRoot + "/manifest.json")
	check(err)
	sources, err := fetchGraph(client, locked, *refresh)
	check(err)
	check(writeGraph(client, sources))
}

func lockedVersions(name string) (map[string]string, error) {
	locked := make(map[string]string)
	data, err := os.ReadFile(name)
	if os.IsNotExist(err) {
		return locked, nil
	}
	if err != nil {
		return nil, err
	}
	var current manifest
	if err := json.Unmarshal(data, &current); err != nil {
		return nil, err
	}
	for _, item := range current.Modules {
		locked[item.Package] = item.Version
	}
	return locked, nil
}

func fetchGraph(client *http.Client, locked map[string]string, refresh bool) (map[string]sourceModule, error) {
	queue := append([]string(nil), roots...)
	sources := make(map[string]sourceModule)
	for len(queue) > 0 {
		modulePath := queue[0]
		queue = queue[1:]
		if _, ok := sources[modulePath]; ok {
			continue
		}
		raw, _, err := get(client, esmBase+modulePath)
		if err != nil {
			return nil, err
		}
		body := normalize(modulePath, raw)
		imports := make(map[string]string)
		for _, match := range rootImport.FindAllSubmatch(body, -1) {
			spec := string(match[1])
			if spec == "" {
				spec = string(match[2])
			}
			exact, err := resolve(client, spec, locked, refresh)
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

func resolve(client *http.Client, spec string, locked map[string]string, refresh bool) (string, error) {
	if strings.Contains(spec, "/es2022/") {
		return strings.Split(spec, "?")[0], nil
	}
	requestPath := spec
	pkg, _, suffix, err := packageSpec(spec)
	if err != nil {
		return "", err
	}
	if version := locked[pkg]; version != "" && !refresh {
		requestPath = "/" + pkg + "@" + version + suffix
	}
	_, header, err := get(client, esmBase+requestPath)
	if err != nil {
		return "", err
	}
	exact := header.Get("x-esm-path")
	if exact == "" {
		return "", fmt.Errorf("response has no x-esm-path")
	}
	return exact, nil
}

func packageSpec(spec string) (pkg, version, suffix string, err error) {
	value := strings.TrimPrefix(strings.Split(spec, "?")[0], "/")
	if strings.HasPrefix(value, "@") {
		slash := strings.Index(value, "/")
		at := strings.Index(value[slash+1:], "@")
		if slash < 0 || at < 0 {
			return "", "", "", fmt.Errorf("invalid package spec %q", spec)
		}
		at += slash + 1
		next := strings.Index(value[at+1:], "/")
		if next < 0 {
			return value[:at], value[at+1:], "?target=es2022", nil
		}
		next += at + 1
		return value[:at], value[at+1 : next], value[next:] + "?target=es2022", nil
	}
	at := strings.Index(value, "@")
	if at < 0 {
		return "", "", "", fmt.Errorf("invalid package spec %q", spec)
	}
	next := strings.Index(value[at+1:], "/")
	if next < 0 {
		return value[:at], value[at+1:], "?target=es2022", nil
	}
	next += at + 1
	return value[:at], value[at+1 : next], value[next:] + "?target=es2022", nil
}

func normalize(modulePath string, body []byte) []byte {
	text := sourceMapComment.ReplaceAllString(string(body), "\n")
	if strings.Contains(modulePath, "/@lezer/lr@") {
		text = strings.Replace(text, `import __Process$ from "/node/process.mjs";`, "", 1)
		text = strings.Replace(text, `var g=typeof __Process$<"u"&&__Process$.env&&/\bparse\b/.test(__Process$.env.LOG),A=null;`, `var g=false,A=null;`, 1)
	}
	return []byte(text)
}

func writeGraph(client *http.Client, sources map[string]sourceModule) error {
	parent := filepath.Dir(vendorRoot)
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
	result := manifest{Schema: 1}
	licenses := make(map[string]string)
	for _, modulePath := range paths {
		source := sources[modulePath]
		pkg, version, err := packageVersion(modulePath)
		if err != nil {
			return err
		}
		licenseKey := pkg + "@" + version
		license := licenses[licenseKey]
		if license == "" {
			license, err = fetchLicense(client, pkg, version)
			if err != nil {
				return err
			}
			licenses[licenseKey] = license
		}
		text := string(source.body)
		for spec, exact := range source.imports {
			local := "./" + slug(exact)
			text = strings.ReplaceAll(text, `"`+spec+`"`, `"`+local+`"`)
			text = strings.ReplaceAll(text, `'`+spec+`'`, `'`+local+`'`)
		}
		text = "/*!\n" + strings.TrimSpace(license) + "\n*/\n" + text
		localPath := "modules/" + slug(modulePath)
		data := []byte(text)
		if err := os.WriteFile(filepath.Join(temporary, filepath.FromSlash(localPath)), data, 0o644); err != nil {
			return err
		}
		result.Modules = append(result.Modules, module{
			Path:         localPath,
			Package:      pkg,
			Version:      version,
			SourceURL:    esmBase + modulePath,
			SourceSHA256: digest(source.raw),
			SHA256:       digest(data),
		})
	}
	manifestData, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return err
	}
	manifestData = append(manifestData, '\n')
	if err := os.WriteFile(filepath.Join(temporary, "manifest.json"), manifestData, 0o644); err != nil {
		return err
	}
	if err := os.RemoveAll(vendorRoot); err != nil {
		return err
	}
	return os.Rename(temporary, vendorRoot)
}

func fetchLicense(client *http.Client, pkg, version string) (string, error) {
	metadataURL := "https://registry.npmjs.org/" + url.PathEscape(pkg) + "/" + version
	data, _, err := get(client, metadataURL)
	if err != nil {
		return "", err
	}
	var metadata npmMetadata
	if err := json.Unmarshal(data, &metadata); err != nil {
		return "", err
	}
	archive, _, err := get(client, metadata.Dist.Tarball)
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
	return "", fmt.Errorf("%s@%s has no license file", pkg, version)
}

func packageVersion(modulePath string) (string, string, error) {
	value := strings.TrimPrefix(strings.Split(modulePath, "/es2022/")[0], "/")
	if strings.HasPrefix(value, "@") {
		slash := strings.Index(value, "/")
		at := strings.LastIndex(value, "@")
		if slash < 0 || at <= slash {
			return "", "", fmt.Errorf("invalid module path %q", modulePath)
		}
		return value[:at], value[at+1:], nil
	}
	at := strings.LastIndex(value, "@")
	if at < 0 {
		return "", "", fmt.Errorf("invalid module path %q", modulePath)
	}
	return value[:at], value[at+1:], nil
}

func slug(modulePath string) string {
	pkg, _, err := packageVersion(modulePath)
	if err != nil {
		panic(err)
	}
	value := strings.TrimPrefix(strings.Split(modulePath, "?")[0], "/")
	moduleName := strings.SplitN(value, "/es2022/", 2)[1]
	return strings.ReplaceAll(pkg, "/", "__") + "__" + strings.ReplaceAll(moduleName, "/", "__")
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
