package activeproject

import (
	"os"
	"path/filepath"
	"testing"
)

// useTempRoot points the package at a throwaway state dir for one test.
func useTempRoot(t *testing.T) string {
	t.Helper()

	root := t.TempDir()
	original := stateRoot
	stateRoot = root
	t.Cleanup(func() { stateRoot = original })

	return root
}

func TestState(t *testing.T) {
	tests := []struct {
		name string
		run  func(t *testing.T, root string)
	}{
		{
			name: "write then read round-trips",
			run: func(t *testing.T, root string) {
				dir := t.TempDir()
				if err := Write(dir, "demo"); err != nil {
					t.Fatalf("Write: %v", err)
				}

				got, err := Read()
				if err != nil {
					t.Fatalf("Read: %v", err)
				}
				if got.Dir != filepath.Clean(dir) || got.Project != "demo" {
					t.Fatalf("got %+v, want dir=%q project=%q", got, dir, "demo")
				}
			},
		},
		{
			name: "absent file reads as zero state",
			run: func(t *testing.T, root string) {
				got, err := Read()
				if err != nil {
					t.Fatalf("Read: %v", err)
				}
				if got != (State{}) {
					t.Fatalf("got %+v, want zero State", got)
				}
			},
		},
		{
			name: "malformed json reads as zero state",
			run: func(t *testing.T, root string) {
				path := filepath.Join(root, stateFileName)
				if err := os.WriteFile(path, []byte("{not json"), 0644); err != nil {
					t.Fatalf("seed: %v", err)
				}

				got, err := Read()
				if err != nil {
					t.Fatalf("Read: %v", err)
				}
				if got != (State{}) {
					t.Fatalf("got %+v, want zero State", got)
				}
			},
		},
		{
			name: "clear with matching dir empties state",
			run: func(t *testing.T, root string) {
				dir := t.TempDir()
				if err := Write(dir, "demo"); err != nil {
					t.Fatalf("Write: %v", err)
				}

				if err := Clear(dir); err != nil {
					t.Fatalf("Clear: %v", err)
				}

				got, err := Read()
				if err != nil {
					t.Fatalf("Read: %v", err)
				}
				if got != (State{}) {
					t.Fatalf("got %+v, want zero State", got)
				}
			},
		},
		{
			name: "clear with other dir leaves state untouched",
			run: func(t *testing.T, root string) {
				dir := t.TempDir()
				if err := Write(dir, "demo"); err != nil {
					t.Fatalf("Write: %v", err)
				}

				if err := Clear(t.TempDir()); err != nil {
					t.Fatalf("Clear: %v", err)
				}

				got, err := Read()
				if err != nil {
					t.Fatalf("Read: %v", err)
				}
				if got.Dir != filepath.Clean(dir) || got.Project != "demo" {
					t.Fatalf("got %+v, want state preserved for %q", got, dir)
				}
			},
		},
		{
			// `frank up --dir .` then a bare `frank up` must not look like two
			// different projects.
			name: "relative dir matches the absolute one",
			run: func(t *testing.T, root string) {
				dir := t.TempDir()
				if err := Write(dir, "demo"); err != nil {
					t.Fatalf("Write: %v", err)
				}

				t.Chdir(dir)
				if err := Clear("."); err != nil {
					t.Fatalf("Clear: %v", err)
				}

				got, err := Read()
				if err != nil {
					t.Fatalf("Read: %v", err)
				}
				if got != (State{}) {
					t.Fatalf("got %+v, want relative dir to match and clear", got)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.run(t, useTempRoot(t))
		})
	}
}

func TestResolveStateRootIgnoresRelativeXDG(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home dir")
	}

	t.Setenv("XDG_STATE_HOME", "relative/path")
	if got, want := resolveStateRoot(), filepath.Join(home, ".local", "state", "frank"); got != want {
		t.Fatalf("got %q, want %q", got, want)
	}

	t.Setenv("XDG_STATE_HOME", filepath.Join(home, "xdgstate"))
	if got, want := resolveStateRoot(), filepath.Join(home, "xdgstate", "frank"); got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}
