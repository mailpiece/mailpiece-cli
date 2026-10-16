package main

import (
	"runtime/debug"

	"github.com/mailpiece/mailpiece-cli/internal/commands"
)

// Stamped by ldflags at release; a build from source stays "dev".
var (
	version = "dev"
	commit  = ""
	date    = ""
)

func main() {
	if version == "dev" {
		if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" && info.Main.Version != "(devel)" {
			version = info.Main.Version
		}
	}

	commands.SetVersion(version)
	commands.SetBuildInfo(commit, date)
	commands.Execute()
}
