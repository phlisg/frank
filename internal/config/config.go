package config

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"gopkg.in/yaml.v3"
)

const (
	DefaultPHPVersion     = "8.5"
	DefaultPHPRuntime     = "frankenphp"
	DefaultLaravelVersion = "latest"
	ConfigFileName        = "frank.yaml"
)

var validPHPVersions = map[string]bool{
	"8.2": true,
	"8.3": true,
	"8.4": true,
	"8.5": true,
}

var validLaravelVersions = map[string]bool{
	"12.*":   true,
	"13.*":   true,
	"latest": true,
}

var validRuntimes = map[string]bool{
	"frankenphp": true,
	"fpm":        true,
}

var databaseServices = map[string]bool{
	"pgsql":   true,
	"mysql":   true,
	"mariadb": true,
	"sqlite":  true,
}

var validServices = map[string]bool{
	"pgsql":       true,
	"mysql":       true,
	"mariadb":     true,
	"sqlite":      true,
	"redis":       true,
	"memcached":   true,
	"meilisearch": true,
	"mailpit":     true,
}

var defaultServices = []string{"pgsql", "mailpit"}

type Config struct {
	Version  int                      `yaml:"version"`
	PHP      PHP                      `yaml:"php"`
	Laravel  Laravel                  `yaml:"laravel"`
	Services []string                 `yaml:"services"`
	Config   map[string]ServiceConfig `yaml:"config"`
}

type PHP struct {
	Version string `yaml:"version"`
	Runtime string `yaml:"runtime"`
}

type Laravel struct {
	Version string `yaml:"version"`
}

type ServiceConfig struct {
	Port    int    `yaml:"port"`
	Version string `yaml:"version"`
}

// ProjectName derives the project name from the target directory basename.
func ProjectName(dir string) string {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return filepath.Base(dir)
	}
	return filepath.Base(abs)
}

// Load reads frank.yaml from dir, applies defaults, and validates.
func Load(dir string) (*Config, error) {
	path := filepath.Join(dir, ConfigFileName)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("could not read %s: %w", ConfigFileName, err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("could not parse %s: %w", ConfigFileName, err)
	}

	applyDefaults(&cfg)

	if err := validate(&cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}

// New returns a Config with all defaults applied (no frank.yaml required).
func New() *Config {
	cfg := &Config{}
	applyDefaults(cfg)
	return cfg
}

func applyDefaults(cfg *Config) {
	if cfg.PHP.Version == "" {
		cfg.PHP.Version = DefaultPHPVersion
	}
	if cfg.PHP.Runtime == "" {
		cfg.PHP.Runtime = DefaultPHPRuntime
	}
	if cfg.Laravel.Version == "" {
		cfg.Laravel.Version = DefaultLaravelVersion
	}
	if len(cfg.Services) == 0 {
		cfg.Services = append([]string{}, defaultServices...)
	}
}

func validate(cfg *Config) error {
	if !validPHPVersions[cfg.PHP.Version] {
		return fmt.Errorf("unsupported PHP version %q — valid options: 8.2, 8.3, 8.4, 8.5", cfg.PHP.Version)
	}
	if !validLaravelVersions[cfg.Laravel.Version] {
		return fmt.Errorf("unsupported Laravel version %q — valid options: 12.*, 13.*, latest", cfg.Laravel.Version)
	}
	if !validRuntimes[cfg.PHP.Runtime] {
		return fmt.Errorf("unsupported runtime %q — valid options: frankenphp, fpm", cfg.PHP.Runtime)
	}

	var dbCount int
	for _, svc := range cfg.Services {
		if !validServices[svc] {
			return fmt.Errorf("unsupported service %q — valid options: pgsql, mysql, mariadb, sqlite, redis, memcached, meilisearch, mailpit", svc)
		}
		if databaseServices[svc] {
			dbCount++
		}
	}
	if dbCount > 1 {
		return fmt.Errorf("only one database service is allowed (pgsql, mysql, mariadb, sqlite) — found %d", dbCount)
	}

	return nil
}

// HasService reports whether the named service is in cfg.Services.
func (cfg *Config) HasService(name string) bool {
	for _, svc := range cfg.Services {
		if svc == name {
			return true
		}
	}
	return false
}

// IsDatabase reports whether name is a database service.
func IsDatabase(name string) bool {
	return databaseServices[name]
}

// ValidService reports whether name is a supported service.
func ValidService(name string) bool {
	return validServices[name]
}

// AllServices returns a sorted list of all supported service names.
func AllServices() []string {
	names := make([]string, 0, len(validServices))
	for name := range validServices {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// Database returns the configured database service name, or "" if none.
func (cfg *Config) Database() string {
	for _, svc := range cfg.Services {
		if databaseServices[svc] {
			return svc
		}
	}
	return ""
}
