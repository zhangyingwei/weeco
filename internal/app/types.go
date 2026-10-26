package app

import (
	"encoding/json"
	"fmt"
	"os"
	"time"
)

// Version is the CLI version. Override at build time with -ldflags.
var Version = "0.2.0"

const (
	StateDraft          = "draft"
	StateRunning        = "running"
	StateBlocked        = "blocked"
	StateReported       = "reported"
	StateUncertain      = "uncertain"
	StateLost           = "lost"
	StateVerified       = "verified"
	StateFailed         = "failed"
	StateBudgetExceeded = "budget_exceeded"
	StateAccepted       = "accepted"
	StateRejected       = "rejected"
	StateClosed         = "closed"
	stateUnknown        = "unknown"
)

var terminalStates = map[string]bool{
	StateAccepted: true,
	StateRejected: true,
	StateClosed:   true,
}

var retryableStates = map[string]bool{
	StateFailed:    true,
	StateUncertain: true,
	StateLost:      true,
}

// Budget is the per-task verification budget.
type Budget struct {
	MaxTotalSeconds float64 `json:"max_total_seconds"`
	MaxRunsPerCheck int     `json:"max_runs_per_check"`
	MaxOutputLines  int     `json:"max_output_lines"`
}

// Config is the user-level configuration at WEEFLEET_HOME/config.json.
type Config struct {
	WorktreeRoot   *string `json:"worktree_root"`
	AgentKind      string  `json:"agent_kind"`
	ReadyTimeoutMS int     `json:"ready_timeout_ms"`
	WaitTimeoutS   float64 `json:"wait_timeout_s"`
	Budget         Budget  `json:"budget"`
}

func defaultConfig() Config {
	return Config{
		WorktreeRoot:   nil,
		AgentKind:      "claude",
		ReadyTimeoutMS: 60000,
		WaitTimeoutS:   1800,
		Budget: Budget{
			MaxTotalSeconds: 300,
			MaxRunsPerCheck: 2,
			MaxOutputLines:  80,
		},
	}
}

// RepoMeta is WEEFLEET_HOME/repos/<key>/repo.json.
type RepoMeta struct {
	RepoRoot      string `json:"repo_root"`
	RepoKey       string `json:"repo_key"`
	DefaultBranch string `json:"default_branch"`
	CreatedAt     string `json:"created_at"`
	UpdatedAt     string `json:"updated_at"`
}

// Event is one line in ledger.jsonl.
type Event struct {
	TS    string         `json:"ts"`
	Task  string         `json:"task"`
	Event string         `json:"event"`
	Data  map[string]any `json:"data"`
}

// Task is the folded state of one task.
type Task struct {
	ID               string
	State            string
	Attempt          int
	ContractApproved bool
	ContractNote     string
	Goal             string
	Scope            []string
	BriefPath        string
	ChecksPath       string
	ChecksSHA256     string
	RepoRoot         string
	Worktree         string
	PaneID           string
	TabID            string
	Agent            string
	BaseSHA          string
	Branch           string
	WorkspaceID      string
	PromptSeq        *int
	BlockedReason    string
	ReportPath       string
	HeadSHA          string
	VerifyPath       string
	FailureReason    string
	BudgetReason     string
	AcceptedBy       string
	AcceptedAt       string
	AcceptNote       string
	RejectedReason   string
	RetryReason      string
	RecordPath       string
	MergeCommit      string
	CreatedAt        string
	UpdatedAt        string
	LastEvent        string
}

// Check is one acceptance command.
type Check struct {
	ID         string  `json:"id"`
	Cmd        string  `json:"cmd"`
	ExpectExit int     `json:"expect_exit"`
	TimeoutS   float64 `json:"timeout_s"`
}

// ChecksFile is the canonical checks.json.
type ChecksFile struct {
	Checks []Check `json:"checks"`
	Budget Budget  `json:"budget"`
}

// Report is the worker-written report.json.
type Report struct {
	TaskID        string        `json:"task_id"`
	Status        string        `json:"status"`
	Summary       string        `json:"summary"`
	BaseSHA       string        `json:"base_sha"`
	HeadSHA       string        `json:"head_sha"`
	ChangedFiles  []string      `json:"changed_files"`
	Checks        []ReportCheck `json:"checks"`
	Evidence      []string      `json:"evidence"`
	OpenRisks     []string      `json:"open_risks"`
	NeedsDecision any           `json:"needs_decision"`
}

// ReportCheck is the worker's own claim about a check. Verification re-runs
// the command and does not trust these values.
type ReportCheck struct {
	ID   string `json:"id"`
	Cmd  string `json:"cmd"`
	Exit int    `json:"exit"`
	Log  string `json:"log"`
}

// CheckResult is the verifier's result for one check.
type CheckResult struct {
	ID         string  `json:"id"`
	Cmd        string  `json:"cmd"`
	Exit       int     `json:"exit"`
	ExpectExit int     `json:"expect_exit"`
	Passed     bool    `json:"passed"`
	TimedOut   bool    `json:"timed_out"`
	DurationS  float64 `json:"duration_s"`
	Log        string  `json:"log"`
	Tail       string  `json:"tail"`
}

// EvidenceResult records a hashed evidence file.
type EvidenceResult struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
}

// VerifyResult is verify.json.
type VerifyResult struct {
	Key            string            `json:"key,omitempty"`
	TaskID         string            `json:"task_id"`
	BaseSHA        string            `json:"base_sha,omitempty"`
	HeadSHA        string            `json:"head_sha,omitempty"`
	Passed         bool              `json:"passed"`
	BudgetExceeded bool              `json:"budget_exceeded,omitempty"`
	Reason         string            `json:"reason,omitempty"`
	Checks         []CheckResult     `json:"checks,omitempty"`
	Warnings       []string          `json:"warnings,omitempty"`
	Evidence       []EvidenceResult  `json:"evidence,omitempty"`
	EvidenceErrors []string          `json:"evidence_errors,omitempty"`
	Trailers       map[string]string `json:"trailers,omitempty"`
	VerifiedAt     string            `json:"verified_at"`
}

// StatusRow is one row of `weefleet status --json`.
type StatusRow struct {
	Task     string `json:"task"`
	State    string `json:"state"`
	Agent    string `json:"agent"`
	Pane     string `json:"pane"`
	Minutes  int    `json:"minutes"`
	Note     string `json:"note"`
	Worktree string `json:"worktree,omitempty"`
	TaskDir  string `json:"task_dir,omitempty"`
}

// WeefleetError is a user-facing error.
type WeefleetError struct{ Message string }

func (e *WeefleetError) Error() string { return e.Message }

func errorf(format string, args ...any) error {
	return &WeefleetError{Message: fmt.Sprintf(format, args...)}
}

func nowISO() string {
	return time.Now().Format(time.RFC3339)
}

func asString(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func asInt(v any) int {
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	case int64:
		return int(n)
	case json.Number:
		i, _ := n.Int64()
		return int(i)
	}
	return 0
}

func asFloat(v any) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case int:
		return float64(n)
	case int64:
		return float64(n)
	case json.Number:
		f, _ := n.Float64()
		return f
	}
	return 0
}

func asStringSlice(v any) []string {
	items, ok := v.([]any)
	if !ok {
		if ss, ok := v.([]string); ok {
			return ss
		}
		return nil
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		if s, ok := item.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

// fileExists is a small convenience used by several commands.
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
