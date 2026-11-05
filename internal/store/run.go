package store

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"syscall"

	"weeco/internal/sysutil"
)

type Run struct {
	ID            string             `json:"id"`
	LeadPane      string             `json:"lead_pane"`
	LeadSession   string             `json:"lead_session"`
	Workspace     string             `json:"workspace"`
	TabID         string             `json:"tab_id"`
	Dir           string             `json:"dir"`
	TeamPane      string             `json:"team_pane,omitempty"`
	TeamWorkspace string             `json:"team_workspace,omitempty"`
	TeamTab       string             `json:"team_tab,omitempty"`
	Members       map[string]*Member `json:"members"`
	Closed        bool               `json:"closed"`
}

type Member struct {
	Name         string    `json:"name"`
	Agent        string    `json:"agent"`
	Kind         string    `json:"kind,omitempty"`
	PaneID       string    `json:"pane_id"`
	Session      string    `json:"session"`
	Role         string    `json:"role"`
	Level        string    `json:"level,omitempty"`
	Model        string    `json:"model,omitempty"`
	Effort       string    `json:"effort,omitempty"`
	Task         string    `json:"task"`
	TaskSHA      string    `json:"task_sha"`
	State        string    `json:"state"`
	Turn         int       `json:"turn"`
	Report       string    `json:"report,omitempty"`
	Revision     int       `json:"revision"`
	Seen         int       `json:"seen"`
	SeenLive     string    `json:"seen_live,omitempty"`
	LastError    string    `json:"last_error,omitempty"`
	Attempts     []Attempt `json:"attempts,omitempty"`
	AcceptedTurn int       `json:"accepted_turn,omitempty"`
	AcceptedRev  int       `json:"accepted_revision,omitempty"`
	AcceptNote   string    `json:"accept_note,omitempty"`
	AcceptedAt   string    `json:"accepted_at,omitempty"`
}

// Attempt records one model candidate the lead tried before a member started.
type Attempt struct {
	Agent  string `json:"agent"`
	Model  string `json:"model,omitempty"`
	Effort string `json:"effort,omitempty"`
	Error  string `json:"error,omitempty"`
	At     string `json:"at"`
}

var validID = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,63}$`)

func ValidID(id string) bool { return validID.MatchString(id) }

func NewID(prefix string) (string, error) {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return prefix + hex.EncodeToString(b), nil
}

func RunDir(id string) string { return filepath.Join(Home(), "runs", id) }

func SaveRun(run *Run) error {
	if !ValidID(run.ID) {
		return fmt.Errorf("invalid run id %q", run.ID)
	}
	return sysutil.WriteJSON(filepath.Join(RunDir(run.ID), "run.json"), run)
}

func LoadRun(id string) (*Run, error) {
	if !ValidID(id) {
		return nil, fmt.Errorf("invalid run id %q", id)
	}
	var run Run
	if err := sysutil.ReadJSON(filepath.Join(RunDir(id), "run.json"), &run); err != nil {
		return nil, err
	}
	if run.ID != id || run.LeadPane == "" || run.Members == nil {
		return nil, fmt.Errorf("invalid run record %s", id)
	}
	for name, m := range run.Members {
		if !ValidID(name) || m == nil || m.Name != name {
			return nil, fmt.Errorf("invalid member record in %s", id)
		}
	}
	return &run, nil
}

func Runs() ([]*Run, error) {
	paths, err := filepath.Glob(filepath.Join(Home(), "runs", "*", "run.json"))
	if err != nil {
		return nil, err
	}
	var runs []*Run
	for _, path := range paths {
		run, err := LoadRun(filepath.Base(filepath.Dir(path)))
		if err != nil {
			return nil, err
		}
		runs = append(runs, run)
	}
	return runs, nil
}

func MemberNames(run *Run) []string {
	names := make([]string, 0, len(run.Members))
	for name := range run.Members {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// RuntimeLock also serializes creation, so repeated starts from one pane cannot
// create separate teams. Reports use the same lock as send/close.
func RuntimeLock(fn func() error) error {
	if err := os.MkdirAll(filepath.Join(Home(), "runs"), 0o700); err != nil {
		return err
	}
	f, err := os.OpenFile(filepath.Join(Home(), "runs", ".lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		return err
	}
	defer syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
	return fn()
}
