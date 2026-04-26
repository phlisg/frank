package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/phlisg/frank/internal/config"
	"github.com/phlisg/frank/internal/output"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

// settableKeys maps dotted key paths to their valid values.
var settableKeys = map[string][]string{
	"php.version":        {"8.2", "8.3", "8.4", "8.5"},
	"php.runtime":        {"frankenphp", "fpm"},
	"laravel.version":    {"12.*", "13.*", "latest"},
	"node.packageManager": {"npm", "pnpm", "bun"},
}

func init() {
	configCmd.AddCommand(configSetCmd)
}

var configSetCmd = &cobra.Command{
	Use:   "set <key> <value>",
	Short: "Set a configuration value",
	Long: `Set a scalar value in frank.yaml by dotted key path.

Supported keys:
  php.version         8.2, 8.3, 8.4, 8.5
  php.runtime         frankenphp, fpm
  laravel.version     12.*, 13.*, latest
  node.packageManager npm, pnpm, bun`,
	Args:              cobra.ExactArgs(2),
	SilenceUsage:      true,
	ValidArgsFunction: configSetCompletion,
	RunE:              runConfigSet,
}

func runConfigSet(cmd *cobra.Command, args []string) error {
	key, value := args[0], args[1]

	// Validate key.
	allowed, ok := settableKeys[key]
	if !ok {
		keys := make([]string, 0, len(settableKeys))
		for k := range settableKeys {
			keys = append(keys, k)
		}
		return fmt.Errorf("unknown key %q — valid keys: %s", key, strings.Join(keys, ", "))
	}

	// Validate value.
	valid := false
	for _, v := range allowed {
		if v == value {
			valid = true
			break
		}
	}
	if !valid {
		return fmt.Errorf("invalid value %q for %s — valid options: %s", value, key, strings.Join(allowed, ", "))
	}

	dir := resolveDir()
	cfgPath := filepath.Join(dir, config.ConfigFileName)

	// Read raw YAML as node tree to preserve comments/formatting.
	raw, err := os.ReadFile(cfgPath)
	if err != nil {
		return fmt.Errorf("read %s: %w", config.ConfigFileName, err)
	}

	var doc yaml.Node
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return fmt.Errorf("parse %s: %w", config.ConfigFileName, err)
	}

	// Walk the dotted path, creating intermediate mapping nodes if needed.
	parts := strings.Split(key, ".")
	target := walkOrCreateNodePath(&doc, parts)
	if target == nil {
		return fmt.Errorf("cannot set %s: unexpected YAML structure", key)
	}

	// Set the scalar value.
	target.Value = value
	target.Tag = "!!str"
	target.Kind = yaml.ScalarNode

	// Marshal the modified tree back to YAML.
	out, err := marshalNode(&doc)
	if err != nil {
		return fmt.Errorf("marshal YAML: %w", err)
	}

	// Write file, then validate via config.Load. Restore original on failure.
	if err := os.WriteFile(cfgPath, out, 0644); err != nil {
		return fmt.Errorf("write %s: %w", config.ConfigFileName, err)
	}
	cfg, loadErr := config.Load(dir)
	if loadErr != nil {
		// Restore original frank.yaml.
		_ = os.WriteFile(cfgPath, raw, 0644)
		return fmt.Errorf("validation failed: %w", loadErr)
	}
	output.Group(fmt.Sprintf("Set %s = %s", key, value), "")

	// Regenerate .frank/ files.
	stopGen := output.Spin("Generating Docker files")
	if err := generate(cfg, dir); err != nil {
		stopGen(err)
		return err
	}
	stopGen(nil)

	// Prompt to rebuild.
	if err := setupRebuildPrompt(dir); err != nil {
		return err
	}

	return nil
}

// walkOrCreateNodePath traverses (or creates) the dotted key path in a YAML
// document node, returning the value node for the final key segment.
func walkOrCreateNodePath(root *yaml.Node, path []string) *yaml.Node {
	if root.Kind != yaml.DocumentNode || len(root.Content) == 0 {
		return nil
	}
	node := root.Content[0]

	for i, key := range path {
		if node.Kind != yaml.MappingNode {
			return nil
		}

		// Search existing keys.
		found := false
		for j := 0; j+1 < len(node.Content); j += 2 {
			if node.Content[j].Value == key {
				if i == len(path)-1 {
					// Final segment — return the value node.
					return node.Content[j+1]
				}
				node = node.Content[j+1]
				found = true
				break
			}
		}
		if found {
			continue
		}

		// Key not found — create intermediate mapping or final scalar.
		keyNode := &yaml.Node{Kind: yaml.ScalarNode, Value: key, Tag: "!!str"}
		if i == len(path)-1 {
			valNode := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str"}
			node.Content = append(node.Content, keyNode, valNode)
			return valNode
		}
		mapNode := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		node.Content = append(node.Content, keyNode, mapNode)
		node = mapNode
	}
	return node
}

// marshalNode serializes a yaml.Node document back to bytes.
func marshalNode(doc *yaml.Node) ([]byte, error) {
	var buf strings.Builder
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(doc); err != nil {
		return nil, err
	}
	if err := enc.Close(); err != nil {
		return nil, err
	}
	return []byte(buf.String()), nil
}

// configSetCompletion provides shell completions for config set.
func configSetCompletion(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	switch len(args) {
	case 0:
		// Complete key names.
		keys := make([]string, 0, len(settableKeys))
		for k := range settableKeys {
			keys = append(keys, k)
		}
		return keys, cobra.ShellCompDirectiveNoFileComp
	case 1:
		// Complete valid values for the given key.
		if vals, ok := settableKeys[args[0]]; ok {
			return vals, cobra.ShellCompDirectiveNoFileComp
		}
		return nil, cobra.ShellCompDirectiveNoFileComp
	default:
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
}
