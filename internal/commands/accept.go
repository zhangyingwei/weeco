package commands

import (
	"fmt"
	"path/filepath"
	"strconv"

	"weeco/internal/store"
	"weeco/internal/sysutil"
)

func cmdAccept(args []string) int {
	fs, err := runtimeFlags(args, []string{"turn", "file"}, nil, 1)
	if err != nil {
		return fail(err)
	}
	if helpRequested(fs) {
		fmt.Println("usage: weeco accept [ALIAS] --turn N --file NOTE [--run ID]")
		return 0
	}
	turn, err := strconv.Atoi(fs.value("turn"))
	if err != nil || turn < 1 {
		return fail(fmt.Errorf("--turn must be the positive turn number being accepted"))
	}
	note, err := readText(fs.value("file"))
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
		if err := requireLead(alias); err != nil {
			return err
		}
		name := ""
		if len(fs.positionals) == 1 {
			name = fs.positionals[0]
		} else if len(run.Members) == 1 {
			for member := range run.Members {
				name = member
			}
		} else {
			return fmt.Errorf("specify which member to accept")
		}
		m := run.Members[name]
		if m == nil {
			return fmt.Errorf("unknown member %s", name)
		}
		if m.Turn != turn {
			return fmt.Errorf("member %s is on turn %d, not turn %d", name, m.Turn, turn)
		}
		if m.State != "submitted" {
			return fmt.Errorf("member %s is %s; only a submitted report can be accepted", name, m.State)
		}
		path := filepath.Join(memberDir(run, m), "accepts", fmt.Sprintf("turn-%d-accept.md", turn))
		if err := sysutil.AtomicWrite(path, note); err != nil {
			return err
		}
		m.AcceptedTurn = turn
		m.AcceptedRev = m.Revision
		m.AcceptNote = path
		m.AcceptedAt = sysutil.NowISO()
		if err := store.SaveRun(run); err != nil {
			return err
		}
		fmt.Printf("member: %s\nturn: %d\nreport: %s\nnote: %s\n", m.Name, m.Turn, m.Report, path)
		return nil
	})
	if err != nil {
		return fail(err)
	}
	return 0
}
