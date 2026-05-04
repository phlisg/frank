package cmd

import (
	"fmt"
	"io"
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

	return installLaravel(dir, cfg, true)
}

func installLaravel(dir string, cfg *config.Config, regenerate bool) error {
	dir, err := filepath.Abs(dir)
	if err != nil {
		return fmt.Errorf("resolve absolute path: %w", err)
	}

	scriptBytes, err := fs.ReadFile(TemplateFS, "templates/scripts/laravel-init.sh")
	if err != nil {
		return fmt.Errorf("read laravel-init.sh: %w", err)
	}
	script := string(scriptBytes)

	laravelVersion := cfg.Laravel.Version
	if laravelVersion == "latest" || laravelVersion == "lts" {
		laravelVersion = ""
	}

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
	if output.GetLevel() == output.Verbose {
		c.Stdout = os.Stdout
		c.Stderr = os.Stderr
	} else {
		c.Stdout = io.Discard
		c.Stderr = io.Discard
	}

	if err := c.Run(); err != nil {
		return fmt.Errorf("laravel-init container: %w", err)
	}

	if err := patchComposerPHPVersion(dir, cfg.PHP.Version); err != nil {
		output.Warning(fmt.Sprintf("could not patch composer.json: %v", err))
	}

	if regenerate {
		output.Detail("regenerating Docker files")
		if err := generate(cfg, dir, rootCmd.Version); err != nil {
			return err
		}
	}

	if err := patchViteConfig(dir); err != nil {
		output.Warning(fmt.Sprintf("could not patch vite.config.js: %v", err))
	}

	if err := copyPsysh(dir); err != nil {
		output.Warning(fmt.Sprintf("could not copy .psysh.php: %v", err))
	}

	return nil
}

// composerRequireDev runs `composer require --dev` in a disposable container,
// updating both composer.json and composer.lock atomically.
func composerRequireDev(dir string, packages []string) error {
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return err
	}

	uid := os.Getuid()
	gid := os.Getgid()

	args := []string{
		"run", "--rm",
		"-u", fmt.Sprintf("%d:%d", uid, gid),
		"-v", absDir + ":/app",
		"-w", "/app",
		"composer:latest",
		"composer", "require", "--dev", "--no-interaction", "--ignore-platform-reqs",
	}
	args = append(args, packages...)

	output.Detail(fmt.Sprintf("composer require --dev %d packages", len(packages)))

	c := exec.Command("docker", args...)
	if output.GetLevel() == output.Verbose {
		c.Stdout = os.Stdout
		c.Stderr = os.Stderr
	} else {
		c.Stdout = io.Discard
		c.Stderr = io.Discard
	}

	return c.Run()
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
	if output.GetLevel() == output.Verbose {
		c.Stdout = os.Stdout
		c.Stderr = os.Stderr
	} else {
		c.Stdout = io.Discard
		c.Stderr = io.Discard
	}

	if err := c.Run(); err != nil {
		return fmt.Errorf("sail-install container: %w", err)
	}
	return nil
}

// patchComposerPHPVersion updates the "php" version constraint in the require block
// of composer.json to match the PHP version chosen during frank new.
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
// patchViteConfig inserts the frankServer import and server property into
// vite.config.js (or .ts). Only patches if the file exists and doesn't
// already reference vite-server. Covers Docker host binding, CORS, and HTTPS.
func patchViteConfig(dir string) error {
	var name string
	var data []byte
	for _, n := range []string{"vite.config.js", "vite.config.ts"} {
		d, err := os.ReadFile(filepath.Join(dir, n))
		if err == nil {
			name = n
			data = d
			break
		}
	}
	if name == "" {
		return nil
	}

	content := string(data)
	if strings.Contains(content, "vite-server") {
		return nil
	}

	lines := strings.Split(content, "\n")

	// Insert import after the last import line.
	lastImport := -1
	for i, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "import ") {
			lastImport = i
		}
	}
	if lastImport == -1 {
		return fmt.Errorf("no import lines found")
	}
	importLine := "import frankServer from './.frank/vite-server.js';"
	lines = append(lines[:lastImport+1], append([]string{importLine}, lines[lastImport+1:]...)...)

	// Insert server: frankServer before closing });
	closingIdx := -1
	for i := len(lines) - 1; i >= 0; i-- {
		if strings.TrimSpace(lines[i]) == "});" {
			closingIdx = i
			break
		}
	}
	if closingIdx == -1 {
		return fmt.Errorf("could not find closing });")
	}
	serverLine := "    server: frankServer,"
	lines = append(lines[:closingIdx], append([]string{serverLine}, lines[closingIdx:]...)...)

	if err := os.WriteFile(filepath.Join(dir, name), []byte(strings.Join(lines, "\n")), 0644); err != nil {
		return err
	}
	output.Detail("patched vite.config (Frank server)")
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
