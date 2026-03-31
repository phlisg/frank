package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/phlisg/frank-cli/internal/config"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var importFile string

func init() {
	importCmd.Flags().StringVarP(&importFile, "file", "f", "", "path to Sail docker-compose.yml (defaults to ./docker-compose.yml)")
	rootCmd.AddCommand(importCmd)
}

var importCmd = &cobra.Command{
	Use:   "import",
	Short: "Import settings from a Sail docker-compose.yml and write frank.yaml",
	Long: `Reads an existing Laravel Sail docker-compose.yml, detects PHP version and
services, then writes frank.yaml and regenerates Docker files.

Custom Dockerfile modifications in your Sail setup will not be preserved.`,
	SilenceUsage: true,
	RunE:         runImport,
}

// sailComposeFile is a minimal struct for parsing Sail's docker-compose.yml.
type sailComposeFile struct {
	Services map[string]sailService `yaml:"services"`
}

type sailService struct {
	Image string `yaml:"image"`
	Build struct {
		Context string `yaml:"context"`
	} `yaml:"build"`
}

// phpFromSail matches image names like "sail-8.3/app" or context paths like ".../runtimes/8.3".
var phpFromSail = regexp.MustCompile(`(?:sail-|runtimes/)(\d+\.\d+)`)

// sailServiceMap maps known Sail service names to Frank service names.
var sailServiceMap = map[string]string{
	"pgsql":       "pgsql",
	"mysql":       "mysql",
	"mariadb":     "mariadb",
	"redis":       "redis",
	"memcached":   "memcached",
	"meilisearch": "meilisearch",
	"mailpit":     "mailpit",
	"mailhog":     "mailpit", // legacy Sail name → mailpit
}

func runImport(cmd *cobra.Command, args []string) error {
	dir := resolveDir()

	composePath := importFile
	if composePath == "" {
		composePath = filepath.Join(dir, "docker-compose.yml")
	}

	data, err := os.ReadFile(composePath)
	if err != nil {
		return fmt.Errorf("could not read %s: %w", composePath, err)
	}

	var compose sailComposeFile
	if err := yaml.Unmarshal(data, &compose); err != nil {
		return fmt.Errorf("could not parse %s: %w", composePath, err)
	}

	phpVersion, services := parseSailCompose(compose)

	if phpVersion == "" {
		fmt.Println("Warning: could not detect PHP version — defaulting to", config.DefaultPHPVersion)
		phpVersion = config.DefaultPHPVersion
	}

	if len(services) == 0 {
		fmt.Println("Warning: no known services detected — using defaults")
		services = []string{"pgsql", "mailpit"}
	}

	cfg := config.New()
	cfg.PHP.Version = phpVersion
	cfg.Services = services

	yamlBytes, err := marshalConfig(cfg)
	if err != nil {
		return err
	}
	if err := writeFile(filepath.Join(dir, config.ConfigFileName), yamlBytes); err != nil {
		return err
	}
	fmt.Printf("  imported  PHP %s, services: %s\n", phpVersion, strings.Join(services, ", "))
	fmt.Println("  wrote     frank.yaml")

	fmt.Println("\nGenerating Docker files...")
	return generate(cfg, dir)
}

func parseSailCompose(compose sailComposeFile) (phpVersion string, services []string) {
	for name, svc := range compose.Services {
		// Detect PHP version from the app service image or build context.
		if m := phpFromSail.FindStringSubmatch(svc.Image); len(m) == 2 {
			phpVersion = m[1]
		} else if m := phpFromSail.FindStringSubmatch(svc.Build.Context); len(m) == 2 {
			phpVersion = m[1]
		}

		// Map service names.
		if frank, ok := sailServiceMap[name]; ok {
			services = append(services, frank)
		}
	}

	// Deduplicate (mailhog + mailpit could both appear).
	services = dedup(services)
	return
}

func dedup(ss []string) []string {
	seen := map[string]bool{}
	out := ss[:0]
	for _, s := range ss {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}
