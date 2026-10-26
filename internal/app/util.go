package app

import (
	"bytes"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

func sha256Bytes(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func sha1Prefix(text string, n int) string {
	sum := sha1.Sum([]byte(text))
	return hex.EncodeToString(sum[:])[:n]
}

func sha256Text(text string) string {
	return sha256Bytes([]byte(text))
}

func itoa(n int) string {
	return strconv.Itoa(n)
}

func sha256File(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func readJSON(path string, out any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, out); err != nil {
		return errorf("invalid JSON in %s: %v", path, err)
	}
	return nil
}

// readJSONOr returns false when the file does not exist.
func readJSONOr(path string, out any) (bool, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if err := json.Unmarshal(data, out); err != nil {
		return false, errorf("invalid JSON in %s: %v", path, err)
	}
	return true, nil
}

func atomicWrite(path string, data []byte) error {
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

func writeJSON(path string, obj any) error {
	data, err := json.MarshalIndent(obj, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return atomicWrite(path, data)
}

type runResult struct {
	Stdout   string
	Stderr   string
	ExitCode int
	TimedOut bool
}

// runCmd runs a command without a shell. A timeout of 0 means no timeout.
func runCmd(cmd []string, dir string, timeout time.Duration, extraEnv []string) (runResult, error) {
	if len(cmd) == 0 {
		return runResult{}, errorf("empty command")
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
		// Put the command in its own process group so a timeout kills the
		// whole check (sh and anything it spawned), not just the shell.
		c.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	}
	if err := c.Start(); err != nil {
		if _, ok := err.(*exec.Error); ok {
			return runResult{}, errorf("command not found: %s", cmd[0])
		}
		return runResult{}, err
	}
	done := make(chan error, 1)
	go func() { done <- c.Wait() }()
	var timer <-chan time.Time
	if timeout > 0 {
		t := time.NewTimer(timeout)
		defer t.Stop()
		timer = t.C
	}
	res := runResult{}
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

// runCmdWithInput runs a command and feeds it stdin.
func runCmdWithInput(cmd []string, input string) (runResult, error) {
	c := exec.Command(cmd[0], cmd[1:]...)
	c.Stdin = strings.NewReader(input)
	var stdout, stderr bytes.Buffer
	c.Stdout = &stdout
	c.Stderr = &stderr
	err := c.Run()
	res := runResult{Stdout: stdout.String(), Stderr: stderr.String()}
	if c.ProcessState != nil {
		res.ExitCode = c.ProcessState.ExitCode()
	}
	if err != nil {
		if _, ok := err.(*exec.ExitError); ok {
			return res, nil
		}
		return res, err
	}
	return res, nil
}

// runChecked runs a command and turns a non-zero exit into a user-facing error.
func runChecked(cmd []string, dir string, timeout time.Duration) (runResult, error) {
	res, err := runCmd(cmd, dir, timeout, nil)
	if err != nil {
		return res, err
	}
	if res.ExitCode != 0 {
		detail := strings.TrimSpace(res.Stderr)
		if detail == "" {
			detail = strings.TrimSpace(res.Stdout)
		}
		msg := fmt.Sprintf("command failed (%d): %s", res.ExitCode, strings.Join(cmd, " "))
		if detail != "" {
			msg += "\n" + detail
		}
		return res, errorf("%s", msg)
	}
	return res, nil
}

func parseISOTime(value string) time.Time {
	t, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return time.Now()
	}
	return t
}

func todayStamp() string {
	return time.Now().Format("20060102")
}

func homeDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "."
	}
	return home
}

func expandHome(path string) string {
	if path == "~" {
		return homeDir()
	}
	if strings.HasPrefix(path, "~/") {
		return filepath.Join(homeDir(), path[2:])
	}
	return path
}

func absPath(path string) (string, error) {
	expanded := expandHome(path)
	return filepath.Abs(expanded)
}
