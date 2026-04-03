package docker

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
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
	cmdArgs := append([]string{"compose"}, args...)
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
