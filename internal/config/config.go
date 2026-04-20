package config

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"

	"gopkg.in/yaml.v3"
)

const (
	DefaultPHPVersion      = "8.5"
	DefaultPHPRuntime      = "frankenphp"
	DefaultLaravelVersion  = "latest"
	DefaultPackageManager  = "npm"
	ConfigFileName         = "frank.yaml"
)

var validPackageManagers = map[string]bool{
	"npm":  true,
	"pnpm": true,
	"bun":  true,
}

var knownNodeKeys = map[string]bool{
	"packageManager": true,
}

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

var workerPoolNameRe = regexp.MustCompile(`^[a-z0-9_-]+$`)

var knownWorkersKeys = map[string]bool{
	"schedule": true,
	"queue":    true,
}

var knownQueueItemKeys = map[string]bool{
	"name":    true,
	"queues":  true,
	"count":   true,
	"tries":   true,
	"timeout": true,
	"memory":  true,
	"sleep":   true,
	"backoff": true,
}

type Config struct {
	Version  int                      `yaml:"version"`
	PHP      PHP                      `yaml:"php"`
	Laravel  Laravel                  `yaml:"laravel"`
	Services []string                 `yaml:"services"`
	Config   map[string]ServiceConfig `yaml:"config"`
	Workers  Workers                  `yaml:"workers"`
	Node     Node                     `yaml:"node,omitempty"`
	Tools    []string                 `yaml:"tools,omitempty"`
}

type Node struct {
	PackageManager string `yaml:"packageManager,omitempty"`
}

type Workers struct {
	Schedule bool        `yaml:"schedule,omitempty"`
	Queue    []QueuePool `yaml:"queue,omitempty"`
}

type QueuePool struct {
	Name    string   `yaml:"name"`
	Queues  []string `yaml:"queues"`
	Count   int      `yaml:"count"`
	Tries   int      `yaml:"tries,omitempty"`
	Timeout int      `yaml:"timeout,omitempty"`
	Memory  int      `yaml:"memory,omitempty"`
	Sleep   int      `yaml:"sleep,omitempty"`
	Backoff int      `yaml:"backoff,omitempty"`
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

	var root yaml.Node
	if err := yaml.Unmarshal(data, &root); err != nil {
		return nil, fmt.Errorf("could not parse %s: %w", ConfigFileName, err)
	}

	var cfg Config
	if err := root.Decode(&cfg); err != nil {
		return nil, fmt.Errorf("could not parse %s: %w", ConfigFileName, err)
	}

	warnUnknownWorkerKeys(&root)
	warnUnknownNodeKeys(&root)

	// Capture explicit-empty-queues before defaulting overwrites.
	explicitEmptyQueues := make([]bool, len(cfg.Workers.Queue))
	for i, p := range cfg.Workers.Queue {
		explicitEmptyQueues[i] = p.Queues != nil && len(p.Queues) == 0
	}

	applyDefaults(&cfg)

	if err := validate(&cfg, explicitEmptyQueues); err != nil {
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
	if cfg.Node.PackageManager == "" {
		cfg.Node.PackageManager = DefaultPackageManager
	}
	// Default workers: schedule + 1 queue worker on the "default" queue.
	if !cfg.Workers.Schedule && len(cfg.Workers.Queue) == 0 {
		cfg.Workers.Schedule = true
		cfg.Workers.Queue = []QueuePool{{Count: 1}}
	}
	for i := range cfg.Workers.Queue {
		p := &cfg.Workers.Queue[i]
		if p.Queues == nil {
			p.Queues = []string{"default"}
		}
		if p.Name == "" && len(p.Queues) > 0 {
			p.Name = p.Queues[0]
		}
		if p.Count == 0 {
			p.Count = 1
		}
	}
}

func validate(cfg *Config, explicitEmptyQueues []bool) error {
	if !validPHPVersions[cfg.PHP.Version] {
		return fmt.Errorf("unsupported PHP version %q — valid options: 8.2, 8.3, 8.4, 8.5", cfg.PHP.Version)
	}
	if !validLaravelVersions[cfg.Laravel.Version] {
		return fmt.Errorf("unsupported Laravel version %q — valid options: 12.*, 13.*, latest", cfg.Laravel.Version)
	}
	if !validRuntimes[cfg.PHP.Runtime] {
		return fmt.Errorf("unsupported runtime %q — valid options: frankenphp, fpm", cfg.PHP.Runtime)
	}
	if !validPackageManagers[cfg.Node.PackageManager] {
		return fmt.Errorf("unsupported package manager %q — valid options: npm, pnpm, bun", cfg.Node.PackageManager)
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

	if err := validateWorkers(&cfg.Workers, explicitEmptyQueues); err != nil {
		return err
	}

	return nil
}

func validateWorkers(w *Workers, explicitEmptyQueues []bool) error {
	names := make(map[string]int, len(w.Queue))
	for i, p := range w.Queue {
		if i < len(explicitEmptyQueues) && explicitEmptyQueues[i] {
			return fmt.Errorf("workers.queue[%d]: queues must not be empty", i)
		}
		if p.Count < 1 {
			return fmt.Errorf("workers.queue[%d] (%s): count must be ≥ 1", i, p.Name)
		}
		if !workerPoolNameRe.MatchString(p.Name) {
			return fmt.Errorf("workers.queue[%d]: invalid pool name %q — must match [a-z0-9_-]+", i, p.Name)
		}
		if p.Tries < 0 || p.Timeout < 0 || p.Memory < 0 || p.Sleep < 0 || p.Backoff < 0 {
			return fmt.Errorf("workers.queue[%d] (%s): passthrough values must be ≥ 0", i, p.Name)
		}
		for _, q := range p.Queues {
			if q == "" {
				return fmt.Errorf("workers.queue[%d] (%s): queue name must not be empty", i, p.Name)
			}
		}
		if prev, ok := names[p.Name]; ok {
			return fmt.Errorf("workers.queue[%d] and [%d]: duplicate pool name %q", prev, i, p.Name)
		}
		names[p.Name] = i
	}
	return nil
}

// warnUnknownWorkerKeys emits a warning for unknown keys under the workers
// block or any queue item, for forward-compat with future fields.
func warnUnknownWorkerKeys(root *yaml.Node) {
	if root.Kind != yaml.DocumentNode || len(root.Content) == 0 {
		return
	}
	top := root.Content[0]
	if top.Kind != yaml.MappingNode {
		return
	}
	workers := mapValue(top, "workers")
	if workers == nil || workers.Kind != yaml.MappingNode {
		return
	}
	for i := 0; i+1 < len(workers.Content); i += 2 {
		key := workers.Content[i].Value
		if !knownWorkersKeys[key] {
			fmt.Fprintf(os.Stderr, "warning: unknown key %q under workers — ignored\n", key)
		}
	}
	queue := mapValue(workers, "queue")
	if queue == nil || queue.Kind != yaml.SequenceNode {
		return
	}
	for idx, item := range queue.Content {
		if item.Kind != yaml.MappingNode {
			continue
		}
		for i := 0; i+1 < len(item.Content); i += 2 {
			key := item.Content[i].Value
			if !knownQueueItemKeys[key] {
				fmt.Fprintf(os.Stderr, "warning: unknown key %q under workers.queue[%d] — ignored\n", key, idx)
			}
		}
	}
}

// warnUnknownNodeKeys emits a warning for unknown keys under the node block,
// for forward-compat with future fields.
func warnUnknownNodeKeys(root *yaml.Node) {
	if root.Kind != yaml.DocumentNode || len(root.Content) == 0 {
		return
	}
	top := root.Content[0]
	if top.Kind != yaml.MappingNode {
		return
	}
	node := mapValue(top, "node")
	if node == nil || node.Kind != yaml.MappingNode {
		return
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		key := node.Content[i].Value
		if !knownNodeKeys[key] {
			fmt.Fprintf(os.Stderr, "warning: unknown key %q under node — ignored\n", key)
		}
	}
}

func mapValue(m *yaml.Node, key string) *yaml.Node {
	if m.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			return m.Content[i+1]
		}
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
