package main

import (
	"os"

	"github.com/irevolve/bear/commands"
)

func main() {
	if err := commands.Execute(); err != nil {
		os.Exit(1)
	}
}
