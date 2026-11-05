package commands

import (
	"fmt"
	"os"
	"path/filepath"

	"weeco/internal/config"
	"weeco/internal/store"
	"weeco/internal/sysutil"
)

func cmdConfig(args []string) int {
	if len(args) == 0 || helpRequestedFor(args) {
		fmt.Print(`usage: weeco config <init|check>

  init    write a minimal config file (use --project for ./.weeco/config.json, --force to replace)
  check   print the resolved levels and candidates
`)
		if len(args) == 0 {
			return 1
		}
		return 0
	}
	switch args[0] {
	case "init":
		return configInit(args[1:])
	case "check":
		return configCheck(args[1:])
	default:
		return fail(fmt.Errorf("unknown config command: %s", args[0]))
	}
}

func helpRequestedFor(args []string) bool {
	for _, arg := range args {
		if arg == "--help" || arg == "-h" {
			return true
		}
	}
	return false
}

func configInit(args []string) int {
	fs, err := parseFlags(args, map[string]bool{})
	if err != nil {
		return fail(err)
	}
	for name := range fs.bools {
		if name != "project" && name != "force" && name != "help" {
			return fail(fmt.Errorf("unknown flag --%s", name))
		}
	}
	path := filepath.Join(store.Home(), "config.json")
	if fs.bool("project") {
		dir, err := os.Getwd()
		if err != nil {
			return fail(err)
		}
		path = filepath.Join(dir, ".weeco", "config.json")
	}
	if _, err := os.Stat(path); err == nil && !fs.bool("force") {
		return fail(fmt.Errorf("%s already exists; use --force to replace it", path))
	}
	if err := sysutil.AtomicWrite(path, config.Template()); err != nil {
		return fail(err)
	}
	fmt.Printf("config: %s\n\nEdit it using this shape:\n%s\n", path, config.Example)
	return 0
}

func configCheck(args []string) int {
	fs, err := parseFlags(args, map[string]bool{})
	if err != nil {
		return fail(err)
	}
	for name := range fs.bools {
		if name != "help" {
			return fail(fmt.Errorf("unknown flag --%s", name))
		}
	}
	dir, err := os.Getwd()
	if err != nil {
		return fail(err)
	}
	cfg, err := config.Load(dir)
	if err != nil {
		return fail(err)
	}
	fmt.Print(cfg.Describe())
	return 0
}
