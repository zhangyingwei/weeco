package commands

import (
	"fmt"
	"strings"

	"weeco/internal/herdr"
	"weeco/internal/store"
)

func cmdClose(args []string) int {
	fs, err := runtimeFlags(args, []string{"reason"}, []string{"cancel"}, 1)
	if err != nil {
		return fail(err)
	}
	if helpRequested(fs) {
		fmt.Println("usage: weeco close [ALIAS] [--cancel --reason TEXT] [--run ID]")
		return 0
	}
	if fs.bool("cancel") && strings.TrimSpace(fs.value("reason")) == "" {
		return fail(fmt.Errorf("--cancel requires --reason"))
	}
	ctx, err := currentRuntime()
	if err != nil {
		return fail(err)
	}
	err = store.RuntimeLock(func() error {
		run, alias, err := resolveRun(ctx, fs.value("run"))
		if err != nil {
			return err
		}
		if err := requireLead(alias); err != nil {
			return err
		}
		names := store.MemberNames(run)
		if len(fs.positionals) == 1 {
			name := fs.positionals[0]
			if run.Members[name] == nil {
				return fmt.Errorf("unknown member %s", name)
			}
			names = []string{name}
		}
		for _, name := range names {
			m := run.Members[name]
			if m.State != "closed" && !fs.bool("cancel") && m.State != "submitted" {
				return fmt.Errorf("member %s is %s; verify a submission or cancel with a reason", name, m.State)
			}
		}
		teamWorkspace, teamTab := runTeamLocation(run)
		panes, err := herdr.List("pane", teamWorkspace)
		if err != nil {
			return err
		}
		agents, err := herdr.List("agent", "")
		if err != nil {
			return err
		}
		var failures []string
		for _, name := range names {
			m := run.Members[name]
			if m.State == "closed" {
				continue
			}
			if m.PaneID == run.LeadPane {
				return fmt.Errorf("refusing to close the lead pane")
			}
			pane := herdr.ByPane(panes, m.PaneID)
			if pane != nil {
				liveAgent := herdr.ByPane(agents, m.PaneID)
				if herdr.String(pane, "tab_id") != teamTab || (liveAgent != nil && !liveMemberMatches(liveAgent, m)) {
					failures = append(failures, name+": pane identity changed")
					continue
				}
				if err := herdr.PaneClose(m.PaneID); err != nil {
					m.LastError = err.Error()
					failures = append(failures, name+": "+err.Error())
					continue
				}
			}
			m.State = "closed"
			m.LastError = ""
			if fs.bool("cancel") {
				m.LastError = "cancelled: " + fs.value("reason")
			}
			fmt.Printf("closed: %s\n", name)
		}
		run.Closed = true
		for _, m := range run.Members {
			if m.State != "closed" {
				run.Closed = false
			}
		}
		if err := store.SaveRun(run); err != nil {
			return err
		}
		if len(failures) > 0 {
			return fmt.Errorf("some members remain open: %s; retry close", strings.Join(failures, "; "))
		}
		return nil
	})
	if err != nil {
		return fail(err)
	}
	return 0
}
