package main

import (
	"os"

	"github.com/dgjalic/loremaster/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
