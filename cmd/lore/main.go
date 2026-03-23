package main

import (
	"os"

	"github.com/GyroZepelix/loremaster/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
