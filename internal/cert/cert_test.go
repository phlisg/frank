package cert

import (
	"errors"
	"os"
	"testing"
)

// mockRunner implements commandRunner for testing.
type mockRunner struct {
	lookPathFn func(name string) (string, error)
	runFn      func(name string, args ...string) ([]byte, error)
}

func (m *mockRunner) LookPath(name string) (string, error) {
	return m.lookPathFn(name)
}

func (m *mockRunner) Run(name string, args ...string) ([]byte, error) {
	return m.runFn(name, args...)
}

// mockFS implements fileSystem for testing.
type mockFS struct {
	existsFn   func(path string) bool
	mkdirAllFn func(path string, perm os.FileMode) error
}

func (m *mockFS) Exists(path string) bool {
	return m.existsFn(path)
}

func (m *mockFS) MkdirAll(path string, perm os.FileMode) error {
	return m.mkdirAllFn(path, perm)
}

func TestGenerate_CertsAlreadyExist(t *testing.T) {
	fs := &mockFS{
		existsFn: func(path string) bool { return true },
	}
	runner := &mockRunner{}

	result, err := generate("/tmp/frank", runner, fs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Skipped {
		t.Error("expected Skipped=true")
	}
	if result.Generated || result.MkcertMissing || result.CANotTrusted {
		t.Error("expected only Skipped to be true")
	}
}

func TestGenerate_MkcertNotInstalled(t *testing.T) {
	fs := &mockFS{
		existsFn: func(path string) bool { return false },
	}
	runner := &mockRunner{
		lookPathFn: func(name string) (string, error) {
			return "", errors.New("not found")
		},
	}

	result, err := generate("/tmp/frank", runner, fs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.MkcertMissing {
		t.Error("expected MkcertMissing=true")
	}
	if result.Generated || result.Skipped {
		t.Error("expected only MkcertMissing to be true")
	}
}

func TestGenerate_CANotTrusted(t *testing.T) {
	fs := &mockFS{
		existsFn: func(path string) bool {
			// cert files don't exist, rootCA.pem doesn't exist
			return false
		},
		mkdirAllFn: func(path string, perm os.FileMode) error { return nil },
	}
	runner := &mockRunner{
		lookPathFn: func(name string) (string, error) {
			return "/usr/bin/mkcert", nil
		},
		runFn: func(name string, args ...string) ([]byte, error) {
			if len(args) == 1 && args[0] == "-CAROOT" {
				return []byte("/home/user/.local/share/mkcert\n"), nil
			}
			// cert generation succeeds
			return []byte("Created a new certificate\n"), nil
		},
	}

	result, err := generate("/tmp/frank", runner, fs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Generated {
		t.Error("expected Generated=true")
	}
	if !result.CANotTrusted {
		t.Error("expected CANotTrusted=true")
	}
}

func TestGenerate_Success(t *testing.T) {
	fs := &mockFS{
		existsFn: func(path string) bool {
			// cert files don't exist, but rootCA.pem does
			if path == "/home/user/.local/share/mkcert/rootCA.pem" {
				return true
			}
			return false
		},
		mkdirAllFn: func(path string, perm os.FileMode) error { return nil },
	}
	runner := &mockRunner{
		lookPathFn: func(name string) (string, error) {
			return "/usr/bin/mkcert", nil
		},
		runFn: func(name string, args ...string) ([]byte, error) {
			if len(args) == 1 && args[0] == "-CAROOT" {
				return []byte("/home/user/.local/share/mkcert\n"), nil
			}
			return []byte("Created a new certificate\n"), nil
		},
	}

	result, err := generate("/tmp/frank", runner, fs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Generated {
		t.Error("expected Generated=true")
	}
	if result.CANotTrusted {
		t.Error("expected CANotTrusted=false")
	}
	if result.Skipped || result.MkcertMissing {
		t.Error("expected Skipped and MkcertMissing to be false")
	}
}

func TestGenerate_MkcertRunFails(t *testing.T) {
	fs := &mockFS{
		existsFn: func(path string) bool {
			if path == "/home/user/.local/share/mkcert/rootCA.pem" {
				return true
			}
			return false
		},
		mkdirAllFn: func(path string, perm os.FileMode) error { return nil },
	}
	runner := &mockRunner{
		lookPathFn: func(name string) (string, error) {
			return "/usr/bin/mkcert", nil
		},
		runFn: func(name string, args ...string) ([]byte, error) {
			if len(args) == 1 && args[0] == "-CAROOT" {
				return []byte("/home/user/.local/share/mkcert\n"), nil
			}
			return []byte("ERROR: something went wrong"), errors.New("exit status 1")
		},
	}

	result, err := generate("/tmp/frank", runner, fs)
	if err == nil {
		t.Fatal("expected error")
	}
	if result.Generated || result.Skipped || result.MkcertMissing {
		t.Error("expected all result fields to be false on error")
	}
	if !errors.Is(err, errors.Unwrap(err)) {
		// Just verify the error message contains useful info
		if got := err.Error(); got == "" {
			t.Error("expected non-empty error message")
		}
	}
}
