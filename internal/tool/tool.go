package tool

import (
	"fmt"
	"sort"
)

type Tool struct {
	Name            string
	Category        string // "php" or "project"
	ConfigFiles     map[string]string
	ComposerDev     []string
	ComposerScripts map[string]string
	PostInstall     []string
}

var registry = []Tool{
	{
		Name:        "pint",
		Category:    "php",
		ConfigFiles: map[string]string{"pint.json": "pint.json"},
		ComposerDev: []string{"laravel/pint:^1.0"},
		ComposerScripts: map[string]string{
			"lint": "pint --config pint.json",
		},
	},
	{
		Name:        "larastan",
		Category:    "php",
		ConfigFiles: map[string]string{"phpstan.neon": "phpstan.neon"},
		ComposerDev: []string{"larastan/larastan:^3.0"},
		ComposerScripts: map[string]string{
			"analyse": "phpstan analyse -c phpstan.neon",
		},
	},
	{
		Name:        "rector",
		Category:    "php",
		ConfigFiles: map[string]string{"rector.php": "rector.php"},
		ComposerDev: []string{"rector/rector:^2.0", "dereuromark/rector-laravel:^2.0"},
		ComposerScripts: map[string]string{
			"refactor": "rector process --config rector.php",
		},
	},
	{
		Name:        "lefthook",
		Category:    "project",
		ConfigFiles: map[string]string{},
		PostInstall: []string{"lefthook install"},
	},
}

func Lookup(name string) (Tool, bool) {
	for _, t := range registry {
		if t.Name == name {
			return t, true
		}
	}
	return Tool{}, false
}

func Valid(name string) bool {
	_, ok := Lookup(name)
	return ok
}

func AllNames() []string {
	names := make([]string, len(registry))
	for i, t := range registry {
		names[i] = t.Name
	}
	sort.Strings(names)
	return names
}

func AllTools() []Tool {
	out := make([]Tool, len(registry))
	copy(out, registry)
	return out
}

func PHPTools(selected []string) []Tool {
	set := make(map[string]bool, len(selected))
	for _, s := range selected {
		set[s] = true
	}
	var out []Tool
	for _, t := range registry {
		if t.Category == "php" && set[t.Name] {
			out = append(out, t)
		}
	}
	return out
}

func ValidateNames(names []string) error {
	for _, n := range names {
		if !Valid(n) {
			return fmt.Errorf("unknown tool %q — valid options: %v", n, AllNames())
		}
	}
	return nil
}
