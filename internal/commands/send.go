package commands

import (
	"errors"
	"fmt"
	"path/filepath"

	"weeco/internal/herdr"
	"weeco/internal/store"
	"weeco/internal/sysutil"
)

func cmdSend(args []string) int {
	fs, err := runtimeFlags(args, []string{"file"}, nil, 1)
	if err != nil {
		return fail(err)
	}
	if helpRequested(fs) {
		fmt.Println("usage: weeco send <ALIAS|lead> --file FILE [--run ID]")
		return 0
	}
	target, err := fs.positional(0, "member alias or lead")
	if err != nil {
		return fail(err)
	}
	data, err := readText(fs.value("file"))
	if err != nil {
		return fail(err)
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
		if run.Closed {
			return fmt.Errorf("run %s is closed", run.ID)
		}
		agents, err := herdr.List("agent", "")
		if err != nil {
			return err
		}
		if target == "lead" {
			if alias == "lead" {
				return fmt.Errorf("lead cannot message itself")
			}
			a := herdr.ByPane(agents, run.LeadPane)
			if a == nil || herdr.Session(a) != run.LeadSession {
				return fmt.Errorf("lead session is unavailable")
			}
			message := fmt.Sprintf("weeco %s member %s: %s", run.ID, alias, data)
			if err := herdr.AgentPrompt(run.LeadPane, message, false, 0); err != nil {
				return err
			}
			fmt.Println("sent to lead")
			return nil
		}
		if err := requireLead(alias); err != nil {
			return err
		}
		m := run.Members[target]
		if m == nil {
			return fmt.Errorf("unknown member %s", target)
		}
		if m.State == "closed" || m.State == "failed" {
			return fmt.Errorf("member %s is %s", target, m.State)
		}
		if err := verifyMember(run, m, agents); err != nil {
			return err
		}
		old := *m
		m.Turn++
		m.Report, m.State, m.LastError = "", "running", ""
		m.Seen, m.SeenLive = m.Revision, ""
		directive := filepath.Join(memberDir(run, m), fmt.Sprintf("instruction-%d.md", m.Turn))
		if err := sysutil.AtomicWrite(directive, data); err != nil {
			*m = old
			return err
		}
		if err := store.SaveRun(run); err != nil {
			*m = old
			return err
		}
		message := memberPrompt(run, m, directive)
		if err := herdr.AgentPrompt(m.PaneID, message, false, 0); err != nil {
			var commandErr *herdr.CommandError
			if errors.As(err, &commandErr) && commandErr.Code == "agent_blocked" {
				*m = old
				if saveErr := store.SaveRun(run); saveErr != nil {
					return fmt.Errorf("%v; restore assignment: %w", err, saveErr)
				}
				return fmt.Errorf("instruction was not sent; current turn %d is unchanged: %w", m.Turn, err)
			}
			// Delivery can be ambiguous after a timeout. Keep the new turn, never
			// restore an older completion claim; the lead can inspect and retry.
			m.LastError = "delivery not confirmed: " + err.Error()
			if saveErr := store.SaveRun(run); saveErr != nil {
				return fmt.Errorf("%v; save: %w", err, saveErr)
			}
			return fmt.Errorf("%s; inspect member before resending", m.LastError)
		}
		fmt.Printf("sent: %s\nturn: %d\n", m.Name, m.Turn)
		return nil
	})
	if err != nil {
		return fail(err)
	}
	return 0
}
