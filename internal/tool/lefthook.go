package tool

import "strings"

// AssembleLefthook builds a complete lefthook.yml as a string.
// The post-merge section is always included. Pre-commit commands are
// conditionally included based on which tool names are in the tools slice.
func AssembleLefthook(tools []string) string {
	var b strings.Builder

	b.WriteString("assert_lefthook_installed: true\n")
	b.WriteString("\n")

	// post-merge: always included
	b.WriteString("post-merge:\n")
	b.WriteString("  jobs:\n")
	b.WriteString("    - name: composer-install\n")
	b.WriteString("      run: >\n")
	b.WriteString("        git diff ORIG_HEAD HEAD --name-only | grep -q \"composer.lock\"\n")
	b.WriteString("        && frank composer install || true\n")
	b.WriteString("    - name: migrate\n")
	b.WriteString("      run: >\n")
	b.WriteString("        git diff ORIG_HEAD HEAD --name-only | grep -q \"database/migrations/\"\n")
	b.WriteString("        && frank composer run migrate || true\n")
	b.WriteString("    - name: node-install\n")
	b.WriteString("      run: |\n")
	b.WriteString("        if git diff ORIG_HEAD HEAD --name-only | grep -qE \"(package-lock|pnpm-lock|bun\\.lock)\"; then\n")
	b.WriteString("          frank exec sh -c '\n")
	b.WriteString("            if command -v pnpm >/dev/null 2>&1; then pnpm install\n")
	b.WriteString("            elif command -v bun >/dev/null 2>&1; then bun install\n")
	b.WriteString("            else npm install\n")
	b.WriteString("            fi\n")
	b.WriteString("          '\n")
	b.WriteString("        fi\n")

	// pre-commit: only if PHP tools selected
	selected := phpToolsSelected(tools)
	if len(selected) > 0 {
		b.WriteString("\n")
		b.WriteString("pre-commit:\n")
		b.WriteString("  parallel: true\n")
		b.WriteString("  commands:\n")
		for _, name := range selected {
			t := lookupTool(name)
			if t == nil {
				continue
			}
			b.WriteString(lefthookEntry(*t))
		}
	}

	return b.String()
}

// lefthookEntry returns a YAML snippet for a single pre-commit entry.
func lefthookEntry(t Tool) string {
	switch t.Name {
	case "pint":
		return `    pint:
      glob: "*.php"
      run: frank exec php vendor/bin/pint {staged_files}
      stage_fixed: true
`
	case "rector":
		return `    rector:
      glob: "*.php"
      run: frank exec php vendor/bin/rector process {staged_files}
      stage_fixed: true
`
	case "larastan":
		return `    larastan:
      glob: "*.php"
      run: frank exec php vendor/bin/phpstan analyse -c phpstan.neon {staged_files}
`
	default:
		return ""
	}
}

// phpToolsSelected returns the subset of tools that have pre-commit entries,
// in a stable order (pint, rector, larastan).
func phpToolsSelected(tools []string) []string {
	order := []string{"pint", "rector", "larastan"}
	set := make(map[string]bool, len(tools))
	for _, t := range tools {
		set[t] = true
	}
	var result []string
	for _, name := range order {
		if set[name] {
			result = append(result, name)
		}
	}
	return result
}

// lookupTool finds a tool by name in the registry.
func lookupTool(name string) *Tool {
	for i := range registry {
		if registry[i].Name == name {
			return &registry[i]
		}
	}
	return nil
}
