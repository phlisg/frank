package cmd

import (
	"fmt"
	"strings"

	"github.com/phlisg/frank/internal/config"
	"github.com/phlisg/frank/internal/docker"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(exportCmd)
}

var exportCmd = &cobra.Command{
	Use:   "export",
	Short: "Install Laravel Sail into the running project containers",
	Long: `Delegates to Laravel Sail's own installer (sail:install) running inside
the laravel.test container. Sail will generate its own docker-compose.yml,
docker/ folder, and related files.

Requires containers to be running — run frank up first.`,
	SilenceUsage: true,
	RunE:         runExport,
}

func runExport(cmd *cobra.Command, args []string) error {
	dir := resolveDir()

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

	if err := client.Exec("laravel.test", "composer", "require", "laravel/sail", "--dev"); err != nil {
		return fmt.Errorf("composer require laravel/sail failed: %w", err)
	}

	if err := client.Exec("laravel.test", "php", "artisan", "sail:install",
		"--with="+withList,
		"--php="+cfg.PHP.Version,
	); err != nil {
		return fmt.Errorf("sail:install failed: %w", err)
	}

	fmt.Println("  export complete  run vendor/bin/sail up to start containers")
	return nil
}
