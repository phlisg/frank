package tool

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestComposerDevPackages_New(t *testing.T) {
	dir := t.TempDir()
	writeJSON(t, dir, map[string]any{})

	tools := []Tool{
		{ComposerDev: []string{"vendor/pkg:^1.0", "vendor/other:^2.0"}},
	}

	pkgs := ComposerDevPackages(dir, tools)
	if len(pkgs) != 2 {
		t.Fatalf("expected 2 packages, got %d", len(pkgs))
	}
}

func TestComposerDevPackages_SkipExisting(t *testing.T) {
	dir := t.TempDir()
	writeJSON(t, dir, map[string]any{
		"require-dev": map[string]any{"vendor/pkg": "^0.9"},
	})

	tools := []Tool{
		{ComposerDev: []string{"vendor/pkg:^1.0", "vendor/other:^2.0"}},
	}

	pkgs := ComposerDevPackages(dir, tools)
	if len(pkgs) != 1 || pkgs[0] != "vendor/other:^2.0" {
		t.Fatalf("expected [vendor/other:^2.0], got %v", pkgs)
	}
}

func TestComposerDevPackages_NoFile(t *testing.T) {
	dir := t.TempDir()

	tools := []Tool{
		{ComposerDev: []string{"vendor/pkg:^1.0"}},
	}

	pkgs := ComposerDevPackages(dir, tools)
	if len(pkgs) != 1 {
		t.Fatalf("expected 1 package when no composer.json, got %d", len(pkgs))
	}
}

func TestPatchComposerScripts_AddNew(t *testing.T) {
	dir := t.TempDir()
	writeJSON(t, dir, map[string]any{})

	tools := []Tool{
		{ComposerScripts: map[string]string{"lint": "do-lint"}},
	}

	if err := PatchComposerScripts(dir, tools); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	doc := readJSON(t, dir)
	scripts := doc["scripts"].(map[string]any)
	if scripts["lint"] != "do-lint" {
		t.Errorf("expected lint=do-lint, got %v", scripts["lint"])
	}
}

func TestPatchComposerScripts_SkipExisting(t *testing.T) {
	dir := t.TempDir()
	writeJSON(t, dir, map[string]any{
		"scripts": map[string]any{"lint": "old-lint"},
	})

	tools := []Tool{
		{ComposerScripts: map[string]string{"lint": "new-lint"}},
	}

	if err := PatchComposerScripts(dir, tools); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	doc := readJSON(t, dir)
	scripts := doc["scripts"].(map[string]any)
	if scripts["lint"] != "old-lint" {
		t.Errorf("existing script overwritten: got %v", scripts["lint"])
	}
}

func TestPatchComposerScripts_Indent(t *testing.T) {
	dir := t.TempDir()
	writeJSON(t, dir, map[string]any{})

	tools := []Tool{
		{ComposerScripts: map[string]string{"lint": "do-lint"}},
	}

	if err := PatchComposerScripts(dir, tools); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, _ := os.ReadFile(filepath.Join(dir, "composer.json"))
	content := string(data)
	if !strings.Contains(content, "    ") {
		t.Error("expected 4-space indent in output")
	}
}

// helpers

func writeJSON(t *testing.T, dir string, doc map[string]any) {
	t.Helper()
	data, err := json.MarshalIndent(doc, "", "    ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "composer.json"), data, 0644); err != nil {
		t.Fatal(err)
	}
}

func readJSON(t *testing.T, dir string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, "composer.json"))
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	return doc
}
