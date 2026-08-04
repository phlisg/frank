// Package activeproject records which Frank project is currently active, in a
// single global state file outside any project directory. Frank publishes fixed
// host ports for non-worktree projects, so only one can run at a time; this
// pointer is what lets `frank up` stop the previous one from a different cwd.
package activeproject

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

const stateFileName = "active-project.json"

// stateRoot is the directory holding the state file. Empty means the state
// directory could not be resolved (os.UserHomeDir failed), in which case every
// function below is a silent no-op — this package must never be the reason
// `frank up` fails. Tests overwrite it with t.TempDir().
var stateRoot = resolveStateRoot()

// State is the one record in the state file. Dir is authoritative and always
// absolute; Project is display-only, so "Stopping X" still reads well after the
// directory has been deleted.
type State struct {
	Dir     string `json:"dir"`
	Project string `json:"project"`
}

// Read returns the active project, or a zero State when there is none.
// A missing, unreadable or malformed file is reported as "no active project" —
// callers have no use for the distinction, both mean nothing to stop.
func Read() (State, error) {
	if stateRoot == "" {
		return State{}, nil
	}

	data, err := os.ReadFile(filepath.Join(stateRoot, stateFileName))
	if err != nil {
		return State{}, nil
	}

	var s State
	if err := json.Unmarshal(data, &s); err != nil {
		return State{}, nil
	}

	return s, nil
}

// Write records dir as the active project. dir is normalized so that a later
// bare `frank up` in the same project compares equal to one run with a relative
// `--dir`.
func Write(dir, project string) error {
	if stateRoot == "" {
		return nil
	}

	abs, err := normalize(dir)
	if err != nil {
		return err
	}

	data, err := json.Marshal(State{Dir: abs, Project: project})
	if err != nil {
		return fmt.Errorf("encode active project state: %w", err)
	}

	return writeFile(data)
}

// Clear removes the active-project pointer, but only when it names dir —
// downing project A must not clear a pointer to project B.
func Clear(dir string) error {
	if stateRoot == "" {
		return nil
	}

	abs, err := normalize(dir)
	if err != nil {
		return err
	}

	current, err := Read()
	if err != nil || current.Dir != abs {
		return err
	}

	if err := os.Remove(filepath.Join(stateRoot, stateFileName)); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("clear active project state: %w", err)
	}

	return nil
}

// writeFile swaps the state file in atomically: two extra lines to rule out a
// torn read. It does not make concurrent writers safe — last one wins, which is
// accepted (see the design doc).
func writeFile(data []byte) error {
	if err := os.MkdirAll(stateRoot, 0755); err != nil {
		return fmt.Errorf("create state dir: %w", err)
	}

	tmp, err := os.CreateTemp(stateRoot, stateFileName+".*")
	if err != nil {
		return fmt.Errorf("create temp state file: %w", err)
	}
	defer os.Remove(tmp.Name()) // no-op once the rename succeeds

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("write active project state: %w", err)
	}

	if err := tmp.Close(); err != nil {
		return fmt.Errorf("write active project state: %w", err)
	}

	if err := os.Rename(tmp.Name(), filepath.Join(stateRoot, stateFileName)); err != nil {
		return fmt.Errorf("write active project state: %w", err)
	}

	return nil
}

func normalize(dir string) (string, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", fmt.Errorf("resolve project dir %q: %w", dir, err)
	}

	return filepath.Clean(abs), nil
}

// resolveStateRoot follows the XDG base directory spec: a non-absolute
// XDG_STATE_HOME is ignored rather than honoured.
func resolveStateRoot() string {
	if base := os.Getenv("XDG_STATE_HOME"); filepath.IsAbs(base) {
		return filepath.Join(base, "frank")
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}

	return filepath.Join(home, ".local", "state", "frank")
}
