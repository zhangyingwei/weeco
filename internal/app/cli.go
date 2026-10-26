package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// flagSet is a small parser that accepts flags before or after positional
// arguments, unlike the stdlib flag package. Value flags are declared per
// command; every other `--name` is a boolean flag.
type flagSet struct {
	valueFlags  map[string]bool
	valuesMap   map[string][]string
	bools       map[string]bool
	positionals []string
}

func parseFlags(args []string, valueFlags map[string]bool) (*flagSet, error) {
	fs := &flagSet{
		valueFlags: valueFlags,
		valuesMap:  map[string][]string{},
		bools:      map[string]bool{},
	}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			fs.positionals = append(fs.positionals, args[i+1:]...)
			break
		}
		if arg == "-h" || arg == "--help" {
			fs.bools["help"] = true
			continue
		}
		if strings.HasPrefix(arg, "--") {
			name := strings.TrimPrefix(arg, "--")
			value := ""
			hasValue := false
			if idx := strings.Index(name, "="); idx >= 0 {
				value = name[idx+1:]
				name = name[:idx]
				hasValue = true
			}
			if name == "" {
				return nil, errorf("invalid flag: %s", arg)
			}
			if fs.valueFlags[name] {
				if !hasValue {
					if i+1 >= len(args) || strings.HasPrefix(args[i+1], "--") {
						return nil, errorf("flag --%s needs a value", name)
					}
					i++
					value = args[i]
				}
				fs.valuesMap[name] = append(fs.valuesMap[name], value)
				continue
			}
			if hasValue {
				switch strings.ToLower(value) {
				case "1", "true", "yes", "on":
					fs.bools[name] = true
				case "0", "false", "no", "off":
					fs.bools[name] = false
				default:
					return nil, errorf("flag --%s does not take a value", name)
				}
				continue
			}
			fs.bools[name] = true
			continue
		}
		fs.positionals = append(fs.positionals, arg)
	}
	return fs, nil
}

func (f *flagSet) value(name string) string {
	if vs := f.valuesMap[name]; len(vs) > 0 {
		return vs[len(vs)-1]
	}
	return ""
}

func (f *flagSet) values(name string) []string {
	return f.valuesMap[name]
}

func (f *flagSet) bool(name string) bool {
	return f.bools[name]
}

func (f *flagSet) has(name string) bool {
	_, ok := f.bools[name]
	return ok
}

func (f *flagSet) positional(i int, name string) (string, error) {
	if i >= len(f.positionals) {
		return "", errorf("missing %s", name)
	}
	return f.positionals[i], nil
}

func fail(err error) int {
	fmt.Fprintf(os.Stderr, "weefleet: error: %v\n", err)
	return 1
}

const usageText = `weefleet: minimal Herdr + Claude task control plane

usage: weefleet <command> [flags]

commands:
  install      install the binary, skill, slash command and templates
  doctor       check git, herdr, claude and the weefleet setup
  init         initialize a repo (writes the CLAUDE.md block by default)
  coordinator  start Claude Code as the weefleet coordinator
  new          create a task draft
  approve      record human approval of a contract
  dispatch     create worktree/pane/agent and send the brief
  status       show task states
  wait         wait for a dispatched agent to settle
  verify       verify the task revision
  accept       record human acceptance
  reject       record human rejection
  resolve      record a blocked decision
  retry        prepare another attempt
  close        close pane/tab and remove the worktree
  record       write the compact record after merge
  gc           list or clean old task dirs
  version      print the version

run ` + "`weefleet <command> --help`" + ` for command flags.
`

// Run is the CLI entry point. It returns the process exit code.
func Run(args []string) int {
	if len(args) == 0 {
		fmt.Print(usageText)
		return 1
	}
	if args[0] == "--version" || args[0] == "version" {
		fmt.Printf("weefleet %s\n", Version)
		return 0
	}
	cmd, rest := args[0], args[1:]
	// Coordinator arguments belong to Claude Code, not to weefleet.
	if cmd != "coordinator" {
		for _, arg := range rest {
			if arg == "-h" || arg == "--help" {
				fmt.Print(usageText)
				return 0
			}
		}
	}
	switch cmd {
	case "help", "-h", "--help":
		fmt.Print(usageText)
		return 0
	case "install":
		return cmdInstall(rest)
	case "doctor":
		return cmdDoctor(rest)
	case "init":
		return cmdInit(rest)
	case "coordinator":
		return cmdCoordinator(rest)
	case "new":
		return cmdNew(rest)
	case "approve":
		return cmdApprove(rest)
	case "dispatch":
		return cmdDispatch(rest)
	case "status":
		return cmdStatus(rest)
	case "wait":
		return cmdWait(rest)
	case "verify":
		return cmdVerify(rest)
	case "accept":
		return cmdAccept(rest)
	case "reject":
		return cmdReject(rest)
	case "resolve":
		return cmdResolve(rest)
	case "retry":
		return cmdRetry(rest)
	case "close":
		return cmdClose(rest)
	case "record":
		return cmdRecord(rest)
	case "gc":
		return cmdGC(rest)
	default:
		fmt.Fprintf(os.Stderr, "weefleet: unknown command: %s\n\n", cmd)
		fmt.Print(usageText)
		return 1
	}
}

func cmdCoordinator(args []string) int {
	return RunCoordinator(args)
}

// RunCoordinator starts Claude Code with the coordinator tool restrictions and
// system prompt. It is also the behavior of the `weefleet-claude` launcher.
func RunCoordinator(args []string) int {
	claude, err := lookPath("claude")
	if err != nil {
		return fail(errorf("claude not found in PATH"))
	}
	prompt := coordinatorPrompt()
	execArgs := append([]string{
		claude,
		"--disallowedTools", "Edit", "Write", "NotebookEdit",
		"--append-system-prompt", prompt,
	}, args...)
	return execReplace(execArgs)
}

// IsCoordinatorInvocation reports whether argv[0] is the weefleet-claude
// launcher (a symlink to the same binary).
func IsCoordinatorInvocation(argv0 string) bool {
	return filepath.Base(argv0) == "weefleet-claude"
}

var _ = os.Getenv
