package cmd

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPatchComposerPHPVersion(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		phpVersion string
		want       string
	}{
		{
			name:       "patches default 8.3 to 8.5",
			phpVersion: "8.5",
			input:      `{"require":{"php":"^8.3","laravel/framework":"^13.0"}}`,
			want:       `{"require":{"php":"^8.5","laravel/framework":"^13.0"}}`,
		},
		{
			name:       "patches 8.2 to 8.4",
			phpVersion: "8.4",
			input:      `{"require":{"php":"^8.2","laravel/framework":"^12.0"}}`,
			want:       `{"require":{"php":"^8.4","laravel/framework":"^12.0"}}`,
		},
		{
			name:       "handles space after colon",
			phpVersion: "8.5",
			input:      `{"require":{"php": "^8.3","laravel/framework":"^13.0"}}`,
			want:       `{"require":{"php": "^8.5","laravel/framework":"^13.0"}}`,
		},
		{
			name:       "no-op when constraint already correct",
			phpVersion: "8.5",
			input:      `{"require":{"php":"^8.5","laravel/framework":"^13.0"}}`,
			want:       `{"require":{"php":"^8.5","laravel/framework":"^13.0"}}`,
		},
		{
			name:       "no-op when composer.json has no php key",
			phpVersion: "8.5",
			input:      `{"require":{"laravel/framework":"^13.0"}}`,
			want:       `{"require":{"laravel/framework":"^13.0"}}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "composer.json")
			if err := os.WriteFile(path, []byte(tt.input), 0644); err != nil {
				t.Fatalf("write composer.json: %v", err)
			}

			if err := patchComposerPHPVersion(dir, tt.phpVersion); err != nil {
				t.Fatalf("patchComposerPHPVersion: %v", err)
			}

			got, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read composer.json: %v", err)
			}
			if string(got) != tt.want {
				t.Errorf("got  %s\nwant %s", got, tt.want)
			}
		})
	}

	t.Run("no-op when composer.json missing", func(t *testing.T) {
		dir := t.TempDir()
		if err := patchComposerPHPVersion(dir, "8.5"); err != nil {
			t.Fatalf("expected no error for missing file, got: %v", err)
		}
	})
}
