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

	fmt.Println("Installing Laravel (this may take a moment on first run while composer:latest is pulled)...")

	c := exec.Command("docker", dockerArgs...)
	c.Stdin = strings.NewReader(script)
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr

	if err := c.Run(); err != nil {
		return fmt.Errorf("laravel-init container: %w", err)
	}

	// Patch composer.json to use the PHP version the user selected.
	// composer create-project always writes Laravel's own default (e.g. ^8.3).
	if err := patchComposerPHPVersion(dir, cfg.PHP.Version); err != nil {
		fmt.Fprintf(os.Stderr, "warning: could not patch composer.json: %v\n", err)
	}

	// Regenerate Docker files so .env/.env.example reflect Frank's service config.
	fmt.Println("Regenerating Docker files...")
	if err := generate(cfg, dir); err != nil {
		return err
	}

	// Patch vite.config.js for Docker HMR.
	if err := patchViteConfig(dir); err != nil {
		fmt.Fprintf(os.Stderr, "warning: could not patch vite.config.js: %v\n", err)
	}

	// Copy .psysh.php from embedded templates if not present.
	if err := copyPsysh(dir); err != nil {
		fmt.Fprintf(os.Stderr, "warning: could not copy .psysh.php: %v\n", err)
	}

	fmt.Println("Laravel installed successfully.")
	fmt.Println("Run 'frank up -d' to start your project.")
	return nil
}

// patchComposerPHPVersion updates the "php" constraint in composer.json to match
// the PHP version selected during frank init. composer create-project always
// writes Laravel's own default (e.g. ^8.3) regardless of the host PHP version.
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

	patched := composerPHPRe.ReplaceAllString(string(data), `${1}^`+phpVersion+`${2}`)
	if patched == string(data) {
		return nil
	}

	if err := os.WriteFile(path, []byte(patched), 0644); err != nil {
		return err
	}
	fmt.Println("  patched  composer.json (php constraint)")
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
	fmt.Println("  patched  vite.config.js (Docker HMR)")
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
	fmt.Println("  wrote  .psysh.php")
	return nil
}
