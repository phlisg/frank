package tool

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// PatchComposer merges ComposerDev and ComposerScripts from the given tools
// into the project's composer.json. Returns true if anything was written.
func PatchComposer(dir string, tools []Tool) (patched bool, err error) {
	path := filepath.Join(dir, "composer.json")

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Println("  warning    composer.json not found — skipping composer patching")
			return false, nil
		}
		return false, err
	}

	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		return false, fmt.Errorf("parsing composer.json: %w", err)
	}

	changed := false

	for _, t := range tools {
		// Merge require-dev entries
		for _, entry := range t.ComposerDev {
			parts := strings.SplitN(entry, ":", 2)
			if len(parts) != 2 {
				continue
			}
			pkg, version := parts[0], parts[1]

			reqDev, _ := doc["require-dev"].(map[string]any)
			if reqDev == nil {
				reqDev = make(map[string]any)
				doc["require-dev"] = reqDev
			}

			if _, exists := reqDev[pkg]; exists {
				continue
			}

			reqDev[pkg] = version
			changed = true
			fmt.Printf("  composer    added %s to require-dev\n", pkg)
		}

		// Merge scripts entries
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
			fmt.Printf("  composer    added %q script\n", key)
		}
	}

	if !changed {
		return false, nil
	}

	out, err := json.MarshalIndent(doc, "", "    ")
	if err != nil {
		return false, fmt.Errorf("marshaling composer.json: %w", err)
	}

	out = append(out, '\n')

	if err := os.WriteFile(path, out, 0644); err != nil {
		return false, fmt.Errorf("writing composer.json: %w", err)
	}

	return true, nil
}
