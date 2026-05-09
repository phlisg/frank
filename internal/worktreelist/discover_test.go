package worktreelist

import (
	"os"
	"testing"
)

func TestParsePorcelain_Empty(t *testing.T) {
	got := parsePorcelain("")
	if got != nil {
		t.Fatalf("expected nil, got %v", got)
	}
}

func TestParsePorcelain_OnlyMain(t *testing.T) {
	raw := "worktree /home/user/project\nHEAD abc123\nbranch refs/heads/main\n\n"
	got := parsePorcelain(raw)
	if got != nil {
		t.Fatalf("expected nil (skip main), got %v", got)
	}
}

func TestParsePorcelain_MainPlus2Linked(t *testing.T) {
	raw := `worktree /home/user/project
HEAD abc123
branch refs/heads/main

worktree /home/user/project/.worktrees/feat-x
HEAD def456
branch refs/heads/feature/x

worktree /home/user/project/.worktrees/fix-y
HEAD 789abc
branch refs/heads/fix/y

`
	got := parsePorcelain(raw)
	if len(got) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(got))
	}
	if got[0].path != "/home/user/project/.worktrees/feat-x" {
		t.Errorf("entry[0].path = %q", got[0].path)
	}
	if got[0].branch != "feature/x" {
		t.Errorf("entry[0].branch = %q", got[0].branch)
	}
	if got[1].path != "/home/user/project/.worktrees/fix-y" {
		t.Errorf("entry[1].path = %q", got[1].path)
	}
	if got[1].branch != "fix/y" {
		t.Errorf("entry[1].branch = %q", got[1].branch)
	}
}

func TestParsePorcelain_DetachedHEAD(t *testing.T) {
	raw := `worktree /home/user/project
HEAD abc123
branch refs/heads/main

worktree /home/user/project/.worktrees/detached
HEAD deadbeef

`
	got := parsePorcelain(raw)
	if len(got) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(got))
	}
	if got[0].branch != "(detached)" {
		t.Errorf("expected (detached), got %q", got[0].branch)
	}
}

func TestSplitPorcelainBlocks(t *testing.T) {
	raw := "a\nb\n\nc\nd\n\ne\n"
	blocks := splitPorcelainBlocks(raw)
	if len(blocks) != 3 {
		t.Fatalf("expected 3 blocks, got %d: %v", len(blocks), blocks)
	}
	if blocks[0] != "a\nb" {
		t.Errorf("block[0] = %q", blocks[0])
	}
	if blocks[1] != "c\nd" {
		t.Errorf("block[1] = %q", blocks[1])
	}
	if blocks[2] != "e" {
		t.Errorf("block[2] = %q", blocks[2])
	}
}

func TestShortenHome(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("cannot determine home dir")
	}

	tests := []struct {
		name string
		path string
		want string
	}{
		{"prefix", home + "/code/project", "~/code/project"},
		{"no home", "/tmp/other", "/tmp/other"},
		{"exact home", home, "~"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := shortenHome(tt.path)
			if got != tt.want {
				t.Errorf("shortenHome(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}
