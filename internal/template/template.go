package template

import (
	"bytes"
	"fmt"
	"io/fs"
	"path"
	texttemplate "text/template"

	"github.com/phlisg/frank-cli/internal/config"
)

// Data is the context passed to every template.
type Data struct {
	// Runtime templates
	PHPVersion string
	// Project-level
	ProjectName string
	// Service-level (populated per service)
	Version       string
	Port          int
	DashboardPort int // mailpit UI port
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

// RenderServiceCompose renders the compose.fragment.tmpl for a service.
// ServiceConfig port/version override defaults if non-zero/non-empty.
func (e *Engine) RenderServiceCompose(service string, cfg config.ServiceConfig, projectName string) (string, error) {
	data := e.serviceData(service, cfg, projectName)
	return e.Render(path.Join("templates", "services", service, "compose.fragment.tmpl"), data)
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
