package cmd

import (
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/phlisg/frank/internal/config"
	"github.com/phlisg/frank/internal/output"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(installCmd)
}

var installCmd = &cobra.Command{
	Use:   "install",
	Short: "Install a new Laravel project",
	Long: `Runs the Laravel installer inside a disposable composer container — no local PHP needed.

Steps:
  1. Spin up a composer:latest container with the project dir mounted
  2. Install Laravel into .temp-laravel/ then move files to project root
  3. Restore Frank's README.md and .gitignore
  4. Overwrite .env and .env.example with Frank-generated versions
  5. Patch vite.config.js for Docker HMR (server.host = '0.0.0.0')
  6. Copy .psysh.php if not already present`,
	SilenceUsage:      true,
	ValidArgsFunction: cobra.NoFileCompletions,
	RunE:              runInstall,
}

func runInstall(cmd *cobra.Command, args []string) error {
	dir, err := filepath.Abs(resolveDir())
	if err != nil {
		return err
	}

	cfg, err := config.Load(dir)
	if err != nil {
		return err
	}

	// Read the embedded laravel-init.sh script.
	scriptBytes, err := fs.ReadFile(TemplateFS, "templates/scripts/laravel-init.sh")
	if err != nil {
		return fmt.Errorf("read laravel-init.sh: %w", err)
	}
	script := string(scriptBytes)

	// Build the laravel version argument for the script ($1).
	// "latest" and "lts" are treated as unversioned (pass "" so composer picks latest stable).
	laravelVersion := cfg.Laravel.Version
	if laravelVersion == "latest" || laravelVersion == "lts" {
		laravelVersion = ""
	}

	// Run a disposable composer:latest container with the project dir mounted.
	// -i          : keep stdin open to pipe the script
	// --rm        : remove container when done
	// -u uid:gid  : run as current user to avoid root-owned files
	// -v dir:/app : mount project dir
	// -w /app     : set working dir inside container
	// sh -s -- <version> : sh reads script from stdin (-s), version becomes $1
	uid := os.Getuid()
	gid := os.Getgid()

	dockerArgs := []string{
		"run", "--rm", "-i",
		"-u", fmt.Sprintf("%d:%d", uid, gid),
		"-v", dir + ":/app",
		"-w", "/app",
		"composer:latest",
		"sh", "-s", "--", laravelVersion,
	}

	output.Detail("installing Laravel (this may take a moment on first run while composer:latest is pulled)")

	c := exec.Command("docker", dockerArgs...)
	c.Stdin = strings.NewReader(script)
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr

	if err := c.Run(); err != nil {
		return fmt.Errorf("laravel-init container: %w", err)
	}

	if err := patchComposerPHPVersion(dir, cfg.PHP.Version); err != nil {
		output.Warning(fmt.Sprintf("could not patch composer.json: %v", err))
	}

	output.Detail("regenerating Docker files")
	if err := generate(cfg, dir); err != nil {
		return err
	}

	if err := patchViteConfig(dir); err != nil {
		output.Warning(fmt.Sprintf("could not patch vite.config.js: %v", err))
	}

	if err := copyPsysh(dir); err != nil {
		output.Warning(fmt.Sprintf("could not copy .psysh.php: %v", err))
	}

	return nil
}

// runSailInstall runs composer require laravel/sail and php artisan sail:install
// inside a disposable composer:latest container. Running these commands via
// docker compose exec (inside a live container) causes inception problems that
// result in exit 137. sail:install only writes files so a disposable container
// is sufficient and avoids starting any Frank containers at all.
func runSailInstall(dir string, services []string, phpVersion string) error {
	uid := os.Getuid()
	gid := os.Getgid()

	withList := strings.Join(services, ",")

	script := `#!/bin/sh
set -e
# Laravel 12+ ships Sail in the skeleton; earlier versions do not.
# Check vendor presence rather than parsing version strings.
if [ ! -d vendor/laravel/sail ]; then
    # --ignore-platform-reqs: container PHP may differ from the project's target
    # (e.g. composer:latest ships 8.4 but the project declares ^8.5).
    # sail:install only writes files so the platform mismatch is harmless here.
    composer require laravel/sail --dev --no-interaction --ignore-platform-reqs
fi
php artisan sail:install --with="$1" --php="$2"
`

	dockerArgs := []string{
		"run", "--rm", "-i",
		"-u", fmt.Sprintf("%d:%d", uid, gid),
		"-v", dir + ":/app",
		"-w", "/app",
		"composer:latest",
		"sh", "-s", "--", withList, phpVersion,
	}

	output.Detail("installing Sail")

	c := exec.Command("docker", dockerArgs...)
	c.Stdin = strings.NewReader(script)
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr

	if err := c.Run(); err != nil {
		return fmt.Errorf("sail-install container: %w", err)
	}
	return nil
}

// patchComposerPHPVersion updates the "php" version constraint in the require block
// of composer.json to match the PHP version chosen during frank init.
//
// composer create-project always writes Laravel's own default constraint (e.g. ^8.2)
// regardless of which PHP version was selected. This patches the constraint so that
// composer install/update inside the Docker container doesn't complain about the
// mismatch between the container's PHP version and the declared requirement.
//
// The regex targets the "php" key's value string inside JSON.  It works on both
// minified and pretty-printed (multi-line, indented) composer.json because [^"]*
// matches any non-quote characters on the same line — no newlines appear inside a
// JSON string value.
var composerPHPRe = regexp.MustCompile(`("php":\s*")[^"]*(")`)

func patchComposerPHPVersion(dir, phpVersion string) error {
	path := filepath.Join(dir, "composer.json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	original := string(data)
	patched := composerPHPRe.ReplaceAllString(original, "${1}^"+phpVersion+"${2}")
	if patched == original {
		// Nothing changed — either php constraint already matches or key not found.
		return nil
	}

	if err := os.WriteFile(path, []byte(patched), 0644); err != nil {
		return err
	}
	output.Detail(fmt.Sprintf("patched composer.json (php constraint → ^%s)", phpVersion))
	return nil
}

// patchViteConfig patches vite.config.js for Docker HMR: binds to all interfaces,
// enables CORS, uses polling (inotify unreliable over volume mounts), and allows
// serving files from the project root.
func patchViteConfig(dir string) error {
	path := filepath.Join(dir, "vite.config.js")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	content := string(data)

	if strings.Contains(content, "'0.0.0.0'") {
		return nil
	}

	const dockerServer = `server: {
        host: '0.0.0.0',
        cors: true,
        hmr: { host: 'localhost' },
        watch: {
            usePolling: true,`

	var patched string
	if strings.Contains(content, "server:") {
		// Merge into existing server block instead of adding a duplicate key.
		patched = strings.Replace(content, "server: {\n        watch: {", dockerServer, 1)
		if patched == content {
			// Fallback: server block has different indentation/shape — inject at open brace.
			patched = strings.Replace(content, "server: {", "server: {\n        host: '0.0.0.0',\n        cors: true,", 1)
		}
	} else {
		patched = strings.Replace(
			content,
			"defineConfig({",
			"defineConfig({\n    server: { host: '0.0.0.0', cors: true, hmr: { host: 'localhost' }, watch: { usePolling: true }, fs: { allow: ['.'] } },",
			1,
		)
	}

	if patched == content {
		return nil
	}

	if err := os.WriteFile(path, []byte(patched), 0644); err != nil {
		return err
	}
	output.Detail("patched vite.config.js (Docker HMR)")
	return nil
}

// copyPsysh writes a default .psysh.php into dir for nicer Tinker sessions.
func copyPsysh(dir string) error {
	dst := filepath.Join(dir, ".psysh.php")
	if _, err := os.Stat(dst); err == nil {
		return nil
	}

	content, err := fs.ReadFile(TemplateFS, "templates/psysh.php")
	if err != nil {
		return err
	}

	if err := os.WriteFile(dst, content, 0644); err != nil {
		return err
	}
	output.Detail("wrote .psysh.php")
	return nil
}
