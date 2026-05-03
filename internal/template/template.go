package template

import (
	"bytes"
	"fmt"
	"io/fs"
	"path"
	texttemplate "text/template"

	"github.com/phlisg/frank/internal/config"
)

// Data is the context passed to every template.
type Data struct {
	// Runtime templates
	PHPVersion string
	// Project-level
	ProjectName string
	// Server/TLS
	HTTPS      bool
	ServerPort int
	CustomPort bool
	// Service-level (populated per service)
	Version       string
	Port          int
	DashboardPort int // mailpit UI port
}

// WorkerData is the context passed to worker fragment templates.
// Fields not applicable to a given kind (e.g. schedule has no pool) are zero-valued.
type WorkerData struct {
	ProjectName string
	// ServiceName is the full compose service key for a queue worker,
	// e.g. "laravel.queue.default.1". Unused by the schedule fragment.
	ServiceName string
	// PoolName is the queue pool identifier (label frank.worker.pool).
	PoolName string
	// QueuesCSV is the comma-joined queue list for --queue.
	QueuesCSV string
	// Passthrough flags — rendered only when > 0.
	Tries   int
	Timeout int
	Memory  int
	Sleep   int
	Backoff int
}

// defaultPorts maps service name → default host port.
var defaultPorts = map[string]int{
	"pgsql":       5432,
	"mysql":       3306,
	"mariadb":     3306,
	"redis":       6379,
	"memcached":   11211,
	"meilisearch": 7700,
	"mailpit":     1025,
}

const defaultMailpitDashboardPort = 8025

// defaultVersions maps service name → default image tag.
var defaultVersions = map[string]string{
	"pgsql":       "latest",
	"mysql":       "latest",
	"mariadb":     "latest",
	"redis":       "alpine",
	"memcached":   "alpine",
	"meilisearch": "latest",
	"mailpit":     "latest",
}

// Engine renders templates from an fs.FS rooted at the templates directory.
type Engine struct {
	fs fs.FS
}

// New creates an Engine backed by fsys.
// For production use, pass the embed.FS from main.
// For tests, pass os.DirFS("path/to/templates").
func New(fsys fs.FS) *Engine {
	return &Engine{fs: fsys}
}

// Render executes the template at the given path within the FS with data.
func (e *Engine) Render(tmplPath string, data Data) (string, error) {
	content, err := fs.ReadFile(e.fs, tmplPath)
	if err != nil {
		return "", fmt.Errorf("template %q not found: %w", tmplPath, err)
	}

	t, err := texttemplate.New(path.Base(tmplPath)).Parse(string(content))
	if err != nil {
		return "", fmt.Errorf("template %q parse error: %w", tmplPath, err)
	}

	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("template %q render error: %w", tmplPath, err)
	}

	return buf.String(), nil
}

// RenderRuntime renders a file from templates/runtimes/<runtime>/.
func (e *Engine) RenderRuntime(runtime, file string, data Data) (string, error) {
	return e.Render(path.Join("templates", "runtimes", runtime, file), data)
}

// RenderWorker renders templates/workers/<kind>.fragment.tmpl where kind is
// "schedule" or "queue". Templates stay runtime-agnostic; any runtime-specific
// injection (e.g. `user: sail` for fpm) is the caller's responsibility.
func (e *Engine) RenderWorker(kind string, data WorkerData) (string, error) {
	tmplPath := path.Join("templates", "workers", kind+".fragment.tmpl")
	content, err := fs.ReadFile(e.fs, tmplPath)
	if err != nil {
		return "", fmt.Errorf("worker template %q not found: %w", tmplPath, err)
	}
	t, err := texttemplate.New(path.Base(tmplPath)).Parse(string(content))
	if err != nil {
		return "", fmt.Errorf("worker template %q parse error: %w", tmplPath, err)
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("worker template %q render error: %w", tmplPath, err)
	}
	return buf.String(), nil
}

// RenderServiceCompose renders the compose.fragment.tmpl for a service.
// ServiceConfig port/version override defaults if non-zero/non-empty.
func (e *Engine) RenderServiceCompose(service string, cfg config.ServiceConfig, projectName string) (string, error) {
	data := e.serviceData(service, cfg, projectName)
	return e.Render(path.Join("templates", "services", service, "compose.fragment.tmpl"), data)
}

// ReadFile reads raw file content from the engine's FS.
func (e *Engine) ReadFile(filePath string) (string, error) {
	data, err := fs.ReadFile(e.fs, filePath)
	if err != nil {
		return "", fmt.Errorf("read %q: %w", filePath, err)
	}
	return string(data), nil
}

// RenderServiceEnv renders the env.tmpl for a service.
func (e *Engine) RenderServiceEnv(service string, cfg config.ServiceConfig, projectName string) (string, error) {
	data := e.serviceData(service, cfg, projectName)
	return e.Render(path.Join("templates", "services", service, "env.tmpl"), data)
}

// serviceData builds a Data value for a service, applying config overrides
// on top of per-service defaults.
func (e *Engine) serviceData(service string, cfg config.ServiceConfig, projectName string) Data {
	port := defaultPorts[service]
	if cfg.Port != 0 {
		port = cfg.Port
	}

	version := defaultVersions[service]
	if cfg.Version != "" {
		version = cfg.Version
	}

	dashboardPort := defaultMailpitDashboardPort

	return Data{
		ProjectName:   projectName,
		Version:       version,
		Port:          port,
		DashboardPort: dashboardPort,
	}
}
