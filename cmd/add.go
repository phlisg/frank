package cmd

import (
	"fmt"
	"path/filepath"

	"github.com/phlisg/frank/internal/config"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(addCmd)
}

var addCmd = &cobra.Command{
	Use:          "add <service>",
	Short:        "Add a service to frank.yaml and regenerate",
	Args:         cobra.ExactArgs(1),
	SilenceUsage: true,
	ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		if len(args) == 0 {
			return config.AllServices(), cobra.ShellCompDirectiveNoFileComp
		}
		return nil, cobra.ShellCompDirectiveNoFileComp
	},
	RunE: runAdd,
}

func runAdd(cmd *cobra.Command, args []string) error {
	service := args[0]
	dir := resolveDir()

	if !config.ValidService(service) {
		return fmt.Errorf("unsupported service %q — valid options: pgsql, mysql, mariadb, sqlite, redis, memcached, meilisearch, mailpit", service)
	}

	cfg, err := config.Load(dir)
	if err != nil {
		return err
	}

	if cfg.HasService(service) {
		return fmt.Errorf("service %q is already configured", service)
	}

	if config.IsDatabase(service) && cfg.Database() != "" {
		return fmt.Errorf("cannot add %q: %q is already configured (only one database allowed)", service, cfg.Database())
	}

	cfg.Services = append(cfg.Services, service)

	if err := saveConfig(cfg, dir); err != nil {
		return err
	}

	fmt.Printf("  added    %s\n", service)

	fmt.Println("\nRegenerating Docker files...")

	return generate(cfg, dir, rootCmd.Version)
}

func saveConfig(cfg *config.Config, dir string) error {
	content, err := marshalConfig(cfg)
	if err != nil {
		return err
	}

	return writeFile(filepath.Join(dir, config.ConfigFileName), content)
}
