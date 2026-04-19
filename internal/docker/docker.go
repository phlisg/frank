package docker

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
	"time"
)

// ContainerState represents the running state of a project's containers.
type ContainerState int

const (
	StateUnknown ContainerState = iota
	StateStopped
	StateRunning
	StatePartial // some containers running, some not
)

// Client is a thin wrapper around the docker compose CLI.
type Client struct {
	// dir is the project directory containing compose.yaml.
	dir string
}

// New creates a Client targeting the given project directory.
func New(dir string) *Client {
	return &Client{dir: dir}
}

// CheckDependencies verifies that docker and docker compose are available and
// the daemon is running. Returns a clear error message if not.
func CheckDependencies() error {
	if _, err := exec.LookPath("docker"); err != nil {
		return errors.New("docker not found — please install Docker: https://docs.docker.com/get-docker/")
	}

	// Check docker compose (plugin form)
	out, err := exec.Command("docker", "compose", "version").Output()
	if err != nil {
		return errors.New("docker compose plugin not found — please install Docker Compose: https://docs.docker.com/compose/install/")
	}
	if !strings.Contains(string(out), "Docker Compose") {
		return errors.New("unexpected output from 'docker compose version' — please ensure Docker Compose v2 is installed")
	}

	// Check daemon is running
	if err := exec.Command("docker", "info").Run(); err != nil {
		return errors.New("docker daemon is not running — please start Docker")
	}

	return nil
}

// Run executes `docker compose <args>` in the project directory,
// streaming stdout/stderr directly to the terminal.
func (c *Client) Run(args ...string) error {
	cmd := c.composeCmd(args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	return runCmd(cmd)
}

// RunQuiet executes `docker compose <args>` and captures output,
// returning it as a string. Used for state detection (e.g. ps).
func (c *Client) RunQuiet(args ...string) (string, error) {
	cmd := c.composeCmd(args...)
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	err := runCmd(cmd)
	return buf.String(), err
}

// Exec runs `docker compose exec --user sail <service> <command...>` streaming I/O.
// --user sail ensures files created inside the container are owned by sail (remapped
// to the host user's UID via usermod in the entrypoint), not root.
func (c *Client) Exec(service string, command ...string) error {
	args := append([]string{"exec", "--user", "sail", service}, command...)
	return c.Run(args...)
}

// ExecQuiet runs `docker compose exec --user sail <service> <command...>` and captures output.
func (c *Client) ExecQuiet(service string, command ...string) (string, error) {
	args := append([]string{"exec", "--user", "sail", service}, command...)
	return c.RunQuiet(args...)
}

// Up runs `docker compose up` with optional extra args (e.g. "-d", "--build").
func (c *Client) Up(extraArgs ...string) error {
	args := append([]string{"up"}, extraArgs...)
	return c.Run(args...)
}

// Down runs `docker compose down`.
func (c *Client) Down() error {
	return c.Run("down")
}

// PS runs `docker compose ps`.
func (c *Client) PS() error {
	return c.Run("ps")
}

// PSWorkers lists worker containers (both declared and ad-hoc) for the given
// project by filtering on the `frank.project` and `frank.worker` labels.
//
// This bypasses `docker compose ps` because ad-hoc workers are launched with
// plain `docker run` and are therefore invisible to compose.
func (c *Client) PSWorkers(projectName string) error {
	args := []string{
		"ps",
		"--filter", "label=frank.project=" + projectName,
		"--filter", "label=frank.worker",
		"--format", "table {{.Names}}\t{{.Status}}\t{{.Label \"frank.worker\"}}\t{{.Label \"frank.worker.name\"}}",
	}
	cmd := exec.Command("docker", args...)
	cmd.Dir = c.dir

	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = os.Stderr
	if err := runCmd(cmd); err != nil {
		return err
	}

	out := buf.String()
	// `docker ps --format table ...` always emits a header row. If the only
	// line present is the header, there are no matching containers.
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) <= 1 {
		fmt.Printf("No worker containers found for project %s.\n", projectName)
		return nil
	}

	fmt.Print(out)
	return nil
}

// RunAdhoc spawns a detached container via `docker compose run -d` with the
// given name and labels. The container inherits image/env/network/entrypoint
// from the `laravel.test` service. cmdArgs are appended verbatim after the
// service name (e.g. ["php", "artisan", "queue:work", "--queue=default"]).
func (c *Client) RunAdhoc(name string, labels map[string]string, cmdArgs []string) error {
	args := []string{"run", "-d", "--name", name}
	// Sort label keys for deterministic arg order (makes tests + user output stable).
	keys := make([]string, 0, len(labels))
	for k := range labels {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		args = append(args, "--label", k+"="+labels[k])
	}
	args = append(args, "laravel.test")
	args = append(args, cmdArgs...)
	return c.Run(args...)
}

// StopContainers stops + removes containers by name using `docker rm -f`.
// SIGKILL is used; queue workers can be restarted safely.
func (c *Client) StopContainers(names []string) error {
	if len(names) == 0 {
		return nil
	}
	args := append([]string{"rm", "-f"}, names...)
	cmd := exec.Command("docker", args...)
	cmd.Dir = c.dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return runCmd(cmd)
}

// ListContainers runs `docker ps` filtered by the frank.project label and
// optionally by the frank.worker label value. workerFilter is one of
// "declared", "adhoc", or "" (both). The format string is passed through to
// `--format`. Output is captured and returned.
func (c *Client) ListContainers(projectName string, workerFilter string, format string) (string, error) {
	args := []string{
		"ps",
		"--filter", "label=frank.project=" + projectName,
	}
	if workerFilter == "" {
		args = append(args, "--filter", "label=frank.worker")
	} else {
		args = append(args, "--filter", "label=frank.worker="+workerFilter)
	}
	if format != "" {
		args = append(args, "--format", format)
	}
	cmd := exec.Command("docker", args...)
	cmd.Dir = c.dir
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = os.Stderr
	if err := runCmd(cmd); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// LogsForWorkers streams `docker compose logs` for the given service list.
// If follow is true, `-f` is passed.
func (c *Client) LogsForWorkers(services []string, follow bool) error {
	args := []string{"logs"}
	if follow {
		args = append(args, "-f")
	}
	args = append(args, services...)
	return c.Run(args...)
}

// ComposePSServiceExists returns true if the given service name resolves to
// a container under the current compose project. Used to decide whether to
// use `docker compose logs <name>` vs `docker logs <name>` for ad-hoc workers.
func (c *Client) ComposePSServiceExists(name string) bool {
	out, err := c.RunQuiet("ps", "-q", name)
	if err != nil {
		return false
	}
	return strings.TrimSpace(out) != ""
}

// LogsRaw streams `docker logs <name>` for a non-compose container (ad-hoc
// workers launched via `docker compose run -d` still show up under
// `docker logs`, but not necessarily under `docker compose logs`).
func (c *Client) LogsRaw(name string, follow bool) error {
	args := []string{"logs"}
	if follow {
		args = append(args, "-f")
	}
	args = append(args, name)
	cmd := exec.Command("docker", args...)
	cmd.Dir = c.dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	return runCmd(cmd)
}

// Clean runs `docker compose down -v` (removes volumes).
func (c *Client) Clean() error {
	return c.Run("down", "-v")
}

// ContainerStatus queries the running state of the project's containers.
// Uses a quiet ps call so it doesn't print to the terminal.
func (c *Client) ContainerStatus() (ContainerState, int, int) {
	out, err := c.RunQuiet("ps", "--format", "json")
	if err != nil {
		return StateStopped, 0, 0
	}
	return parseContainerStatus(out)
}

// WaitForContainer polls until the named service is exec-able (i.e. running and
// accepting commands), or until timeout is exceeded.
// Uses a zero-exit exec ("true") as a lightweight readiness probe.
func (c *Client) WaitForContainer(service string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if _, err := c.ExecQuiet(service, "true"); err == nil {
			return nil
		}
		time.Sleep(2 * time.Second)
	}
	return fmt.Errorf("container %q did not become ready within %s", service, timeout)
}

// parseContainerStatus interprets the JSON output from `docker compose ps --format json`.
// Each non-empty line is a JSON object for one container.
func parseContainerStatus(out string) (ContainerState, int, int) {
	if strings.TrimSpace(out) == "" {
		return StateStopped, 0, 0
	}

	var running, total int
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		total++
		if strings.Contains(line, `"State":"running"`) || strings.Contains(line, `"Status":"running"`) {
			running++
		}
	}

	if running == 0 {
		return StateStopped, 0, 0
	}
	if running == total {
		return StateRunning, running, total
	}
	return StatePartial, running, total
}

// composeCmd builds an exec.Cmd for `docker compose <args>` in the project dir.
func (c *Client) composeCmd(args ...string) *exec.Cmd {
	prefix := []string{"compose", "--project-directory", ".", "-f", ".frank/compose.yaml"}
	cmdArgs := append(prefix, args...)
	cmd := exec.Command("docker", cmdArgs...)
	cmd.Dir = c.dir
	return cmd
}

// runCmd runs a command and translates the exit code into a readable error.
func runCmd(cmd *exec.Cmd) error {
	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return fmt.Errorf("command exited with code %d", exitErr.ExitCode())
		}
		return err
	}
	return nil
}
