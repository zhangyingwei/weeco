package commands

import (
	"fmt"
	"os"
)

// Version is the CLI version. Override at build time with -ldflags.
var Version = "0.9.9"

const usageText = `weeco: a company/team control plane for Herdr + Claude

usage: weeco <command> [flags]

commands:
  init          initialize a project and write the boss block to CLAUDE.md
  status        show the current requirement, stage and next skill
  requirement   new | list | show | stage | close
  workspace     create
  team          recruit | list | status | wait | nudge | repair | check | accept | rework | answer | disband
  lead          status | submit | ask
  member        recruit | status | wait | accept | rework | close
  install       install the binary, skills, slash command and templates
  trust         trust a project root for Claude Code (worktrees inherit)
  doctor        check claude, herdr and the weeco setup
  version       print the version

run ` + "`weeco <command> --help`" + ` for command flags.
`

func Run(args []string) int {
	if len(args) == 0 {
		fmt.Print(usageText)
		return 1
	}
	if args[0] == "--version" || args[0] == "version" {
		fmt.Printf("weeco %s\n", Version)
		return 0
	}
	cmd, rest := args[0], args[1:]
	switch cmd {
	case "help", "-h", "--help":
		fmt.Print(usageText)
		return 0
	case "init":
		return cmdInit(rest)
	case "status":
		return cmdStatus(rest)
	case "requirement":
		return cmdRequirement(rest)
	case "workspace":
		return cmdWorkspace(rest)
	case "team":
		return cmdTeam(rest)
	case "lead":
		return cmdLead(rest)
	case "member":
		return cmdMember(rest)
	case "install":
		return cmdInstall(rest)
	case "trust":
		return cmdTrust(rest)
	case "doctor":
		return cmdDoctor(rest)
	default:
		fmt.Fprintf(os.Stderr, "weeco: unknown command: %s\n\n", cmd)
		fmt.Print(usageText)
		return 1
	}
}

func fail(err error) int {
	fmt.Fprintf(os.Stderr, "weeco: error: %v\n", err)
	return 1
}

func helpRequested(fs *flagSet) bool { return fs.bool("help") }
