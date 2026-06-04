package main

import (
	"fmt"
	"os"

	"github.com/zxzinn/neon-selfhost/cmd"
)

// Set via -ldflags at release time.
var (
	version = "dev"
	commit  = "none"
)

func main() {
	cmd.SetVersion(version, commit)
	if err := cmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
