package main

import (
	"os"

	"weefleet/internal/app"
)

func main() {
	if app.IsCoordinatorInvocation(os.Args[0]) {
		os.Exit(app.RunCoordinator(os.Args[1:]))
	}
	os.Exit(app.Run(os.Args[1:]))
}
