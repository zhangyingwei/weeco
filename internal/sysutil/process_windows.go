//go:build windows

package sysutil

import (
	"os/exec"
	"strconv"
	"syscall"
)

func setProcessGroup(c *exec.Cmd) {
	// Start the child in its own process group so a timeout can terminate the
	// whole tree instead of only the direct child.
	c.SysProcAttr = &syscall.SysProcAttr{CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP}
}

func killProcessGroup(pid int) error {
	return exec.Command("taskkill", "/T", "/F", "/PID", strconv.Itoa(pid)).Run()
}
