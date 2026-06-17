package output

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestSessionLog_TruncateVsAppend verifies the truncate-ownership rule:
// `frank up` (truncate=true) resets the file; every other command appends.
// Also checks the tee (raw region stream lands in the log) and step markers.
func TestSessionLog_TruncateVsAppend(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, ".frank", "debug.log")

	// First session: append-mode (e.g. `frank down`) writes a region stream.
	if err := OpenSessionLog(dir, "test", false); err != nil {
		t.Fatalf("open append: %v", err)
	}
	r := Region("Doing thing")
	r.Write([]byte("\x1b[37;44m INFO \x1b[39;49m line from disposable container\n"))
	r.Stop(nil)
	CloseSessionLog()

	first := readFile(t, logPath)
	if !strings.Contains(first, "line from disposable container") {
		t.Errorf("tee missing raw stream; got:\n%s", first)
	}
	if strings.Contains(first, "\x1b[") {
		t.Errorf("ANSI escapes not stripped from log; got:\n%q", first)
	}

	// \r progress redraws collapse to the final frame; CRLF text survives.
	if got := string(collapseCR([]byte("Downloading 45%\rDownloading 50%\rDownloading 100%\n"))); got != "Downloading 100%\n" {
		t.Errorf("collapseCR redraw: got %q", got)
	}
	if got := string(collapseCR([]byte("plain text\r\n"))); got != "plain text\n" {
		t.Errorf("collapseCR CRLF: got %q", got)
	}
	if got := string(collapseCR([]byte("no cr here"))); got != "no cr here" {
		t.Errorf("collapseCR passthrough: got %q", got)
	}
	if !strings.Contains(first, "-- Doing thing ") {
		t.Errorf("missing step header; got:\n%s", first)
	}
	if !strings.Contains(first, "OK Doing thing") {
		t.Errorf("missing OK marker; got:\n%s", first)
	}

	// Second append session must NOT wipe the first.
	if err := OpenSessionLog(dir, "test", false); err != nil {
		t.Fatalf("open append 2: %v", err)
	}
	logLine("second session")
	CloseSessionLog()
	if got := readFile(t, logPath); !strings.Contains(got, "line from disposable container") || !strings.Contains(got, "second session") {
		t.Errorf("append wiped prior session; got:\n%s", got)
	}

	// `frank up` truncate=true wipes and starts fresh.
	if err := OpenSessionLog(dir, "test", true); err != nil {
		t.Fatalf("open truncate: %v", err)
	}
	CloseSessionLog()
	if got := readFile(t, logPath); strings.Contains(got, "line from disposable container") {
		t.Errorf("truncate did not wipe prior content; got:\n%s", got)
	}
}

// TestSessionLog_LevelIndependent: the log is written even in Quiet mode.
func TestSessionLog_LevelIndependent(t *testing.T) {
	defer SetLevel(Normal)
	SetLevel(Quiet)

	dir := t.TempDir()
	if err := OpenSessionLog(dir, "test", true); err != nil {
		t.Fatalf("open: %v", err)
	}
	Group("Quiet group", "")
	Warning("quiet warning")
	CloseSessionLog()

	got := readFile(t, filepath.Join(dir, ".frank", "debug.log"))
	if !strings.Contains(got, "OK Quiet group") || !strings.Contains(got, "WARN quiet warning") {
		t.Errorf("Quiet mode dropped log writes; got:\n%s", got)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(b)
}
