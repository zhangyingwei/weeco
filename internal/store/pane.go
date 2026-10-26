package store

import (
	"os"
	"path/filepath"
	"regexp"

	"weeco/internal/sysutil"
)

// PaneContext maps a Herdr pane back to its project and team. It lets the
// team lead run `weeco lead submit` without repeating --project and --team.
type PaneContext struct {
	ProjectRoot string `json:"project_root"`
	ProjectKey  string `json:"project_key"`
	TeamID      string `json:"team_id"`
	PaneID      string `json:"pane_id"`
	LeadAgent   string `json:"lead_agent,omitempty"`
	MemberAgent string `json:"member_agent,omitempty"`
	MemberName  string `json:"member_name,omitempty"`
	CreatedAt   string `json:"created_at"`
}

var paneFileChars = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

func panesDir() string { return filepath.Join(Home(), "panes") }

func paneContextPath(paneID string) string {
	name := paneFileChars.ReplaceAllString(paneID, "_")
	if name == "" {
		name = "pane"
	}
	return filepath.Join(panesDir(), name+"-"+sysutil.SHA1Prefix(paneID, 8)+".json")
}

func PaneContextPath(paneID string) string { return paneContextPath(paneID) }

func WritePaneContext(ctx PaneContext) error {
	if ctx.CreatedAt == "" {
		ctx.CreatedAt = sysutil.NowISO()
	}
	return sysutil.WriteJSON(paneContextPath(ctx.PaneID), ctx)
}

func LoadPaneContext(paneID string) (PaneContext, error) {
	var ctx PaneContext
	if err := sysutil.ReadJSON(paneContextPath(paneID), &ctx); err != nil {
		return ctx, err
	}
	return ctx, nil
}

func DeletePaneContext(paneID string) error {
	if paneID == "" {
		return nil
	}
	err := os.Remove(paneContextPath(paneID))
	if os.IsNotExist(err) {
		return nil
	}
	return err
}
