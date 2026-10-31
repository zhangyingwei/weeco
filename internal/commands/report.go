package commands

import (
	"fmt"
	"path/filepath"
	"strconv"

	"weeco/internal/store"
	"weeco/internal/sysutil"
)

func cmdReport(args []string) int {
	fs, err := runtimeFlags(args, []string{"turn", "file"}, []string{"blocked"}, 0)
	if err != nil {
		return fail(err)
	}
	if helpRequested(fs) {
		fmt.Println("usage: weeco report --turn N --file FILE [--blocked] [--run ID]")
		return 0
	}
	turn, err := strconv.Atoi(fs.value("turn"))
	if err != nil || turn < 1 {
		return fail(fmt.Errorf("--turn must match the positive turn number in your assignment"))
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
		if alias == "lead" {
			return fmt.Errorf("only members submit reports; the lead verifies actual deliverables")
		}
		m := run.Members[alias]
		if m.Turn != turn {
			return fmt.Errorf("stale report turn %d; current assignment is turn %d", turn, m.Turn)
		}
		if m.State != "running" && m.State != "blocked" && m.State != "submitted" {
			return fmt.Errorf("member %s is %s", alias, m.State)
		}
		path := filepath.Join(memberDir(run, m), "reports", fmt.Sprintf("turn-%d-report-%d.md", turn, m.Revision+1))
		if err := sysutil.AtomicWrite(path, data); err != nil {
			return err
		}
		m.Report, m.State = path, "submitted"
		if fs.bool("blocked") {
			m.State = "blocked"
		}
		m.Revision++
		if err := store.SaveRun(run); err != nil {
			return err
		}
		fmt.Printf("member: %s\nstate: %s\nturn: %d\nreport: %s\n", m.Name, m.State, m.Turn, path)
		return nil
	})
	if err != nil {
		return fail(err)
	}
	return 0
}
