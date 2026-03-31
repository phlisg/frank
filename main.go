package main

import (
	"embed"

	"github.com/phlisg/frank/cmd"
)

//go:embed templates
var templateFS embed.FS

var version = "dev"

func main() {
	cmd.Execute(templateFS, version)
}
