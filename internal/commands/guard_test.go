package commands

import (
	"path/filepath"
	"strings"
	"testing"

	"weeco/internal/sysutil"
)

func TestGuardBlocksDestructiveCommandsOutsideRunDir(t *testing.T) {
	runDir := t.TempDir()
	home := sysutil.HomeDir()
	blocked := []string{
		"rm -rf /",
		"rm -rf /*",
		"rm -rf ~",
		"rm -rf $HOME",
		"rm -rf " + home,
		"rm -rf ../other",
		"rm -rf /tmp/other",
		"rm -rf *",
		"rm -rf .",
		"rm -rf \"$BUILD_DIR\"",
		"sudo rm -rf build",
		"find / -delete",
		"dd if=/dev/zero of=/dev/disk0",
		"chmod -R 777 /",
		"chown -R " + home + " file",
		"git -C /tmp/other clean -fdx",
		"cd /tmp && rm -rf foo",
		"echo x > ~/.zshrc",
		"echo x > /etc/hosts",
		"mv ~/.zshrc /tmp/x",
		"cp file /tmp/x",
		"rm -rf ./*",
		"rm -rf ./",
		"rm -rf " + runDir,
		"rm -rf " + filepath.Join(runDir, "*"),
		"bash -c 'rm -rf /'",
		"sh -c 'rm -rf ~'",
		"bash -lc 'rm -rf /tmp/x'",
		"env bash -c 'rm -rf /'",
		"/bin/rm -rf /",
		"eval 'rm -rf /tmp/other'",
	}
	for _, command := range blocked {
		if reason := checkBash(runDir, command); reason == "" {
			t.Errorf("guard allowed destructive command: %s", command)
		}
	}
}

func TestGuardAllowsWorkInsideRunDir(t *testing.T) {
	runDir := t.TempDir()
	allowed := []string{
		"rm -rf build",
		"rm -rf node_modules",
		"rm -rf ./dist",
		"npm install",
		"npm run build",
		"python3 -c \"print('ok')\"",
		"git status",
		"git clean -fdx",
		"git -C . clean -fdx",
		"find . -name '*.tmp' -delete",
		"chmod 644 file.txt",
		"echo hi > out.txt",
		"echo hi 2>/dev/null",
		"cp file.txt backup.txt",
		"mv old.txt new.txt",
		"dd if=/dev/zero of=out.bin bs=1 count=1",
		"rm -rf build/*",
		"bash -c 'rm -rf build'",
		"sh -c 'npm run build'",
	}
	for _, command := range allowed {
		if reason := checkBash(runDir, command); reason != "" {
			t.Errorf("guard blocked safe command %q: %s", command, reason)
		}
	}
	inside := filepath.Join(runDir, "build")
	if reason := checkBash(runDir, "rm -rf "+inside); reason != "" {
		t.Errorf("guard blocked absolute path inside run dir: %s", reason)
	}
	if reason := checkBash(runDir, "rm -rf "+filepath.Join(runDir, "..", "other")); reason == "" {
		t.Error("guard allowed absolute path escaping run dir")
	}
}

func TestGuardSegmentsQuotedCommands(t *testing.T) {
	runDir := t.TempDir()
	cases := []struct {
		command string
		blocked bool
	}{
		{"echo 'rm -rf /'", false},
		{"echo \"hello; rm -rf /\"", false},
		{"true; rm -rf /", true},
		{"echo ok && rm -rf /tmp/x", true},
		{"printf '%s' ok | tee out.txt", false},
	}
	for _, tc := range cases {
		reason := checkBash(runDir, tc.command)
		if tc.blocked && reason == "" {
			t.Errorf("expected block: %s", tc.command)
		}
		if !tc.blocked && reason != "" {
			t.Errorf("unexpected block %q: %s", tc.command, reason)
		}
	}
	if reason := checkBash(runDir, "rm -rf /"); !strings.Contains(reason, "outside") {
		t.Errorf("unexpected reason: %s", reason)
	}
}
