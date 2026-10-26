package app

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestMatchScope(t *testing.T) {
	cases := []struct {
		path  string
		scope []string
		want  bool
	}{
		{"src/auth/a.py", []string{"src/auth/**"}, true},
		{"src/b.py", []string{"src/auth/**"}, false},
		{"anything", []string{"**"}, true},
		{"tests/x.test.ts", []string{"tests/*.test.ts"}, true},
		{"tests/nested/x.test.ts", []string{"tests/*.test.ts"}, false},
	}
	for _, c := range cases {
		if got := matchScope(c.path, c.scope); got != c.want {
			t.Errorf("matchScope(%q, %v) = %v, want %v", c.path, c.scope, got, c.want)
		}
	}
}

func TestParseChecks(t *testing.T) {
	raw := []byte(`{"checks":[{"id":"A1","cmd":"true","expect_exit":0,"timeout_s":5}],"budget":{"max_total_seconds":10}}`)
	checks, _, err := parseChecks(raw, "checks.json")
	if err != nil {
		t.Fatal(err)
	}
	if checks.Checks[0].ID != "A1" {
		t.Errorf("id = %q", checks.Checks[0].ID)
	}
	if checks.Budget.MaxTotalSeconds != 10 {
		t.Errorf("max_total_seconds = %v", checks.Budget.MaxTotalSeconds)
	}
	if checks.Budget.MaxRunsPerCheck != 2 {
		t.Errorf("max_runs_per_check default = %v", checks.Budget.MaxRunsPerCheck)
	}
	if _, _, err := parseChecks([]byte(`{"checks":[]}`), "checks.json"); err == nil {
		t.Error("empty checks should fail")
	}
	dup := []byte(`{"checks":[{"id":"A1","cmd":"true"},{"id":"A1","cmd":"true"}]}`)
	if _, _, err := parseChecks(dup, "checks.json"); err == nil {
		t.Error("duplicate check ids should fail")
	}
}

func TestParseChecksUsesConfigBudgetBase(t *testing.T) {
	base := Budget{MaxTotalSeconds: 7, MaxRunsPerCheck: 1, MaxOutputLines: 9}
	checks, _, err := parseChecksWithBase([]byte(`{"checks":[{"id":"A1","cmd":"true"}]}`), "x", base)
	if err != nil {
		t.Fatal(err)
	}
	if checks.Budget != base {
		t.Fatalf("budget = %+v, want %+v", checks.Budget, base)
	}
	checks, _, err = parseChecksWithBase(
		[]byte(`{"checks":[{"id":"A1","cmd":"true"}],"budget":{"max_total_seconds":3}}`), "x", base)
	if err != nil {
		t.Fatal(err)
	}
	if checks.Budget.MaxTotalSeconds != 3 || checks.Budget.MaxRunsPerCheck != 1 || checks.Budget.MaxOutputLines != 9 {
		t.Fatalf("partial override = %+v", checks.Budget)
	}
}

func TestNextTaskIDSkipsLedgerIDs(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("WEEFLEET_HOME", tmp)
	key := "repo-test"
	taskID := "T-" + todayStamp() + "-01"
	if err := appendEvent(key, taskID, "closed", nil); err != nil {
		t.Fatal(err)
	}
	// The task dir is gone (as after gc), but the ledger still owns the id.
	if err := os.MkdirAll(filepath.Join(repoDir(key), "tasks"), 0o755); err != nil {
		t.Fatal(err)
	}
	got, err := nextTaskID(key)
	if err != nil {
		t.Fatal(err)
	}
	if got != "T-"+todayStamp()+"-02" {
		t.Fatalf("nextTaskID = %s, want -02", got)
	}
}

func TestRunCmdTimeoutKillsProcessGroup(t *testing.T) {
	done := make(chan runResult, 1)
	go func() {
		res, _ := runCmd([]string{"sh", "-c", "sleep 10 & wait"}, "", 150*time.Millisecond, nil)
		done <- res
	}()
	select {
	case res := <-done:
		if !res.TimedOut || res.ExitCode != -1 {
			t.Fatalf("result = %+v, want timed out", res)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("runCmd did not return after its timeout")
	}
}

func TestAgentName(t *testing.T) {
	got, err := agentName("T-20261026-01", 1)
	if err != nil || got != "t-20261026-01-impl" {
		t.Errorf("attempt 1 = %q, %v", got, err)
	}
	got, err = agentName("T-20261026-01", 2)
	if err != nil || got != "t-20261026-01-impl-a2" {
		t.Errorf("attempt 2 = %q, %v", got, err)
	}
	got, _ = agentName("T-20261026-01", 12)
	if len(got) > 32 {
		t.Errorf("agent name too long: %q", got)
	}
}

func TestCacheKey(t *testing.T) {
	a := cacheKey("T-1", "abc", []byte("checks"))
	b := cacheKey("T-1", "abc", []byte("checks"))
	c := cacheKey("T-1", "def", []byte("checks"))
	if a != b {
		t.Error("same inputs should give the same key")
	}
	if a == c {
		t.Error("different head should give a different key")
	}
}

func TestCrossCheckRejectsMissingTrailers(t *testing.T) {
	task := Task{ID: "T-20261026-01", Attempt: 1}
	checks := ChecksFile{Checks: []Check{{ID: "A1", Cmd: "true"}}}
	report := Report{Status: "done", BaseSHA: "base", HeadSHA: "head"}
	if errors := crossCheck(report, map[string]string{}, task, checks, "head", "base"); len(errors) == 0 {
		t.Fatal("missing trailers should be rejected")
	}
	trailers := map[string]string{
		"Task": "T-20261026-01", "Attempt": "1", "Checks": "A1",
		"Report": "report.json", "Base": "base", "Status": "done",
	}
	if errors := crossCheck(report, trailers, task, checks, "head", "base"); len(errors) != 0 {
		t.Fatalf("complete trailers should pass: %v", errors)
	}
}
