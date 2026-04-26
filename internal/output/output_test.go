package output

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"
)

func captureStdout(fn func()) string {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	fn()

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	io.Copy(&buf, r)
	return buf.String()
}

func captureStderr(fn func()) string {
	old := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	fn()

	w.Close()
	os.Stderr = old

	var buf bytes.Buffer
	io.Copy(&buf, r)
	return buf.String()
}

func TestGroup_Normal(t *testing.T) {
	SetLevel(Normal)
	defer SetLevel(Normal)

	out := captureStdout(func() {
		Group("config", "frank.yaml")
	})
	if !strings.Contains(out, "✓") || !strings.Contains(out, "config (frank.yaml)") {
		t.Fatalf("expected group output, got: %q", out)
	}
}

func TestGroup_NoDetail(t *testing.T) {
	SetLevel(Normal)
	defer SetLevel(Normal)

	out := captureStdout(func() {
		Group("done", "")
	})
	if !strings.Contains(out, "✓") || !strings.Contains(out, "done") {
		t.Fatalf("expected group without detail, got: %q", out)
	}
}

func TestGroup_Quiet(t *testing.T) {
	SetLevel(Quiet)
	defer SetLevel(Normal)

	out := captureStdout(func() {
		Group("config", "frank.yaml")
	})
	if out != "" {
		t.Fatalf("expected no output in quiet mode, got: %q", out)
	}
}

func TestDetail_Verbose(t *testing.T) {
	SetLevel(Verbose)
	defer SetLevel(Normal)

	out := captureStdout(func() {
		Detail("writing file")
	})
	if !strings.Contains(out, "  writing file") {
		t.Fatalf("expected detail output, got: %q", out)
	}
}

func TestDetail_Normal(t *testing.T) {
	SetLevel(Normal)
	defer SetLevel(Normal)

	out := captureStdout(func() {
		Detail("writing file")
	})
	if out != "" {
		t.Fatalf("expected no output in normal mode, got: %q", out)
	}
}

func TestNextSteps_Empty(t *testing.T) {
	SetLevel(Normal)
	defer SetLevel(Normal)

	out := captureStdout(func() {
		NextSteps(nil)
	})
	if out != "" {
		t.Fatalf("expected no output for empty lines, got: %q", out)
	}
}

func TestNextSteps_Normal(t *testing.T) {
	SetLevel(Normal)
	defer SetLevel(Normal)

	out := captureStdout(func() {
		NextSteps([]string{"run frank up", "open browser"})
	})
	if !strings.Contains(out, "Next steps:") {
		t.Fatalf("expected header, got: %q", out)
	}
	if !strings.Contains(out, "  run frank up") {
		t.Fatalf("expected indented line, got: %q", out)
	}
}

func TestSpin_Success(t *testing.T) {
	SetLevel(Normal)
	defer SetLevel(Normal)

	out := captureStdout(func() {
		stop := Spin("Building image")
		stop(nil)
	})
	if !strings.Contains(out, "✓") || !strings.Contains(out, "Building image") {
		t.Fatalf("expected green tick + label, got: %q", out)
	}
}

func TestSpin_Error(t *testing.T) {
	SetLevel(Normal)
	defer SetLevel(Normal)

	out := captureStdout(func() {
		stop := Spin("Building image")
		stop(io.ErrUnexpectedEOF)
	})
	if !strings.Contains(out, "✗") || !strings.Contains(out, "Building image") {
		t.Fatalf("expected red cross + label, got: %q", out)
	}
}

func TestSpin_Quiet(t *testing.T) {
	SetLevel(Quiet)
	defer SetLevel(Normal)

	out := captureStdout(func() {
		stop := Spin("Building image")
		stop(nil)
	})
	if out != "" {
		t.Fatalf("expected no output in quiet mode, got: %q", out)
	}
}

func TestWarning_Always(t *testing.T) {
	for _, level := range []Level{Quiet, Normal, Verbose} {
		SetLevel(level)
		out := captureStderr(func() {
			Warning("something broke")
		})
		if !strings.Contains(out, "warning: something broke") {
			t.Fatalf("expected warning at level %d, got: %q", level, out)
		}
	}
	SetLevel(Normal)
}
