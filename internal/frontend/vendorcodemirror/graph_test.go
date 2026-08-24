package main

import (
	"context"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

var moduleImport = regexp.MustCompile(`(?:from|import)\s*["']((?:\.{0,2}/|https?://|node:)[^"']+)["']|import\(\s*["']((?:\.{0,2}/|https?://|node:)[^"']+)["']`)

func TestCommittedCodeMirrorGraphSharesVimDependencies(t *testing.T) {
	directory := filepath.Join("..", "static", "vendor", "codemirror")
	locked := readManifest(t, directory)
	byPackage := make(map[string]module)
	byPath := make(map[string]module)
	for _, item := range locked.Modules {
		if previous, ok := byPackage[item.Package]; ok {
			t.Fatalf("%s has multiple identities: %s and %s", item.Package, previous.SourceURL, item.SourceURL)
		}
		if _, ok := byPath[item.Path]; ok {
			t.Fatalf("duplicate local module %s", item.Path)
		}
		byPackage[item.Package] = item
		byPath[item.Path] = item
	}
	for pkg, version := range map[string]string{
		"@replit/codemirror-vim":      "6.4.0",
		"@replit/codemirror-vim-core": "0.1.0",
	} {
		if got := byPackage[pkg].Version; got != version {
			t.Fatalf("%s: want %s, got %q", pkg, version, got)
		}
	}

	dependencies := make(map[string]map[string]bool)
	for _, item := range locked.Modules {
		data, err := os.ReadFile(filepath.Join(directory, filepath.FromSlash(item.Path)))
		if err != nil {
			t.Fatal(err)
		}
		if got := digest(data); got != item.SHA256 {
			t.Errorf("%s: served hash differs from the content lock", item.Path)
		}
		dependencies[item.Package] = make(map[string]bool)
		for _, match := range moduleImport.FindAllStringSubmatch(string(data), -1) {
			spec := match[1]
			if spec == "" {
				spec = match[2]
			}
			if !strings.HasPrefix(spec, "./") {
				t.Errorf("%s: non-local runtime import %q", item.Path, spec)
				continue
			}
			dependency, ok := byPath[path.Join(path.Dir(item.Path), spec)]
			if !ok {
				t.Errorf("%s: import %q is not in the content lock", item.Path, spec)
				continue
			}
			dependencies[item.Package][dependency.Package] = true
		}
	}
	for _, pkg := range []string{
		"@codemirror/state", "@codemirror/view", "@codemirror/language",
		"@codemirror/commands", "@codemirror/search", "@replit/codemirror-vim-core",
	} {
		if !dependencies["@replit/codemirror-vim"][pkg] {
			t.Errorf("Vim must import the shared local %s module", pkg)
		}
	}
	for _, pkg := range []string{"@codemirror/view", "@codemirror/language", "@codemirror/commands", "@codemirror/search"} {
		if !dependencies[pkg]["@codemirror/state"] {
			t.Errorf("%s must share the editor's local state module", pkg)
		}
	}
}

func TestCommittedVimExtensionComposesWithMarkdown(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("Node is required to exercise the vendored JavaScript extensions")
	}
	directory, err := filepath.Abs(filepath.Join("..", "static", "vendor", "codemirror", "modules"))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, node, "--input-type=module", "--eval", `
import assert from 'node:assert/strict';
import {Compartment, EditorState} from './@codemirror__state__state.mjs';
import {markdown} from './@codemirror__lang-markdown__lang-markdown.mjs';
import {syntaxTree} from './@codemirror__language__language.mjs';
import {vim} from './@replit__codemirror-vim__codemirror-vim.mjs';

const mode = new Compartment();
let state = EditorState.create({
  doc: '# Notes', selection: {anchor: 3}, extensions: [markdown(), mode.of([])],
});
for (const extensions of [vim(), [], vim(), []]) {
  state = state.update({effects: mode.reconfigure(extensions)}).state;
  assert.equal(state.doc.toString(), '# Notes');
  assert.equal(state.selection.main.head, 3);
  assert.ok(syntaxTree(state).topNode.getChild('ATXHeading1'));
}
state = state.update({effects: mode.reconfigure(vim())}).state;
state = state.update({changes: {from: state.doc.length, insert: '\nSaved'}}).state;
assert.equal(state.doc.toString(), '# Notes\nSaved');
`)
	command.Dir = directory
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("Vim/Markdown shared-identity check failed: %v\n%s", err, output)
	}
}
