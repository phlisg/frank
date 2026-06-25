package cmd

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/phlisg/frank/internal/baseimage"
	"github.com/phlisg/frank/internal/compose"
	"github.com/phlisg/frank/internal/config"
	"github.com/phlisg/frank/internal/docker"
	"github.com/phlisg/frank/internal/output"
	"github.com/phlisg/frank/internal/template"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(ejectCmd)
}

var ejectCmd = &cobra.Command{
	Use:   "eject",
	Short: "Install Laravel Sail into the running project containers",
	Long: `Delegates to Laravel Sail's own installer (sail:install) running inside
the laravel.test container. Sail will generate its own docker-compose.yml,
docker/ folder, and related files.

Requires containers to be running — run frank up first.`,
	SilenceUsage: true,
	RunE:         runEject,
}

func runEject(cmd *cobra.Command, args []string) error {
	dir := resolveDir()
	defer openSessionAppend(dir)()

	cfg, err := config.Load(dir)
	if err != nil {
		return err
	}

	client := docker.New(dir)
	state, _, _ := client.ContainerStatus()
	if state != docker.StateRunning {
		return fmt.Errorf("containers are not running — run frank up first")
	}

	// Build --with list: map Frank services to Sail equivalents, dropping sqlite.
	var sailServices []string
	for _, svc := range cfg.Services {
		if svc == "sqlite" {
			continue
		}
		sailServices = append(sailServices, svc)
	}
	withList := strings.Join(sailServices, ",")

	reqRegion := output.Region("Installing Sail")
	if err := client.ExecStream(reqRegion, "laravel.test", "composer", "require", "laravel/sail", "--dev"); err != nil {
		reqRegion.Stop(err)
		return fmt.Errorf("composer require laravel/sail failed: %w", err)
	}
	reqRegion.Stop(nil)

	installRegion := output.Region("Configuring Sail")
	if err := client.ExecStream(installRegion, "laravel.test", "php", "artisan", "sail:install",
		"--with="+withList,
		"--php="+cfg.PHP.Version,
	); err != nil {
		installRegion.Stop(err)
		return fmt.Errorf("sail:install failed: %w", err)
	}
	installRegion.Stop(nil)

	// Restore phpunit.xml to Laravel defaults (sqlite/:memory:).
	if err := compose.RestorePHPUnitXML(dir); err != nil {
		output.Warning(fmt.Sprintf("could not restore phpunit.xml: %v", err))
	}

	// Flatten .frank/Dockerfile from the thin `FROM frank/runtime:<tag>` form
	// back to a self-contained Dockerfile. Sail can't rebuild FROM frank/runtime
	// (it has no notion of Frank's shared base), so an ejected project must be
	// fully Frank-independent. Non-fatal: the project still works against any
	// already-built image.
	if err := flattenDockerfile(dir, cfg); err != nil {
		output.Warning(fmt.Sprintf("could not flatten .frank/Dockerfile: %v", err))
	} else {
		output.Detail("flattened .frank/Dockerfile to self-contained form")
	}

	fmt.Println("  eject complete — run ./vendor/bin/sail up to start containers")
	return nil
}

// caddyfileBlock is the laravel.test-specific COPY layer the base template omits.
// In the pre-split monolithic Dockerfile it sat between the user-setup block and
// the entrypoint heredoc — flattening reinserts it there so the byte output
// matches that original single-file form.
const caddyfileBlock = "# Copy Caddyfile (generated at .frank/Caddyfile; build context is project root)\n" +
	"COPY .frank/Caddyfile /etc/caddy/Caddyfile\n\n"

// entrypointMarker is the comment line that opens the entrypoint heredoc section
// in both base templates. The Caddyfile block is reinserted immediately before it.
const entrypointMarker = "# Entrypoint (heredoc"

// flattenDockerfile rewrites .frank/Dockerfile from the thin
// `FROM frank/runtime:<tag>` form into a self-contained Dockerfile, so the
// ejected project no longer depends on Frank's shared base image.
//
// It renders the base Dockerfile (whose FROM is dunglas/frankenphp:… for
// frankenphp, ubuntu:24.04 for fpm) and — for frankenphp only — reinserts the
// app-specific Caddyfile COPY layer that the base template omits, restoring the
// original monolithic byte-for-byte layout.
func flattenDockerfile(dir string, cfg *config.Config) error {
	engine := template.New(TemplateFS)

	body, err := baseimage.Render(engine, cfg)
	if err != nil {
		return fmt.Errorf("render base Dockerfile: %w", err)
	}

	if cfg.PHP.Runtime == "frankenphp" {
		body = strings.Replace(body, entrypointMarker, caddyfileBlock+entrypointMarker, 1)
	}

	if err := writeFile(filepath.Join(dir, ".frank", "Dockerfile"), body); err != nil {
		return err
	}
	return nil
}
