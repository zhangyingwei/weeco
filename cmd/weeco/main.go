package main

import (
	"os"

	"weeco/internal/commands"
)

func main() {
	os.Exit(commands.Run(os.Args[1:]))
}
