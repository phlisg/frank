package cmd

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

var exportPath string

func init() {
	exportCmd.Flags().StringVarP(&exportPath, "output", "o", "", "output path for exported docker-compose.yml (defaults to ./docker-compose.yml)")
	rootCmd.AddCommand(exportCmd)
}

var exportCmd = &cobra.Command{
	Use:   "export",
	Short: "Export Frank-generated files as a Sail-compatible docker-compose.yml",
	Long: `Copies the Frank-generated compose.yaml to docker-compose.yml (or a custom path).

Frank uses Sail-compatible service naming natively, so no conversion is needed.
This is a best-effort export — custom Dockerfile modifications are not preserved.`,
	SilenceUsage: true,
	RunE:         runExport,
}

func runExport(cmd *cobra.Command, args []string) error {
	dir := resolveDir()

	src := filepath.Join(dir, "compose.yaml")
	if _, err := os.Stat(src); os.IsNotExist(err) {
		return fmt.Errorf("compose.yaml not found — run frank generate first")
	}

	dst := exportPath
	if dst == "" {
		dst = filepath.Join(dir, "docker-compose.yml")
	}

	if err := copyFile(src, dst); err != nil {
		return fmt.Errorf("export failed: %w", err)
	}

	fmt.Printf("  exported  compose.yaml → %s\n", dst)
	return nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Close()
}
