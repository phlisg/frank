package tool

import (
	"strings"
	"testing"
)

func TestPintJSON_Excludes(t *testing.T) {
	data, err := configFS.ReadFile("files/pint.json")
	if err != nil {
		t.Fatalf("failed to read pint.json: %v", err)
	}
	content := string(data)

	for _, want := range []string{".phpstorm.meta.php", "_ide_helper.php"} {
		if !strings.Contains(content, want) {
			t.Errorf("pint.json missing exclude %q", want)
		}
	}
}

func TestPhpstanNeon_Excludes(t *testing.T) {
	data, err := configFS.ReadFile("files/phpstan.neon")
	if err != nil {
		t.Fatalf("failed to read phpstan.neon: %v", err)
	}
	content := string(data)

	for _, want := range []string{".phpstorm.meta.php", "_ide_helper.php"} {
		if !strings.Contains(content, want) {
			t.Errorf("phpstan.neon missing exclude %q", want)
		}
	}

	if !strings.Contains(content, "routes/") {
		t.Error("phpstan.neon missing routes/ in paths")
	}
}

func TestRectorPHP_Paths(t *testing.T) {
	data, err := configFS.ReadFile("files/rector.php")
	if err != nil {
		t.Fatalf("failed to read rector.php: %v", err)
	}
	content := string(data)

	dirs := []string{"app", "bootstrap", "config", "public", "resources", "routes", "tests"}
	for _, dir := range dirs {
		if !strings.Contains(content, "'/"+dir) {
			t.Errorf("rector.php missing path %q", dir)
		}
	}
}
