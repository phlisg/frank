package output

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

// ansiRe matches ANSI CSI escape sequences (colors, cursor moves). The raw
// container stream is full of them; the log file should be plain text.
// ponytail: per-write strip — an escape split across two Write calls could
// leak a fragment, but docker/composer emit them whole in practice.
var ansiRe = regexp.MustCompile("\x1b\\[[0-9;?]*[ -/]*[@-~]")

// Session log: a persistent, annotated transcript of a Frank command's output,
// written to .frank/debug.log. It complements the transient live Region (which
// only shows the last few lines) and — crucially — captures output from
// disposable --rm container runs (composer create-project, composer require,
// image build, sail:install) that never land in `docker compose logs`.
//
// Level-independent: the log is written even in Quiet mode. Do NOT gate any
// session-log write on the output level.
var (
	sessionMu  sync.Mutex
	sessionLog *os.File
)

const sessionTimeFmt = "2006-01-02 15:04:05"

// OpenSessionLog opens .frank/debug.log under dir. truncate=true (owned by
// `frank up`) resets the file; every other command appends. A header line with
// timestamp, full argv, and frank version is written on open. Best-effort:
// the .frank dir is created if missing.
func OpenSessionLog(dir, version string, truncate bool) error {
	sessionMu.Lock()
	defer sessionMu.Unlock()

	frankDir := filepath.Join(dir, ".frank")
	if err := os.MkdirAll(frankDir, 0755); err != nil {
		return err
	}

	flags := os.O_CREATE | os.O_WRONLY
	if truncate {
		flags |= os.O_TRUNC
	} else {
		flags |= os.O_APPEND
	}

	f, err := os.OpenFile(filepath.Join(frankDir, "debug.log"), flags, 0644)
	if err != nil {
		return err
	}

	sessionLog = f

	ts := time.Now().Format(sessionTimeFmt)
	fmt.Fprintf(f, "\n=== %s | %s | frank %s ===\n", ts, strings.Join(os.Args, " "), version)

	return nil
}

// CloseSessionLog closes the session log. Safe to call when none is open.
func CloseSessionLog() {
	sessionMu.Lock()
	defer sessionMu.Unlock()

	if sessionLog != nil {
		_ = sessionLog.Close()
		sessionLog = nil
	}
}

// logWrite tees raw output bytes into the session log. No-op when closed.
// Thread-safe: regions write from both stdout and stderr goroutines.
func logWrite(p []byte) {
	sessionMu.Lock()
	defer sessionMu.Unlock()

	if sessionLog != nil {
		_, _ = sessionLog.Write(collapseCR(ansiRe.ReplaceAll(p, nil)))
	}
}

// collapseCR flattens carriage-return progress redraws (composer/npm download
// bars: "45%\r50%\r100%\n") down to the final frame per line, while preserving
// a trailing \r as a plain CRLF line ending. Operates per write buffer.
// ponytail: a redraw split across two Write calls leaves one stray intermediate
// frame — bounded and rare; not worth cross-call buffering.
func collapseCR(b []byte) []byte {
	if bytes.IndexByte(b, '\r') < 0 {
		return b
	}

	var out []byte

	for {
		i := bytes.IndexByte(b, '\n')
		seg := b

		if i >= 0 {
			seg = b[:i]
		}

		seg = bytes.TrimSuffix(seg, []byte{'\r'}) // CRLF / trailing return
		if j := bytes.LastIndexByte(seg, '\r'); j >= 0 {
			seg = seg[j+1:] // keep only the last redraw frame
		}

		out = append(out, seg...)

		if i < 0 {
			break
		}

		out = append(out, '\n')
		b = b[i+1:]
	}

	return out
}

// logLine writes a single annotated line (step marker, group tick, warning)
// into the session log. No-op when closed.
func logLine(format string, args ...any) {
	sessionMu.Lock()
	defer sessionMu.Unlock()

	if sessionLog != nil {
		fmt.Fprintf(sessionLog, format+"\n", args...)
	}
}

// stepHeader renders a region-start marker: "-- label ----- <timestamp>".
func stepHeader(label string) string {
	const width = 52

	prefix := "-- " + label + " "
	if pad := width - len(prefix); pad > 0 {
		prefix += strings.Repeat("-", pad)
	}

	return prefix + " " + time.Now().Format(sessionTimeFmt)
}
