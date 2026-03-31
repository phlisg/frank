package cmd

import (
	"fmt"

	"github.com/phlisg/frank-cli/internal/config"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(removeCmd)
}

var removeCmd = &cobra.Command{
	Use:          "remove <service>",
	Short:        "Remove a service from frank.yaml and regenerate",
	Args:         cobra.ExactArgs(1),
	SilenceUsage: true,
	RunE:         runRemove,
}

func runRemove(cmd *cobra.Command, args []string) error {
	service := args[0]
	dir := resolveDir()

	cfg, err := config.Load(dir)
	if err != nil {
		return err
	}

	if !cfg.HasService(service) {
		return fmt.Errorf("service %q is not configured", service)
	}

	filtered := cfg.Services[:0]
	for _, svc := range cfg.Services {
		if svc != service {
			filtered = append(filtered, svc)
		}
	}
	cfg.Services = filtered

	// Drop any per-service config entry too.
	delete(cfg.Config, service)

	if err := saveConfig(cfg, dir); err != nil {
		return err
	}
	fmt.Printf("  removed  %s\n", service)

	fmt.Println("\nRegenerating Docker files...")
	return generate(cfg, dir)
}
