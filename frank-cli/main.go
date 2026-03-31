package main

import (
	"embed"

	"github.com/phlisg/frank-cli/cmd"
)

//go:embed templates
var templateFS embed.FS

func main() {
	cmd.Execute(templateFS)
}
