package tool

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ComposerDevPackages returns all require-dev packages (as "pkg:version" entries)
// for the given tools, filtering out any already present in composer.json.
func ComposerDevPackages(dir string, tools []Tool) []string {
	existing := existingDevDeps(dir)
	var packages []string
	for _, t := range tools {
		for _, entry := range t.ComposerDev {
			parts := strings.SplitN(entry, ":", 2)
			if len(parts) != 2 {
				continue
			}
			if _, exists := existing[parts[0]]; exists {
				continue
			}
			packages = append(packages, entry)
		}
	}
	return packages
}

func existingDevDeps(dir string) map[string]bool {
	path := filepath.Join(dir, "composer.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil
	}
	reqDev, _ := doc["require-dev"].(map[string]any)
	if reqDev == nil {
		return nil
	}
	out := make(map[string]bool, len(reqDev))
	for k := range reqDev {
		out[k] = true
	}
	return out
}

// PatchComposerScripts merges ComposerScripts from tools into composer.json.
// Only touches the "scripts" key — does not modify require-dev.
func PatchComposerScripts(dir string, tools []Tool) error {
	path := filepath.Join(dir, "composer.json")

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		return fmt.Errorf("parsing composer.json: %w", err)
	}

	changed := false
	for _, t := range tools {
		for key, cmd := range t.ComposerScripts {
			scripts, _ := doc["scripts"].(map[string]any)
			if scripts == nil {
				scripts = make(map[string]any)
				doc["scripts"] = scripts
			}
			if _, exists := scripts[key]; exists {
				continue
			}
			scripts[key] = cmd
			changed = true
		}
	}

	if !changed {
		return nil
	}

	out, err := json.MarshalIndent(doc, "", "    ")
	if err != nil {
		return fmt.Errorf("marshaling composer.json: %w", err)
	}
	out = append(out, '\n')

	return os.WriteFile(path, out, 0644)
}
