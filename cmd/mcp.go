package cmd

import (
	"fmt"
	"os"

	"github.com/phlisg/frank/internal/config"
	"github.com/phlisg/frank/internal/docker"
	mcpserver "github.com/phlisg/frank/internal/mcp"
	"github.com/spf13/cobra"
)

func init() {
	mcpCmd.Hidden = true
	rootCmd.AddCommand(mcpCmd)
}

var mcpCmd = &cobra.Command{
	Use:          "mcp",
	Short:        "Start MCP server (stdio transport)",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		dir := resolveDir()
		cfg, err := config.Load(dir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "frank mcp: %v\n", err)
			os.Exit(1)
		}
		client := docker.New(dir)
		return mcpserver.Serve(client, cfg, rootCmd.Version, dir)
	},
}
