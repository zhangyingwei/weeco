package commands

import (
	"fmt"
	"os"
)

// Version is overridden by the build.
var Version = "0.12.0"

const usageText = `weeco: a Skill and real Herdr agents for one team

usage: weeco <command> [flags]

commands:
  start    recruit a Herdr member from the current lead pane
  status   inspect members or wait for new reports
  send     send work, decisions or progress
  report   submit this member's result or blocker
  accept   record that the lead accepted a submitted turn
  close    close this run's member panes
  config   write or check the model configuration
  install  install the binary and the weeco Skill
  version  print the version

run ` + "`weeco <command> --help`" + ` for command flags.
`

func Run(args []string) int {
	if len(args) == 0 {
		fmt.Print(usageText)
		return 1
	}
	if args[0] == "version" || args[0] == "--version" {
		fmt.Printf("weeco %s\n", Version)
		return 0
	}
	switch args[0] {
	case "help", "--help", "-h":
		fmt.Print(usageText)
		return 0
	case "start":
		return cmdStart(args[1:])
	case "status":
		return cmdStatus(args[1:])
	case "send":
		return cmdSend(args[1:])
	case "report":
		return cmdReport(args[1:])
	case "accept":
		return cmdAccept(args[1:])
	case "close":
		return cmdClose(args[1:])
	case "config":
		return cmdConfig(args[1:])
	case "install":
		return cmdInstall(args[1:])
	case "guard":
		return cmdGuard(args[1:])
	default:
		fmt.Fprintf(os.Stderr, "weeco: unknown command: %s\n", args[0])
		return 1
	}
}

func fail(err error) int             { fmt.Fprintf(os.Stderr, "weeco: error: %v\n", err); return 1 }
func helpRequested(fs *flagSet) bool { return fs.bool("help") }
