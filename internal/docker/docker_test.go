package docker

import (
	"os/exec"
	"strings"
	"testing"
)

func TestNew(t *testing.T) {
	c := New("/some/project")
	if c.dir != "/some/project" {
		t.Errorf("dir = %q, want /some/project", c.dir)
	}
}

func TestComposeCmd_Args(t *testing.T) {
	c := New("/proj")
	cmd := c.composeCmd("up", "-d", "--build")

	if cmd.Path == "" {
		t.Fatal("cmd.Path is empty")
	}
	// Args[0] is the binary path; check the rest.
	args := cmd.Args[1:] // skip binary name
	want := []string{"compose", "--project-directory", ".", "-f", ".frank/compose.yaml", "up", "-d", "--build"}
	if len(args) != len(want) {
		t.Fatalf("args = %v, want %v", args, want)
	}
	for i, a := range args {
		if a != want[i] {
			t.Errorf("args[%d] = %q, want %q", i, a, want[i])
		}
	}
}

func TestComposeCmd_Dir(t *testing.T) {
	c := New("/my/project")
	cmd := c.composeCmd("ps")
	if cmd.Dir != "/my/project" {
		t.Errorf("cmd.Dir = %q, want /my/project", cmd.Dir)
	}
}

func TestRunCmd_Success(t *testing.T) {
	cmd := exec.Command("true")
	if err := runCmd(cmd); err != nil {
		t.Errorf("expected no error for 'true', got: %v", err)
	}
}

func TestRunCmd_Failure(t *testing.T) {
	cmd := exec.Command("false")
	err := runCmd(cmd)
	if err == nil {
		t.Error("expected error for 'false'")
	}
	if !strings.Contains(err.Error(), "code 1") {
		t.Errorf("expected exit code in error, got: %v", err)
	}
}

func TestUp_NoArgs(t *testing.T) {
	c := New("/proj")
	cmd := c.composeCmd(upArgs()...)
	args := cmd.Args[1:]
	want := []string{"compose", "--project-directory", ".", "-f", ".frank/compose.yaml", "up"}
	if len(args) != len(want) {
		t.Fatalf("args = %v, want %v", args, want)
	}
	for i, a := range args {
		if a != want[i] {
			t.Errorf("args[%d] = %q, want %q", i, a, want[i])
		}
	}
}

func TestUp_WithDetach(t *testing.T) {
	c := New("/proj")
	cmd := c.composeCmd(upArgs("-d")...)
	args := cmd.Args[1:]
	want := []string{"compose", "--project-directory", ".", "-f", ".frank/compose.yaml", "up", "-d"}
	if len(args) != len(want) {
		t.Fatalf("args = %v, want %v", args, want)
	}
	for i, a := range args {
		if a != want[i] {
			t.Errorf("args[%d] = %q, want %q", i, a, want[i])
		}
	}
}

// upArgs builds the args slice that Up() would pass to composeCmd.
func upArgs(extra ...string) []string {
	return append([]string{"up"}, extra...)
}

func TestContainerStatus_ParseRunning(t *testing.T) {
	// Simulate the JSON output from docker compose ps --format json
	cases := []struct {
		name        string
		output      string
		wantState   ContainerState
		wantRunning int
		wantTotal   int
	}{
		{
			name:        "all running",
			output:      `{"Name":"app","State":"running"}` + "\n" + `{"Name":"db","State":"running"}`,
			wantState:   StateRunning,
			wantRunning: 2,
			wantTotal:   2,
		},
		{
			name:        "partial",
			output:      `{"Name":"app","State":"running"}` + "\n" + `{"Name":"db","State":"exited"}`,
			wantState:   StatePartial,
			wantRunning: 1,
			wantTotal:   2,
		},
		{
			name:        "none running",
			output:      `{"Name":"app","State":"exited"}`,
			wantState:   StateStopped,
			wantRunning: 0,
			wantTotal:   0,
		},
		{
			name:        "empty output",
			output:      "",
			wantState:   StateStopped,
			wantRunning: 0,
			wantTotal:   0,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			state, running, total := parseContainerStatus(tc.output)
			if state != tc.wantState {
				t.Errorf("state = %v, want %v", state, tc.wantState)
			}
			if running != tc.wantRunning {
				t.Errorf("running = %d, want %d", running, tc.wantRunning)
			}
			if total != tc.wantTotal {
				t.Errorf("total = %d, want %d", total, tc.wantTotal)
			}
		})
	}
}
