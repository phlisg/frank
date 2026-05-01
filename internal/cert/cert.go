package cert

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Result reports the outcome of a Generate operation.
type Result struct {
	Generated     bool // newly generated
	Skipped       bool // already existed, skipped
	MkcertMissing bool // mkcert not installed
	CANotTrusted  bool // CA root not installed
}

// Generate produces localhost TLS certificates in frankDir/certs/.
// Idempotent — skips if certs already exist.
// Returns Result{MkcertMissing: true} (no error) if mkcert is not found.
func Generate(frankDir string) (Result, error) {
	return generate(frankDir, execRunner{}, osFS{})
}

// CertsExist checks whether frankDir/certs/ contains the expected cert files.
func CertsExist(frankDir string) bool {
	certFile := filepath.Join(frankDir, "certs", "localhost.pem")
	keyFile := filepath.Join(frankDir, "certs", "localhost-key.pem")
	_, err1 := os.Stat(certFile)
	_, err2 := os.Stat(keyFile)
	return err1 == nil && err2 == nil
}

// MkcertAvailable reports whether mkcert is on PATH.
func MkcertAvailable() bool {
	_, err := exec.LookPath("mkcert")
	return err == nil
}

// commandRunner abstracts command execution for testing.
type commandRunner interface {
	LookPath(name string) (string, error)
	Run(name string, args ...string) ([]byte, error)
}

// fileSystem abstracts filesystem operations for testing.
type fileSystem interface {
	Exists(path string) bool
	MkdirAll(path string, perm os.FileMode) error
}

// execRunner is the production commandRunner.
type execRunner struct{}

func (execRunner) LookPath(name string) (string, error) { return exec.LookPath(name) }
func (execRunner) Run(name string, args ...string) ([]byte, error) {
	return exec.Command(name, args...).CombinedOutput()
}

// osFS is the production fileSystem.
type osFS struct{}

func (osFS) Exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func (osFS) MkdirAll(path string, perm os.FileMode) error {
	return os.MkdirAll(path, perm)
}

func generate(frankDir string, runner commandRunner, fs fileSystem) (Result, error) {
	certDir := filepath.Join(frankDir, "certs")
	certFile := filepath.Join(certDir, "localhost.pem")
	keyFile := filepath.Join(certDir, "localhost-key.pem")

	// Already exist → skip
	if fs.Exists(certFile) && fs.Exists(keyFile) {
		return Result{Skipped: true}, nil
	}

	// Detect mkcert
	mkcertPath, err := runner.LookPath("mkcert")
	if err != nil {
		return Result{MkcertMissing: true}, nil
	}

	// CA trust check
	caNotTrusted := !caInstalled(runner, mkcertPath, fs)

	// Create directory
	if err := fs.MkdirAll(certDir, 0755); err != nil {
		return Result{}, fmt.Errorf("create cert dir: %w", err)
	}

	// Generate certs
	out, err := runner.Run(mkcertPath,
		"-cert-file", certFile,
		"-key-file", keyFile,
		"localhost", "127.0.0.1", "::1")
	if err != nil {
		return Result{}, fmt.Errorf("mkcert: %s: %w", strings.TrimSpace(string(out)), err)
	}

	return Result{Generated: true, CANotTrusted: caNotTrusted}, nil
}

func caInstalled(runner commandRunner, mkcertPath string, fs fileSystem) bool {
	out, err := runner.Run(mkcertPath, "-CAROOT")
	if err != nil {
		return false
	}
	caRoot := strings.TrimSpace(string(out))
	if caRoot == "" {
		return false
	}
	return fs.Exists(filepath.Join(caRoot, "rootCA.pem"))
}
