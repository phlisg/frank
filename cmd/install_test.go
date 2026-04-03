package cmd

import (
	"os"
	"path/filepath"
	"testing"
)

// laravelComposerJSON is a representative Laravel 12 composer.json as produced
// by `composer create-project laravel/laravel` — pretty-printed, with typical
// keys that appear around the "php" constraint.  This matches the real-world
// format (NOT minified) that frank install encounters.
const laravelComposerJSON = `{
    "name": "laravel/laravel",
    "type": "project",
    "description": "The skeleton application for the Laravel framework.",
    "keywords": ["laravel", "framework"],
    "license": "MIT",
    "require": {
        "php": "^8.2",
        "laravel/framework": "^12.0",
        "laravel/tinker": "^2.10.1"
    },
    "require-dev": {
        "fakerphp/faker": "^1.23",
        "laravel/pint": "^1.13",
        "laravel/sail": "^1.26",
        "mockery/mockery": "^1.6",
        "nunomaduro/collision": "^8.1",
        "phpunit/phpunit": "^11.5.3",
        "spatie/laravel-ignition": "^2.4"
    },
    "autoload": {
        "psr-4": {
            "App\\": "app/",
            "Database\\Factories\\": "database/factories/",
            "Database\\Seeders\\": "database/seeders/"
        }
    },
    "autoload-dev": {
        "psr-4": {
            "Tests\\": "tests/"
        }
    },
    "scripts": {
        "post-autoload-dump": [
            "Illuminate\\Foundation\\ComposerScripts::postAutoloadDump",
            "@php artisan package:discover --ansi"
        ],
        "post-update-cmd": [
            "@php artisan vendor:publish --tag=laravel-assets --ansi --force"
        ],
        "post-root-package-install": [
            "@php -r \"file_exists('.env') || copy('.env.example', '.env');\""
        ],
        "post-create-project-cmd": [
            "@php artisan key:generate --ansi",
            "@php -r \"file_exists('database/database.sqlite') || touch('database/database.sqlite');\"",
            "@php artisan migrate --graceful --ansi"
        ]
    },
    "extra": {
        "laravel": {
            "dont-discover": []
        }
    },
    "config": {
        "optimize-autoloader": true,
        "preferred-install": "dist",
        "sort-packages": true,
        "allow-plugins": {
            "pestphp/pest-plugin": true,
            "php-http/discovery": true
        }
    },
    "minimum-stability": "stable",
    "prefer-stable": true
}
`

// laravelComposerJSONPatched is laravelComposerJSON with the php constraint
// updated to ^8.4 — the expected output after patchComposerPHPVersion("8.4").
const laravelComposerJSONPatched = `{
    "name": "laravel/laravel",
    "type": "project",
    "description": "The skeleton application for the Laravel framework.",
    "keywords": ["laravel", "framework"],
    "license": "MIT",
    "require": {
        "php": "^8.4",
        "laravel/framework": "^12.0",
        "laravel/tinker": "^2.10.1"
    },
    "require-dev": {
        "fakerphp/faker": "^1.23",
        "laravel/pint": "^1.13",
        "laravel/sail": "^1.26",
        "mockery/mockery": "^1.6",
        "nunomaduro/collision": "^8.1",
        "phpunit/phpunit": "^11.5.3",
        "spatie/laravel-ignition": "^2.4"
    },
    "autoload": {
        "psr-4": {
            "App\\": "app/",
            "Database\\Factories\\": "database/factories/",
            "Database\\Seeders\\": "database/seeders/"
        }
    },
    "autoload-dev": {
        "psr-4": {
            "Tests\\": "tests/"
        }
    },
    "scripts": {
        "post-autoload-dump": [
            "Illuminate\\Foundation\\ComposerScripts::postAutoloadDump",
            "@php artisan package:discover --ansi"
        ],
        "post-update-cmd": [
            "@php artisan vendor:publish --tag=laravel-assets --ansi --force"
        ],
        "post-root-package-install": [
            "@php -r \"file_exists('.env') || copy('.env.example', '.env');\""
        ],
        "post-create-project-cmd": [
            "@php artisan key:generate --ansi",
            "@php -r \"file_exists('database/database.sqlite') || touch('database/database.sqlite');\"",
            "@php artisan migrate --graceful --ansi"
        ]
    },
    "extra": {
        "laravel": {
            "dont-discover": []
        }
    },
    "config": {
        "optimize-autoloader": true,
        "preferred-install": "dist",
        "sort-packages": true,
        "allow-plugins": {
            "pestphp/pest-plugin": true,
            "php-http/discovery": true
        }
    },
    "minimum-stability": "stable",
    "prefer-stable": true
}
`

func TestPatchComposerPHPVersion(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		phpVersion string
		want       string
	}{
		{
			// Real-world case: pretty-printed Laravel 12 composer.json (8.2 → 8.4).
			// This is the primary regression case: the fix must work on multi-line,
			// indented JSON produced by composer create-project, not just minified JSON.
			name:       "patches pretty-printed Laravel 12 composer.json from 8.2 to 8.4",
			phpVersion: "8.4",
			input:      laravelComposerJSON,
			want:       laravelComposerJSONPatched,
		},
		{
			// pretty-printed JSON with 8.3 constraint being upgraded to 8.5.
			name:       "patches pretty-printed json from 8.3 to 8.5",
			phpVersion: "8.5",
			input: `{
    "require": {
        "php": "^8.3",
        "laravel/framework": "^13.0"
    }
}
`,
			want: `{
    "require": {
        "php": "^8.5",
        "laravel/framework": "^13.0"
    }
}
`,
		},
		{
			// Minified JSON — kept for regression coverage.
			name:       "patches minified json from 8.3 to 8.5",
			phpVersion: "8.5",
			input:      `{"require":{"php":"^8.3","laravel/framework":"^13.0"}}`,
			want:       `{"require":{"php":"^8.5","laravel/framework":"^13.0"}}`,
		},
		{
			// Minified JSON — kept for regression coverage.
			name:       "patches minified json from 8.2 to 8.4",
			phpVersion: "8.4",
			input:      `{"require":{"php":"^8.2","laravel/framework":"^12.0"}}`,
			want:       `{"require":{"php":"^8.4","laravel/framework":"^12.0"}}`,
		},
		{
			// Space after the colon (valid JSON formatting variant).
			name:       "handles space after colon (minified with space)",
			phpVersion: "8.5",
			input:      `{"require":{"php": "^8.3","laravel/framework":"^13.0"}}`,
			want:       `{"require":{"php": "^8.5","laravel/framework":"^13.0"}}`,
		},
		{
			// No-op: constraint already matches the selected version.
			name:       "no-op when constraint already correct",
			phpVersion: "8.4",
			input:      `{"require":{"php":"^8.4","laravel/framework":"^12.0"}}`,
			want:       `{"require":{"php":"^8.4","laravel/framework":"^12.0"}}`,
		},
		{
			// No-op: no php key in require.
			name:       "no-op when composer.json has no php key",
			phpVersion: "8.5",
			input:      `{"require":{"laravel/framework":"^13.0"}}`,
			want:       `{"require":{"laravel/framework":"^13.0"}}`,
		},
		{
			// Keys like "phpunit/phpunit" must NOT be matched by the regex.
			name:       "does not match phpunit or other php-prefixed package names",
			phpVersion: "8.4",
			input: `{
    "require": {
        "php": "^8.2",
        "laravel/framework": "^12.0"
    },
    "require-dev": {
        "phpunit/phpunit": "^11.5.3",
        "fakerphp/faker": "^1.23"
    }
}
`,
			want: `{
    "require": {
        "php": "^8.4",
        "laravel/framework": "^12.0"
    },
    "require-dev": {
        "phpunit/phpunit": "^11.5.3",
        "fakerphp/faker": "^1.23"
    }
}
`,
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
				t.Errorf("result mismatch:\n got:  %s\nwant: %s", got, tt.want)
			}
		})
	}

	t.Run("no-op when composer.json missing", func(t *testing.T) {
		dir := t.TempDir()
		if err := patchComposerPHPVersion(dir, "8.4"); err != nil {
			t.Fatalf("expected no error for missing file, got: %v", err)
		}
	})
}
