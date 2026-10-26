package app

import (
	"os"
	"os/exec"
	"syscall"

	"weefleet/internal/assets"
)

func coordinatorPrompt() string {
	data, err := assets.Read(assets.CoordinatorPrompt)
	if err != nil {
		return "你是 weefleet 协调者。先加载 weefleet-coordinator skill，按它执行；不改代码，只在合同、blocked、accept 三个门停下来问人。"
	}
	return string(data)
}

func lookPath(name string) (string, error) {
	return exec.LookPath(name)
}

// execReplace replaces the current process so the Claude Code session keeps
// the terminal and signal handling of the launcher.
func execReplace(argv []string) int {
	if err := syscall.Exec(argv[0], argv, os.Environ()); err != nil {
		return fail(errorf("could not start claude: %v", err))
	}
	return 0
}
