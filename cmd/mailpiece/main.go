package main

import (
	"runtime/debug"

	"github.com/mailpiece/mailpiece-cli/internal/commands"
)

var version = "dev"

func main() {
	if version == "dev" {
		if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" && info.Main.Version != "(devel)" {
			version = info.Main.Version
		}
	}

	commands.SetVersion(version)
	commands.Execute()
}
