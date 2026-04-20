package tool

import (
	"testing"
)

func TestLookup(t *testing.T) {
	t.Run("found", func(t *testing.T) {
		tool, ok := Lookup("pint")
		if !ok {
			t.Fatal("expected pint to be found")
		}
		if tool.Name != "pint" {
			t.Fatalf("expected Name=pint, got %q", tool.Name)
		}
		if tool.Category != "php" {
			t.Fatalf("expected Category=php, got %q", tool.Category)
		}
	})

	t.Run("not found", func(t *testing.T) {
		_, ok := Lookup("nonexistent")
		if ok {
			t.Fatal("expected nonexistent tool to not be found")
		}
	})
}

func TestValid(t *testing.T) {
	valid := []string{"pint", "larastan", "rector", "lefthook"}
	for _, name := range valid {
		if !Valid(name) {
			t.Errorf("expected %q to be valid", name)
		}
	}

	if Valid("bogus") {
		t.Error("expected 'bogus' to be invalid")
	}
}

func TestAllNames(t *testing.T) {
	names := AllNames()

	if len(names) != 4 {
		t.Fatalf("expected 4 names, got %d: %v", len(names), names)
	}

	// Must be sorted
	for i := 1; i < len(names); i++ {
		if names[i] < names[i-1] {
			t.Fatalf("names not sorted: %v", names)
		}
	}

	// Check all expected names present
	expected := map[string]bool{"larastan": true, "lefthook": true, "pint": true, "rector": true}
	for _, n := range names {
		if !expected[n] {
			t.Errorf("unexpected name %q", n)
		}
	}
}

func TestAllTools(t *testing.T) {
	tools := AllTools()
	if len(tools) != 4 {
		t.Fatalf("expected 4 tools, got %d", len(tools))
	}

	// Verify it's a copy (mutating shouldn't affect registry)
	tools[0].Name = "mutated"
	orig, _ := Lookup("pint")
	if orig.Name != "pint" {
		t.Fatal("AllTools did not return a copy")
	}
}

func TestPHPTools(t *testing.T) {
	t.Run("filters to php category", func(t *testing.T) {
		tools := PHPTools([]string{"pint", "larastan", "lefthook"})
		if len(tools) != 2 {
			t.Fatalf("expected 2 PHP tools, got %d", len(tools))
		}
		for _, tool := range tools {
			if tool.Category != "php" {
				t.Errorf("expected php category, got %q for %s", tool.Category, tool.Name)
			}
		}
	})

	t.Run("empty selection", func(t *testing.T) {
		tools := PHPTools([]string{})
		if len(tools) != 0 {
			t.Fatalf("expected 0 tools, got %d", len(tools))
		}
	})

	t.Run("excludes project category", func(t *testing.T) {
		tools := PHPTools([]string{"lefthook"})
		if len(tools) != 0 {
			t.Fatalf("expected 0 tools (lefthook is project category), got %d", len(tools))
		}
	})
}

func TestValidateNames(t *testing.T) {
	t.Run("valid list passes", func(t *testing.T) {
		err := ValidateNames([]string{"pint", "rector", "lefthook"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("unknown name errors", func(t *testing.T) {
		err := ValidateNames([]string{"pint", "unknown"})
		if err == nil {
			t.Fatal("expected error for unknown tool")
		}
	})

	t.Run("empty list passes", func(t *testing.T) {
		err := ValidateNames([]string{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}
