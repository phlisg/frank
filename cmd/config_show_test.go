package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/phlisg/frank/internal/config"
)

func TestConfigShow_OutputContainsDefaults(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "test-project")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, config.ConfigFileName), []byte("version: 1\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// Override Dir so resolveDir() returns our temp dir.
	oldDir := Dir
	Dir = dir
	defer func() { Dir = oldDir }()

	var buf bytes.Buffer
	configShowCmd.SetOut(&buf)
	configShowCmd.SetErr(&buf)

	// Capture stdout since the command uses fmt.Print.
	r, w, _ := os.Pipe()
	oldStdout := os.Stdout
	os.Stdout = w

	err := configShowCmd.RunE(configShowCmd, nil)

	w.Close()
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("configShowCmd error: %v", err)
	}

	var out bytes.Buffer
	out.ReadFrom(r)
	output := out.String()

	// Resolved config should contain defaults.
	checks := []string{
		config.DefaultPHPVersion,
		config.DefaultPHPRuntime,
		config.DefaultPackageManager,
	}
	for _, want := range checks {
		if !strings.Contains(output, want) {
			t.Errorf("output missing default %q\noutput:\n%s", want, output)
		}
	}
}
