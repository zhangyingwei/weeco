// Package sysutil holds process, JSON, hashing and path helpers shared by the
// weeco command layer and its adapters.
package sysutil

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// RunResult is the captured result of one external command.
type RunResult struct {
	Stdout   string
	Stderr   string
	ExitCode int
	TimedOut bool
}

// Run executes a command without a shell. A timeout of zero means no timeout.
// When a timeout is set the command runs in its own process group so the whole
// tree is killed, not just the direct child.
func Run(cmd []string, dir string, timeout time.Duration, extraEnv []string) (RunResult, error) {
	if len(cmd) == 0 {
		return RunResult{}, fmt.Errorf("empty command")
	}
	c := exec.Command(cmd[0], cmd[1:]...)
	c.Dir = dir
	if len(extraEnv) > 0 {
		c.Env = append(os.Environ(), extraEnv...)
	}
	var stdout, stderr bytes.Buffer
	c.Stdout = &stdout
	c.Stderr = &stderr
	if timeout > 0 {
		c.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	}
	if err := c.Start(); err != nil {
		if _, ok := err.(*exec.Error); ok {
			return RunResult{}, fmt.Errorf("command not found: %s", cmd[0])
		}
		return RunResult{}, err
	}
	done := make(chan error, 1)
	go func() { done <- c.Wait() }()
	var timer <-chan time.Time
	if timeout > 0 {
		t := time.NewTimer(timeout)
		defer t.Stop()
		timer = t.C
	}
	res := RunResult{}
	select {
	case err := <-done:
		res.Stdout = stdout.String()
		res.Stderr = stderr.String()
		res.ExitCode = c.ProcessState.ExitCode()
		if err != nil {
			if _, ok := err.(*exec.ExitError); ok {
				return res, nil
			}
			return res, err
		}
		return res, nil
	case <-timer:
		if c.Process != nil {
			_ = syscall.Kill(-c.Process.Pid, syscall.SIGKILL)
		}
		<-done
		res.Stdout = stdout.String()
		res.Stderr = stderr.String()
		res.ExitCode = -1
		res.TimedOut = true
		return res, nil
	}
}

// WriteJSON writes indented JSON atomically.
func WriteJSON(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return AtomicWrite(path, data)
}

// ReadJSON reads and decodes a JSON file.
func ReadJSON(path string, out any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("invalid JSON in %s: %v", path, err)
	}
	return nil
}

// ReadJSONOr returns false when the file does not exist.
func ReadJSONOr(path string, out any) (bool, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if err := json.Unmarshal(data, out); err != nil {
		return false, fmt.Errorf("invalid JSON in %s: %v", path, err)
	}
	return true, nil
}

// AtomicWrite replaces a file with the supplied bytes.
func AtomicWrite(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}

// SHA256Bytes returns the hex digest of a byte slice.
func SHA256Bytes(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// HomeDir returns the current user's home directory.
func HomeDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "."
	}
	return home
}

// ExpandHome expands a leading ~ or ~/ to the user's home directory.
func ExpandHome(path string) string {
	if path == "~" {
		return HomeDir()
	}
	if strings.HasPrefix(path, "~/") {
		return filepath.Join(HomeDir(), path[2:])
	}
	return path
}

// AbsPath expands ~ and returns a canonical absolute path. Canonicalisation
// keeps a project reached through /var and /private/var under one key.
func AbsPath(path string) (string, error) {
	abs, err := filepath.Abs(ExpandHome(path))
	if err != nil {
		return "", err
	}
	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		return resolved, nil
	}
	return abs, nil
}

// NowISO returns the current time in RFC3339 form.
func NowISO() string { return time.Now().Format(time.RFC3339) }

// Itoa is a short alias used by command builders.
func Itoa(n int) string { return strconv.Itoa(n) }
