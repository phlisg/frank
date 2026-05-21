package worktreelist

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/phlisg/frank/internal/config"
	"github.com/phlisg/frank/internal/docker"
)

// WorktreeItem represents a single linked git worktree with its container status.
type WorktreeItem struct {
	Path     string
	Branch   string
	HasFrank bool
	Services []ServiceInfo
}

// ServiceInfo holds one container's name, state, and port mappings.
type ServiceInfo struct {
	Name       string
	State      string
	Ports      string
	Publishers []Publisher
}

// Publisher is a single port mapping from docker compose ps.
type Publisher struct {
	TargetPort    int
	PublishedPort int
	Protocol      string
}

// StatusLabel returns a human-readable status string.
func (w WorktreeItem) StatusLabel() string {
	if !w.HasFrank {
		return "not configured"
	}
	if len(w.Services) == 0 {
		return "stopped"
	}
	running := 0
	for _, s := range w.Services {
		if s.State == "running" {
			running++
		}
	}
	total := len(w.Services)
	if running == 0 {
		return "stopped"
	}
	if running == total {
		return fmt.Sprintf("running (%d/%d)", running, total)
	}
	return fmt.Sprintf("partial (%d/%d)", running, total)
}

// PortSummary returns a compact port listing from running services.
func (w WorktreeItem) PortSummary() string {
	var ports []string
	for _, s := range w.Services {
		if s.Ports != "" && s.State == "running" {
			ports = append(ports, s.Ports)
		}
	}
	return strings.Join(ports, " ")
}

// IsRunning returns true if at least one service is running.
func (w WorktreeItem) IsRunning() bool {
	for _, s := range w.Services {
		if s.State == "running" {
			return true
		}
	}
	return false
}

// Discover enumerates linked git worktrees and probes their container status.
func Discover(dir string) ([]WorktreeItem, error) {
	entries, err := parseWorktrees(dir)
	if err != nil {
		return nil, err
	}

	var items []WorktreeItem
	for _, e := range entries {
		item := WorktreeItem{
			Path:   e.path,
			Branch: e.branch,
		}

		frankYAML := filepath.Join(e.path, config.ConfigFileName)
		if _, err := os.Stat(frankYAML); err == nil {
			item.HasFrank = true
		}

		composeFile := filepath.Join(e.path, ".frank", "compose.yaml")
		if item.HasFrank {
			if _, err := os.Stat(composeFile); err == nil {
				item.Services = probeServices(e.path)
			}
		}

		items = append(items, item)
	}
	return items, nil
}

type worktreeEntry struct {
	path   string
	branch string
}

// parseWorktrees runs `git worktree list --porcelain` and returns linked
// worktrees (skips the first entry which is always the main working tree).
func parseWorktrees(dir string) ([]worktreeEntry, error) {
	cmd := exec.Command("git", "worktree", "list", "--porcelain")
	cmd.Dir = dir
	cmd.Env = config.CleanGitEnv()
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git worktree list: %w", err)
	}
	return parsePorcelain(string(out)), nil
}

// parsePorcelain parses `git worktree list --porcelain` output, skipping
// the first entry (main working tree).
func parsePorcelain(raw string) []worktreeEntry {
	blocks := splitPorcelainBlocks(raw)
	if len(blocks) <= 1 {
		return nil
	}

	var entries []worktreeEntry
	for _, block := range blocks[1:] {
		e := parsePorcelainBlock(block)
		if e.path != "" {
			entries = append(entries, e)
		}
	}
	return entries
}

func splitPorcelainBlocks(raw string) []string {
	var blocks []string
	var current []string
	for _, line := range strings.Split(raw, "\n") {
		if line == "" {
			if len(current) > 0 {
				blocks = append(blocks, strings.Join(current, "\n"))
				current = nil
			}
			continue
		}
		current = append(current, line)
	}
	if len(current) > 0 {
		blocks = append(blocks, strings.Join(current, "\n"))
	}
	return blocks
}

func parsePorcelainBlock(block string) worktreeEntry {
	var e worktreeEntry
	for _, line := range strings.Split(block, "\n") {
		if strings.HasPrefix(line, "worktree ") {
			e.path = strings.TrimPrefix(line, "worktree ")
		}
		if strings.HasPrefix(line, "branch refs/heads/") {
			e.branch = strings.TrimPrefix(line, "branch refs/heads/")
		}
	}
	if e.branch == "" && e.path != "" {
		e.branch = "(detached)"
	}
	return e
}

// composePSEntry matches the JSON output from `docker compose ps --format json`.
type composePSEntry struct {
	Service    string `json:"Service"`
	State      string `json:"State"`
	Publishers []struct {
		URL           string `json:"URL"`
		TargetPort    int    `json:"TargetPort"`
		PublishedPort int    `json:"PublishedPort"`
		Protocol      string `json:"Protocol"`
	} `json:"Publishers"`
}

func probeServices(worktreePath string) []ServiceInfo {
	client := docker.New(worktreePath)
	out, err := client.RunQuiet("ps", "--format", "json")
	if err != nil {
		return nil
	}

	var services []ServiceInfo
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var entry composePSEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}
		var pubs []Publisher
		for _, p := range entry.Publishers {
			if p.PublishedPort > 0 {
				pubs = append(pubs, Publisher{
					TargetPort:    p.TargetPort,
					PublishedPort: p.PublishedPort,
					Protocol:      p.Protocol,
				})
			}
		}
		services = append(services, ServiceInfo{
			Name:       entry.Service,
			State:      entry.State,
			Ports:      formatPorts(entry),
			Publishers: pubs,
		})
	}
	return services
}

func formatPorts(entry composePSEntry) string {
	seen := make(map[int]bool)
	var parts []string
	for _, p := range entry.Publishers {
		if p.PublishedPort > 0 && !seen[p.PublishedPort] {
			seen[p.PublishedPort] = true
			parts = append(parts, fmt.Sprintf(":%d", p.PublishedPort))
		}
	}
	return strings.Join(parts, " ")
}

// WebPort returns the published TCP port for the web server (443 or 80)
// from the laravel.test service, or 0 if none found.
func (w WorktreeItem) WebPort() int {
	for _, s := range w.Services {
		if s.Name != "laravel.test" {
			continue
		}
		// Prefer 443/tcp, fall back to 80/tcp.
		for _, target := range []int{443, 80} {
			for _, p := range s.Publishers {
				if p.TargetPort == target && p.Protocol == "tcp" {
					return p.PublishedPort
				}
			}
		}
	}
	return 0
}
