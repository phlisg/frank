package main

import (
	"embed"
	"runtime/debug"

	"github.com/phlisg/frank/cmd"
)

//go:embed templates
var templateFS embed.FS

func main() {
	version := "dev"
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" && info.Main.Version != "(devel)" {
		version = info.Main.Version
	}
	cmd.Execute(templateFS, version)
}
