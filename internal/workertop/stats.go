// Package workertop implements the `frank worker top` TUI.
//
// This file owns the statsHub: a background goroutine that periodically
// shells out to `docker stats --no-stream` for a fixed set of container
// IDs, parses the output into per-container samples, and broadcasts
// snapshots over an Updates channel.
//
// The hub is pure in the sense that it never touches the docker CLI
// directly — it calls through an ExecFn, so tests can inject a canned
// stdout without spinning up containers.
package workertop

import (
	"bufio"
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// DefaultInterval is the sample cadence used when callers pass a
// non-positive interval to NewHub.
const DefaultInterval = 2 * time.Second

// StatsSample is a single container's memory reading at one point in time.
type StatsSample struct {
	ContainerID string
	MemPct      float64
	MemBytes    int64
}

// ExecFn mirrors exec.CommandContext(...).Output(). Tests inject a mock;
// production wires DefaultExecFn.
type ExecFn func(ctx context.Context, name string, args ...string) ([]byte, error)

// DefaultExecFn runs the command via os/exec and returns its stdout.
func DefaultExecFn(ctx context.Context, name string, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, name, args...).Output()
}

// Hub periodically samples `docker stats` for a fixed container set and
// broadcasts parsed snapshots. Construct via NewHub, then call Run under
// a cancellable context. Consumers read Updates().
type Hub struct {
	containerIDs []string
	interval     time.Duration
	exec         ExecFn
	updates      chan map[string]StatsSample
}

// NewHub constructs a Hub. If interval <= 0 it falls back to
// DefaultInterval; if exec is nil it falls back to DefaultExecFn.
func NewHub(containerIDs []string, interval time.Duration, exec ExecFn) *Hub {
	if interval <= 0 {
		interval = DefaultInterval
	}
	if exec == nil {
		exec = DefaultExecFn
	}
	// Unbuffered: slow consumers naturally throttle the sampler. A
	// buffered channel would let stale snapshots pile up, which is
	// worse than dropping ticks.
	return &Hub{
		containerIDs: append([]string(nil), containerIDs...),
		interval:     interval,
		exec:         exec,
		updates:      make(chan map[string]StatsSample),
	}
}

// Updates returns the broadcast channel. Closed when Run returns.
func (h *Hub) Updates() <-chan map[string]StatsSample {
	return h.updates
}

// Run drives the sampling loop until ctx is cancelled. It is safe to
// call exactly once per Hub. Always defers channel close so consumers
// detect shutdown via a nil-map receive.
func (h *Hub) Run(ctx context.Context) {
	defer close(h.updates)

	// No IDs → nothing to do; we must never run `docker stats` without
	// an explicit list (it would dump every container on the host).
	if len(h.containerIDs) == 0 {
		<-ctx.Done()
		return
	}

	// Sample immediately so consumers don't wait a full interval for
	// the first snapshot. Then tick.
	h.sampleAndEmit(ctx)

	ticker := time.NewTicker(h.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			h.sampleAndEmit(ctx)
		}
	}
}

// sampleAndEmit runs one docker stats invocation, parses every line,
// and publishes the resulting snapshot. Parse errors on individual
// lines are dropped silently — a malformed row shouldn't kill the hub.
// Subprocess errors likewise drop the tick (the next tick retries).
func (h *Hub) sampleAndEmit(ctx context.Context) {
	args := append(
		[]string{"stats", "--no-stream", "--format", "{{.ID}} {{.MemPerc}} {{.MemUsage}}"},
		h.containerIDs...,
	)
	out, err := h.exec(ctx, "docker", args...)
	if err != nil {
		return
	}

	snapshot := make(map[string]StatsSample, len(h.containerIDs))
	scanner := bufio.NewScanner(strings.NewReader(string(out)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		sample, err := parseStatsLine(line)
		if err != nil {
			continue
		}
		snapshot[sample.ContainerID] = sample
	}

	select {
	case <-ctx.Done():
		return
	case h.updates <- snapshot:
	}
}

// parseStatsLine parses a single line of
//
//	{{.ID}} {{.MemPerc}} {{.MemUsage}}
//
// where MemUsage itself contains spaces, e.g. "128MiB / 2GiB". We only
// care about tokens [0], [1], and [2] — the "/ <limit>" tail is
// discarded.
func parseStatsLine(line string) (StatsSample, error) {
	fields := strings.Fields(line)
	// Need at least ID, MemPerc, used, "/", limit — docker always
	// emits the divider.
	if len(fields) < 4 {
		return StatsSample{}, fmt.Errorf("workertop: stats line has %d fields, want >=4: %q", len(fields), line)
	}

	pctToken := strings.TrimSuffix(fields[1], "%")
	pct, err := strconv.ParseFloat(pctToken, 64)
	if err != nil {
		return StatsSample{}, fmt.Errorf("workertop: parse MemPerc %q: %w", fields[1], err)
	}

	bytesVal, err := parseBytes(fields[2])
	if err != nil {
		return StatsSample{}, fmt.Errorf("workertop: parse MemUsage %q: %w", fields[2], err)
	}

	return StatsSample{
		ContainerID: fields[0],
		MemPct:      pct,
		MemBytes:    bytesVal,
	}, nil
}

// byteUnits maps docker's human-readable size suffixes to their byte
// multipliers. docker emits IEC (KiB/MiB/GiB/TiB) by default but older
// versions and some formatters surface SI (KB/MB/GB/TB), so we accept
// both. Longer suffixes are ordered first so "MiB" matches before "B".
var byteUnits = []struct {
	suffix string
	mult   int64
}{
	{"KiB", 1 << 10},
	{"MiB", 1 << 20},
	{"GiB", 1 << 30},
	{"TiB", 1 << 40},
	{"KB", 1_000},
	{"MB", 1_000_000},
	{"GB", 1_000_000_000},
	{"TB", 1_000_000_000_000},
	{"B", 1},
}

// parseBytes turns "128MiB" / "1.5GiB" / "2GB" / "1024KiB" into a byte
// count. Returns an error for unknown suffixes or malformed numeric
// portions.
func parseBytes(s string) (int64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("workertop: empty size string")
	}
	for _, u := range byteUnits {
		if !strings.HasSuffix(s, u.suffix) {
			continue
		}
		numStr := strings.TrimSpace(strings.TrimSuffix(s, u.suffix))
		if numStr == "" {
			return 0, fmt.Errorf("workertop: size %q missing numeric part", s)
		}
		num, err := strconv.ParseFloat(numStr, 64)
		if err != nil {
			return 0, fmt.Errorf("workertop: parse size %q: %w", s, err)
		}
		return int64(num * float64(u.mult)), nil
	}
	return 0, fmt.Errorf("workertop: unknown size unit in %q", s)
}
