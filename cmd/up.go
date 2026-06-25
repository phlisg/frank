package cmd

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/phlisg/frank/internal/baseimage"
	"github.com/phlisg/frank/internal/cert"
	"github.com/phlisg/frank/internal/config"
	"github.com/phlisg/frank/internal/docker"
	"github.com/phlisg/frank/internal/output"
	"github.com/phlisg/frank/internal/template"
	"github.com/phlisg/frank/internal/watch"
	"github.com/spf13/cobra"
	"golang.org/x/mod/semver"
)

var (
	upDetach bool
	upQuick  bool
)

func init() {
	upCmd.Flags().BoolVarP(&upDetach, "detach", "d", false, "Run containers in the background")
	upCmd.Flags().BoolVar(&upQuick, "quick", false, "Skip post-start tasks (composer install + artisan migrate)")
	upCmd.SetFlagErrorFunc(upFlagError)
	rootCmd.AddCommand(upCmd)
}

var upCmd = &cobra.Command{
	Use:   "up [-d] [--quick] [-- <compose args>]",
	Short: "Start containers (-d / --quick frank-owned; compose flags after --)",
	Long: `Start containers. Frank owns -d/--detach and --quick because it
needs those decisions for watcher spawn and post-start task skipping.
Every other docker compose flag must come after a literal "--".

Examples:
  frank up                              # foreground
  frank up -d                           # detached
  frank up --quick                      # skip post-start tasks
  frank up -- --build                   # force rebuild
  frank up -d -- --force-recreate       # detached + compose flag`,
	SilenceUsage:      true,
	ValidArgsFunction: cobra.NoFileCompletions,
	RunE:              runUp,
}

// upFlagError turns "unknown flag: --foo" into an actionable hint that
// points the user at the `--` separator for docker compose passthrough.
func upFlagError(cmd *cobra.Command, err error) error {
	return fmt.Errorf("%w\n\nHint: frank up only owns -d/--detach and --quick — pass docker compose flags after `--`\n  frank up -- --build", err)
}

func runUp(cmd *cobra.Command, args []string) error {
	dir := resolveDir()

	// `frank up` owns truncation of .frank/debug.log — fresh session.
	if err := output.OpenSessionLog(dir, rootCmd.Version, true); err != nil {
		output.Warning(fmt.Sprintf("could not open debug.log: %v", err))
	}
	defer output.CloseSessionLog()

	composeArgs := splitPassthrough(cmd, args)

	return doUp(dir, upDetach, upQuick, composeArgs, true)
}

func doUp(dir string, detach, quick bool, passthrough []string, showNextSteps bool) error {
	client := docker.New(dir)

	composeArgs := passthrough
	if detach {
		composeArgs = append([]string{"-d"}, composeArgs...)
	}

	// Pre-flight: auto-generate .frank/ if frank.yaml exists but .frank/ doesn't.
	if _, err := os.Stat(filepath.Join(dir, ".frank", "compose.yaml")); os.IsNotExist(err) {
		cfg, loadErr := config.Load(dir)
		if loadErr != nil {
			return fmt.Errorf("no Docker config found — run frank generate first")
		}
		output.Group("Generating Docker files", "frank.yaml found but .frank/ missing")
		if err := generate(cfg, dir, rootCmd.Version); err != nil {
			return fmt.Errorf("auto-generate failed: %w", err)
		}
		composeArgs = append(composeArgs, "--build")
	}

	// Pre-flight: detect stale .frank/ (version mismatch or config change) and auto-regenerate.
	if regenerated, needsBuild, err := autoRegenerate(dir, rootCmd.Version); err != nil {
		return err
	} else if regenerated {
		if needsBuild {
			composeArgs = append(composeArgs, "--build")
		}
		quick = false
	}

	// Pre-flight: build/refresh the shared frank/runtime base image. The thin
	// .frank/Dockerfile is `FROM frank/runtime:<tag>`, so compose would try to
	// pull that base from docker.io and fail without it. Placed AFTER
	// autoRegenerate because regen can change the base templates, requiring a
	// fresh base. Fatal — if the base can't build, compose will certainly fail.
	if err := ensureBaseImage(dir); err != nil {
		return err
	}

	// Pre-flight: generate APP_KEY before containers start so docker's
	// env_file picks it up at container creation time.
	if needsAppKey(dir) {
		if err := generateAppKey(dir); err != nil {
			output.Warning(fmt.Sprintf("could not generate APP_KEY: %v", err))
		}
	}

	// Resolve watcher intent once so fg + -d paths share the decision.
	cfg, _ := config.Load(dir)

	// HTTPS nudge: warn if HTTPS enabled but certs missing.
	if cfg != nil && cfg.Server.IsHTTPS() {
		frankDir := filepath.Join(dir, ".frank")
		if !cert.CertsExist(frankDir) {
			if cert.MkcertAvailable() {
				output.Warning("mkcert found but no certs generated. Run `frank generate && frank up --build` to enable HTTPS.")
			} else {
				output.Warning("HTTPS enabled but mkcert not found. Install mkcert and run `frank generate`, or set `server.https: false` in frank.yaml.\n  https://github.com/FiloSottile/mkcert#installation")
			}
		}
	}

	wantWatcher := shouldRunWatcher(cfg, client, dir)

	// Foreground mode: spawn watcher goroutine BEFORE compose so a .php
	// edit during container boot still lands a reload trigger once the
	// arm-suppression window clears. SIGINT/SIGTERM cancels both.
	var stopWatcher func() error
	if !detach && wantWatcher {
		var err error
		stopWatcher, err = startForegroundWatcher(dir, cfg)
		if err != nil {
			output.Warning(fmt.Sprintf("watcher not started: %v", err))
		}
	}

	region := output.Region("Starting containers")
	upErr := client.RunStream(region, append([]string{"up"}, composeArgs...)...)

	if stopWatcher != nil {
		if err := stopWatcher(); err != nil && !errors.Is(err, context.Canceled) {
			output.Warning(fmt.Sprintf("watcher stopped with error: %v", err))
		}
	}

	region.Stop(upErr)
	if upErr != nil {
		return upErr
	}

	if quick {
		return nil
	}

	stopWait := output.Spin("Waiting for laravel.test")
	if err := client.WaitForContainer("laravel.test", 30*time.Second); err != nil {
		stopWait(err)
		output.Warning(fmt.Sprintf("%v — skipping post-start tasks", err))
		return nil
	}
	stopWait(nil)

	if _, err := os.Stat(filepath.Join(dir, "composer.json")); err == nil {
		region := output.Region("Installing Composer dependencies")
		err := client.ExecStream(region, "laravel.test", "composer", "install", "--no-interaction")
		region.Stop(err)
		if err != nil {
			output.Warning(fmt.Sprintf("composer install failed: %v", err))
		}
	}

	if _, err := os.Stat(filepath.Join(dir, "artisan")); err == nil {
		region := output.Region("Running migrations")
		err := client.ExecStream(region, "laravel.test", "php", "artisan", "migrate", "--force")
		region.Stop(err)
		if err != nil {
			output.Warning(fmt.Sprintf("artisan migrate failed: %v", err))
		}
	}

	if config.IsWorktree(dir) {
		output.Group("Worktree mode", "ports are ephemeral — use `frank compose port <service> <port>` to find mapped ports")
	}

	if detach && showNextSteps {
		var steps []string
		pm := "npm"
		if cfg != nil && cfg.Node.PackageManager != "" {
			pm = cfg.Node.PackageManager
		}
		if _, err := os.Stat(filepath.Join(dir, "package.json")); err == nil {
			steps = append(steps, fmt.Sprintf("%s install && %s run dev", pm, pm))
		}
		output.NextSteps(steps)
	}

	if detach && wantWatcher {
		if err := spawnDetachedWatcher(dir); err != nil {
			output.Warning(fmt.Sprintf("could not start watcher: %v", err))
		}
	}

	return nil
}

// ensureBaseImage builds/refreshes the shared frank/runtime base for dir's
// config before any compose build. Graceful skip on config-load failure
// (mirrors autoRegenerate) so a missing/broken frank.yaml doesn't block — the
// normal flow surfaces the config error instead. When the image is already
// present and fresh this is an instant inspect+label compare, so it is safe to
// call before every up / compose-build / worker launch.
func ensureBaseImage(dir string) error {
	cfg, err := config.Load(dir)
	if err != nil {
		return nil // let the normal flow surface the config error
	}
	engine := template.New(TemplateFS)
	return baseimage.EnsureBase(engine, cfg)
}

// composeSubcmdBuilds reports whether a `frank compose` passthrough argv will
// trigger an image build (and therefore needs the shared base image present).
// It skips leading flags (anything starting with "-", plus the value of a
// space-separated flag like `-f file`) to find the first subcommand, then
// checks it against the build-capable set {build, up, run, create}. Read-only
// subcommands (ps, logs, down, …) return false.
func composeSubcmdBuilds(args []string) bool {
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			continue
		}
		if strings.HasPrefix(a, "-") {
			// `-f file` / `--file file` style: skip the value too when the
			// flag has no "=" and isn't a bundled short flag. We can't know
			// every compose flag's arity, but the build-detection only needs
			// to land on the first non-flag token; treating a following
			// non-flag token as the flag's value risks misclassifying. The
			// common pre-subcommand flag here is `-f <file>`, so honor that.
			if (a == "-f" || a == "--file" || a == "-p" || a == "--project-name" ||
				a == "--project-directory" || a == "--profile" || a == "--env-file") &&
				i+1 < len(args) {
				i++
			}
			continue
		}
		switch a {
		case "build", "up", "run", "create":
			return true
		default:
			return false
		}
	}
	return false
}

// autoRegenerate detects a stale .frank/ and regenerates it. Two tiers:
//
//   Tier 1 (should we regenerate at all?): stale if .state is missing/corrupt,
//   the frank version bumped, this is a "dev" build, or sha256(frank.yaml) no
//   longer matches the stored configHash. The hash check subsumes the old
//   explicit php.version/runtime comparison — those fields live in frank.yaml,
//   so any change to them flips the hash.
//
//   Tier 2 (does the image need a rebuild?): only when regenerating, and BEFORE
//   generate() overwrites the on-disk Dockerfile — see dockerfileChanged.
//
// Returns (regenerated, needsBuild, err). On a config-load failure it skips
// gracefully, returning (false, false, nil) so the normal up flow proceeds.
func autoRegenerate(dir, currentVersion string) (regenerated, needsBuild bool, err error) {
	stateFile := filepath.Join(dir, ".frank", ".state")
	stateData, readErr := os.ReadFile(stateFile)

	var state frankState

	stale := false
	reason := ""

	if readErr != nil {
		// No .state file — existing project upgrading to this frank version.
		stale = true
		reason = ".frank/.state missing"
	} else if jsonErr := json.Unmarshal(stateData, &state); jsonErr != nil {
		stale = true
		reason = ".frank/.state corrupt"
	} else {
		// Check frank version staleness.
		if currentVersion == "dev" {
			stale = true
			reason = "dev build"
		} else if state.FrankVersion == "" {
			stale = true
			reason = "frank version not stamped"
		} else if state.FrankVersion != "dev" {
			vc := "v" + currentVersion
			vs := "v" + state.FrankVersion
			if semver.IsValid(vc) && semver.IsValid(vs) && semver.Compare(vc, vs) > 0 {
				stale = true
				reason = fmt.Sprintf("frank updated %s → %s", state.FrankVersion, currentVersion)
			}
		}

		// Check frank.yaml config drift via content hash (subsumes the old
		// php.version/runtime comparison). Empty hash means frank.yaml is
		// unreadable — treat as not-drifted; the regen path below will surface
		// a real config error if there is one.
		if !stale {
			if h := frankConfigHash(dir); h != "" && h != state.ConfigHash {
				stale = true
				reason = "frank.yaml changed"
			}
		}
	}

	if !stale {
		return false, false, nil
	}

	// Need to regenerate — load config.
	cfg, cfgErr := config.Load(dir)
	if cfgErr != nil {
		// Config can't load — skip auto-regen, let normal flow handle it.
		output.Detail(fmt.Sprintf("skipping auto-regenerate (%s): %v", reason, cfgErr))
		return false, false, nil
	}

	// Decide rebuild BEFORE generate() rewrites the on-disk Dockerfile.
	needsBuild = dockerfileChanged(dir, cfg)

	stopGen := output.Spin(fmt.Sprintf("Regenerating .frank/ (%s)", reason))
	if genErr := generate(cfg, dir, currentVersion); genErr != nil {
		stopGen(genErr)
		return false, false, fmt.Errorf("auto-regenerate failed: %w", genErr)
	}
	stopGen(nil)

	return true, needsBuild, nil
}

// dockerfileChanged reports whether re-rendering the runtime's Dockerfile(s)
// from cfg would differ from the copies currently on disk in .frank/. It is the
// SINGLE source of truth for "did image inputs change": there is deliberately no
// hardcoded list of image-affecting config fields — the set is implicitly
// whatever the Dockerfile templates reference, so a future Dockerfile input
// triggers a rebuild automatically with nothing to remember. A missing on-disk
// file or any render error returns true (fail safe toward a rebuild, never
// toward a stale image).
//
// Only reachable after Tier 1 has already decided to regenerate; a missing
// Dockerfile alone does not trigger regeneration.
func dockerfileChanged(dir string, cfg *config.Config) bool {
	engine := template.New(TemplateFS)
	data := dockerfileData(cfg, config.ProjectName(dir))
	frankDir := filepath.Join(dir, ".frank")

	dockerfiles := []struct{ tmpl, file string }{{"Dockerfile.tmpl", "Dockerfile"}}
	if cfg.PHP.Runtime == "fpm" {
		dockerfiles = append(dockerfiles, struct{ tmpl, file string }{"nginx.Dockerfile.tmpl", "nginx.Dockerfile"})
	}

	for _, d := range dockerfiles {
		rendered, err := engine.RenderRuntime(cfg.PHP.Runtime, d.tmpl, data)
		if err != nil {
			return true
		}
		onDisk, err := os.ReadFile(filepath.Join(frankDir, d.file))
		if err != nil || string(onDisk) != rendered {
			return true
		}
	}
	return false
}

// needsAppKey returns true when .env exists but APP_KEY is empty.
func needsAppKey(dir string) bool {
	data, err := os.ReadFile(filepath.Join(dir, ".env"))
	if err != nil {
		return false
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.TrimSpace(line) == "APP_KEY=" {
			return true
		}
	}
	return false
}

// generateAppKey writes a random APP_KEY into .env before containers start,
// so docker's env_file directive picks it up at container creation time.
func generateAppKey(dir string) error {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return err
	}
	value := "base64:" + base64.StdEncoding.EncodeToString(key)

	path := filepath.Join(dir, ".env")
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	lines := strings.Split(string(data), "\n")
	for i, line := range lines {
		if strings.HasPrefix(line, "APP_KEY=") {
			lines[i] = "APP_KEY=" + value
			break
		}
	}
	updated := strings.Join(lines, "\n")
	output.Detail("generated APP_KEY")
	return os.WriteFile(path, []byte(updated), 0644)
}

// shouldRunWatcher decides whether `frank up` should spawn a watcher.
// Returns true when schedule is enabled, any queue pool is declared, or
// any ad-hoc worker is already running under this project. Degrades to
// false (no watcher) on load/docker errors — worker reload is
// nice-to-have, not a reason to block `frank up`.
func shouldRunWatcher(cfg *config.Config, client *docker.Client, projectRoot string) bool {
	if cfg != nil {
		if cfg.Workers.Schedule {
			return true
		}
		if totalQueueCount(cfg) > 0 {
			return true
		}
	}
	if client != nil {
		project := config.ProjectName(projectRoot)
		if names, err := client.AdhocWorkerNames(project); err == nil && len(names) > 0 {
			return true
		}
	}
	return false
}

// startForegroundWatcher spins up an in-process watcher goroutine that
// shares the parent process's lifecycle. Returns a stop function the
// caller invokes once the compose run returns (Ctrl-C or normal exit).
// Nil cfg is tolerated — callers warn separately on a missing frank.yaml.
func startForegroundWatcher(projectRoot string, cfg *config.Config) (func() error, error) {
	if cfg == nil {
		return nil, fmt.Errorf("frank.yaml missing — skipping watcher")
	}

	w, err := watch.New(watch.Config{
		ProjectRoot:       projectRoot,
		ScheduleEnabled:   cfg.Workers.Schedule,
		QueueCount:        totalQueueCount(cfg),
		DockerComposeFile: ".frank/compose.yaml",
		ArmSuppression:    watch.DefaultArmSuppression,
	})
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithCancel(context.Background())
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		select {
		case <-sigCh:
			cancel()
		case <-ctx.Done():
		}
	}()

	done := make(chan error, 1)
	go func() { done <- w.Start(ctx) }()

	output.Detail(fmt.Sprintf("frank watch: foreground (pid %d) armed", os.Getpid()))

	return func() error {
		signal.Stop(sigCh)
		cancel()
		err := <-done
		return err
	}, nil
}

// spawnDetachedWatcher forks the current binary as `frank watch` detached
// from the controlling terminal. Stdout/stderr land in .frank/watch.log.
// The spawned child writes its own pidfile via Start.
func spawnDetachedWatcher(projectRoot string) error {
	self, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve executable: %w", err)
	}
	argv := []string{self, "--dir", projectRoot, "watch"}
	pid, err := watch.Daemonize(argv, watch.LogfilePath(projectRoot))
	if err != nil {
		return err
	}
	output.Detail(fmt.Sprintf("frank watch: detached child (pid %d)", pid))
	return nil
}
