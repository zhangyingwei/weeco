package commands

import (
	"fmt"
	"math"
	"strconv"
	"time"

	"weeco/internal/herdr"
	"weeco/internal/store"
)

type runtimeMemberView struct {
	*store.Member
	Live string `json:"live"`
}

type runtimeStatus struct {
	Run      string              `json:"run"`
	LeadPane string              `json:"lead_pane"`
	Closed   bool                `json:"closed"`
	Reason   string              `json:"reason"`
	Members  []runtimeMemberView `json:"members"`
}

func cmdStatus(args []string) int {
	fs, err := runtimeFlags(args, []string{"timeout"}, []string{"wait", "json"}, 1)
	if err != nil {
		return fail(err)
	}
	if helpRequested(fs) {
		fmt.Println("usage: weeco status [ALIAS] [--wait --timeout SECONDS] [--json] [--run ID]")
		return 0
	}
	timeout := 120.0
	if text := fs.value("timeout"); text != "" {
		timeout, err = strconv.ParseFloat(text, 64)
		if err != nil || math.IsNaN(timeout) || math.IsInf(timeout, 0) || timeout <= 0 || timeout > 3600 {
			return fail(fmt.Errorf("--timeout must be between 0 and 3600 seconds"))
		}
	}
	ctx, err := currentRuntime()
	if err != nil {
		return fail(err)
	}
	deadline := time.Now().Add(time.Duration(timeout * float64(time.Second)))
	for {
		out := runtimeStatus{Members: []runtimeMemberView{}}
		ready := !fs.bool("wait")
		err := store.RuntimeLock(func() error {
			run, alias, err := resolveRun(ctx, fs.value("run"))
			if err != nil {
				return err
			}
			out.Run, out.LeadPane, out.Closed = run.ID, run.LeadPane, run.Closed
			agents, err := herdr.List("agent", "")
			if err != nil {
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
			changed := false
			for _, name := range names {
				m := run.Members[name]
				live := "missing"
				a := herdr.ByPane(agents, m.PaneID)
				if m.State == "closed" {
					live = "closed"
				} else if a != nil {
					live = herdr.String(a, "agent_status")
					if herdr.Session(a) != m.Session {
						live = "replaced"
					}
				}
				out.Members = append(out.Members, runtimeMemberView{Member: m, Live: live})
				attention := live == "missing" || live == "replaced" || live == "blocked"
				if m.Revision > m.Seen || (attention && live != m.SeenLive) {
					ready = true
					if fs.bool("wait") && alias == "lead" {
						m.Seen, m.SeenLive = m.Revision, live
						changed = true
					}
				}
				if !attention && m.SeenLive != "" && fs.bool("wait") && alias == "lead" {
					m.SeenLive = ""
					changed = true
				}
			}
			if run.Closed {
				ready = true
				out.Reason = "closed"
			}
			if changed {
				return store.SaveRun(run)
			}
			return nil
		})
		if err != nil {
			return fail(err)
		}
		if !ready && !time.Now().Before(deadline) {
			ready = true
			out.Reason = "timeout"
		}
		if ready {
			if out.Reason == "" {
				if fs.bool("wait") {
					out.Reason = "attention"
				} else {
					out.Reason = "snapshot"
				}
			}
			if fs.bool("json") {
				printJSON(out)
			} else {
				fmt.Printf("run: %s\nlead: %s\nreason: %s\n", out.Run, out.LeadPane, out.Reason)
				for _, v := range out.Members {
					fmt.Printf("%-16s state=%-10s live=%-10s turn=%d\n", v.Name, v.State, v.Live, v.Turn)
					if v.Report != "" {
						fmt.Printf("  report: %s\n", v.Report)
					}
					if v.LastError != "" {
						fmt.Printf("  warning: %s\n", v.LastError)
					}
				}
			}
			return 0
		}
		pause := time.Until(deadline)
		if pause > 500*time.Millisecond {
			pause = 500 * time.Millisecond
		}
		if pause > 0 {
			time.Sleep(pause)
		}
	}
}
