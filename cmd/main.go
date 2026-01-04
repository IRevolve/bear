package main

import (
	"os"

	"github.com/IRevolve/Bear/cmd/commands"
)

func main() {
	if err := commands.Execute(); err != nil {
		os.Exit(1)
	}
}
