package tool

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPatchComposer_AddNew(t *testing.T) {
	dir := t.TempDir()
	writeJSON(t, dir, map[string]any{})

	tools := []Tool{
		{
			ComposerDev:     []string{"vendor/pkg:^1.0"},
			ComposerScripts: map[string]string{"lint": "do-lint"},
		},
	}

	patched, err := PatchComposer(dir, tools)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !patched {
		t.Fatal("expected patched=true")
	}

	doc := readJSON(t, dir)
	reqDev := doc["require-dev"].(map[string]any)
	if reqDev["vendor/pkg"] != "^1.0" {
		t.Errorf("expected vendor/pkg=^1.0, got %v", reqDev["vendor/pkg"])
	}

	scripts := doc["scripts"].(map[string]any)
	if scripts["lint"] != "do-lint" {
		t.Errorf("expected lint=do-lint, got %v", scripts["lint"])
	}
}

func TestPatchComposer_SkipExisting(t *testing.T) {
	dir := t.TempDir()
	writeJSON(t, dir, map[string]any{
		"require-dev": map[string]any{"vendor/pkg": "^0.9"},
		"scripts":     map[string]any{"lint": "old-lint"},
	})

	tools := []Tool{
		{
			ComposerDev:     []string{"vendor/pkg:^1.0"},
			ComposerScripts: map[string]string{"lint": "new-lint"},
		},
	}

	patched, err := PatchComposer(dir, tools)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if patched {
		t.Fatal("expected patched=false when all entries exist")
	}

	doc := readJSON(t, dir)
	reqDev := doc["require-dev"].(map[string]any)
	if reqDev["vendor/pkg"] != "^0.9" {
		t.Errorf("existing version overwritten: got %v", reqDev["vendor/pkg"])
	}
	scripts := doc["scripts"].(map[string]any)
	if scripts["lint"] != "old-lint" {
		t.Errorf("existing script overwritten: got %v", scripts["lint"])
	}
}

func TestPatchComposer_PreservesUnknownKeys(t *testing.T) {
	dir := t.TempDir()
	writeJSON(t, dir, map[string]any{
		"name":        "my/project",
		"description": "A project",
		"custom-key":  "custom-value",
	})

	tools := []Tool{
		{
			ComposerDev:     []string{"vendor/pkg:^1.0"},
			ComposerScripts: map[string]string{"test": "phpunit"},
		},
	}

	_, err := PatchComposer(dir, tools)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	doc := readJSON(t, dir)
	if doc["name"] != "my/project" {
		t.Errorf("name key lost: %v", doc["name"])
	}
	if doc["custom-key"] != "custom-value" {
		t.Errorf("custom-key lost: %v", doc["custom-key"])
	}
}

func TestPatchComposer_MissingFile(t *testing.T) {
	dir := t.TempDir()

	tools := []Tool{
		{
			ComposerDev:     []string{"vendor/pkg:^1.0"},
			ComposerScripts: map[string]string{"lint": "do-lint"},
		},
	}

	patched, err := PatchComposer(dir, tools)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if patched {
		t.Fatal("expected patched=false for missing file")
	}
}

func TestPatchComposer_Indent(t *testing.T) {
	dir := t.TempDir()
	writeJSON(t, dir, map[string]any{})

	tools := []Tool{
		{
			ComposerDev:     []string{"vendor/pkg:^1.0"},
			ComposerScripts: map[string]string{"lint": "do-lint"},
		},
	}

	_, err := PatchComposer(dir, tools)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "composer.json"))
	if err != nil {
		t.Fatalf("reading output: %v", err)
	}

	content := string(data)
	if !strings.Contains(content, "    ") {
		t.Error("expected 4-space indent in output")
	}
	// Should not contain tab indentation
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		if strings.HasPrefix(line, "\t") {
			t.Errorf("line %d uses tab indent: %q", i+1, line)
		}
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
