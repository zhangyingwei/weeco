package commands

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"weeco/internal/assets"
	"weeco/internal/herdr"
	"weeco/internal/store"
	"weeco/internal/sysutil"
)

func cmdStart(args []string) int {
	fs, err := runtimeFlags(args, []string{"name", "role", "task"}, []string{"replace"}, 0)
	if err != nil {
		return fail(err)
	}
	if helpRequested(fs) {
		fmt.Println("usage: weeco start --name ALIAS --role TEXT --task FILE [--replace] [--run ID]")
		return 0
	}
	name, role := fs.value("name"), fs.value("role")
	if !store.ValidID(name) || name == "lead" || role == "" {
		return fail(fmt.Errorf("--name must be a lowercase alias other than lead; --role is required"))
	}
	task, err := readText(fs.value("task"))
	if err != nil {
		return fail(err)
	}
	ctx, err := currentRuntime()
	if err != nil {
		return fail(err)
	}
	if _, err := exec.LookPath("claude"); err != nil {
		return fail(fmt.Errorf("claude is not on PATH"))
	}
	err = store.RuntimeLock(func() error {
		run, alias, err := resolveRun(ctx, fs.value("run"))
		if errors.Is(err, errNoRun) && fs.value("run") == "" {
			// A retired member pane must not become a recursive team lead.
			runs, e := store.Runs()
			if e != nil {
				return e
			}
			for _, prior := range runs {
				for _, m := range prior.Members {
					if m.PaneID == ctx.Pane && m.Session == ctx.Session {
						return fmt.Errorf("member panes cannot recruit teams")
					}
				}
			}
			id, e := store.NewID("r-")
			if e != nil {
				return e
			}
			dir, e := os.Getwd()
			if e != nil {
				return e
			}
			dir, e = sysutil.AbsPath(dir)
			if e != nil {
				return e
			}
			run = &store.Run{ID: id, LeadPane: ctx.Pane, LeadSession: ctx.Session, Workspace: ctx.Workspace, TabID: ctx.Tab, Dir: dir, Members: map[string]*store.Member{}}
			alias, err = "lead", nil
		}
		if err != nil {
			return err
		}
		if err := requireLead(alias); err != nil {
			return err
		}
		if run.Closed {
			return fmt.Errorf("run %s is closed", run.ID)
		}
		agents, err := herdr.List("agent", "")
		if err != nil {
			return err
		}
		old := run.Members[name]
		if old != nil && !fs.bool("replace") {
			if old.TaskSHA != sysutil.SHA256Bytes(task) || old.Role != role {
				return fmt.Errorf("member %s already exists; use send to change its work", name)
			}
			if old.State == "closed" || old.State == "failed" {
				return fmt.Errorf("member %s is %s; inspect it, then use --replace", name, old.State)
			}
			if err := verifyMember(run, old, agents); err != nil {
				return err
			}
			fmt.Printf("member: %s\nrun: %s\npane: %s\nexisting: yes\n", name, run.ID, old.PaneID)
			return nil
		}
		if fs.bool("replace") && old == nil {
			return fmt.Errorf("cannot replace missing member %s", name)
		}
		if old != nil && old.PaneID != "" {
			if herdr.ByPane(agents, old.PaneID) != nil {
				return fmt.Errorf("member %s still has a live agent; close it before replacement", name)
			}
			panes, e := herdr.List("pane", run.Workspace)
			if e != nil {
				return e
			}
			if herdr.ByPane(panes, old.PaneID) != nil {
				return fmt.Errorf("old pane still exists; inspect and close it before replacement")
			}
		}
		suffix, err := store.NewID("m-")
		if err != nil {
			return err
		}
		m := &store.Member{Name: name, Agent: "weeco-" + suffix[2:], Role: role, TaskSHA: sysutil.SHA256Bytes(task), State: "starting", Turn: 1}
		if old != nil {
			m.Turn = old.Turn + 1
		}
		m.Task = filepath.Join(memberDir(run, m), fmt.Sprintf("task-%d.md", m.Turn))
		if err := sysutil.AtomicWrite(m.Task, task); err != nil {
			return err
		}
		prompt, err := assets.Read(assets.MemberPrompt)
		if err != nil {
			return err
		}
		promptPath := filepath.Join(memberDir(run, m), "member-prompt.md")
		if err := sysutil.AtomicWrite(promptPath, prompt); err != nil {
			return err
		}
		run.Members[name] = m
		if err := store.SaveRun(run); err != nil {
			return err
		}
		pane, err := herdr.PaneSplit(run.LeadPane, "right", run.Dir, 0.5)
		if err != nil {
			return failStart(run, m, err)
		}
		m.PaneID = pane
		if err := store.SaveRun(run); err != nil {
			return failStart(run, m, err)
		}
		if err := herdr.AgentStart(m.Agent, pane, "claude", 60000, []string{store.RunDir(run.ID)}, "", promptPath, memberAllowedTools()); err != nil {
			return failStart(run, m, err)
		}
		agents, err = herdr.List("agent", "")
		if err != nil {
			return failStart(run, m, err)
		}
		a := herdr.ByPane(agents, pane)
		if a == nil || herdr.Session(a) == "" || herdr.String(a, "tab_id") != run.TabID {
			return failStart(run, m, fmt.Errorf("agent start returned without a matching live session"))
		}
		m.Session, m.State = herdr.Session(a), "running"
		if err := store.SaveRun(run); err != nil {
			return failStart(run, m, err)
		}
		if err := herdr.AgentPrompt(pane, memberPrompt(run, m, m.Task), false, 0); err != nil {
			return failStart(run, m, err)
		}
		if err := herdr.PaneRename(pane, name+" ("+role+")"); err != nil {
			fmt.Printf("warning: rename: %v\n", err)
		}
		fmt.Printf("run: %s\nmember: %s\nagent: %s\npane: %s\nturn: %d\ntask: %s\n", run.ID, m.Name, m.Agent, m.PaneID, m.Turn, m.Task)
		return nil
	})
	if err != nil {
		return fail(err)
	}
	return 0
}

func failStart(run *store.Run, m *store.Member, cause error) error {
	m.State, m.LastError = "failed", cause.Error()
	if m.PaneID != "" {
		if err := herdr.PaneClose(m.PaneID); err != nil {
			m.LastError += "; cleanup failed: " + err.Error()
		} else {
			m.PaneID, m.Session = "", ""
		}
	}
	if err := store.SaveRun(run); err != nil {
		return fmt.Errorf("start failed: %v; save failed: %w", cause, err)
	}
	return fmt.Errorf("start failed: %s", m.LastError)
}
