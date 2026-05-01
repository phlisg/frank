package main

import (
	"embed"
	"runtime/debug"

	"github.com/phlisg/frank/cmd"
)

//go:embed all:templates
var templateFS embed.FS

var version = "dev"

func main() {
	if version == "dev" {
		if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" && info.Main.Version != "(devel)" {
			version = info.Main.Version
		}
	}
	cmd.Execute(templateFS, version)
}
