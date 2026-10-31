package store

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestRunRoundTripAndLockedUpdates(t *testing.T) {
	t.Setenv("WEECO_HOME", t.TempDir())
	id, err := NewID("r-")
	if err != nil {
		t.Fatal(err)
	}
	run := &Run{ID: id, LeadPane: "w1:p1", Members: map[string]*Member{"writer": {Name: "writer", Turn: 1}}}
	if err := SaveRun(run); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := RuntimeLock(func() error {
				r, e := LoadRun(id)
				if e != nil {
					return e
				}
				r.Members["writer"].Revision++
				return SaveRun(r)
			}); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	got, err := LoadRun(id)
	if err != nil {
		t.Fatal(err)
	}
	if got.Members["writer"].Revision != 12 {
		t.Fatalf("lost update: %+v", got)
	}
	for _, invalid := range []string{"../other", "/tmp/run", "", "a/b"} {
		if _, err := LoadRun(invalid); err == nil {
			t.Fatalf("unsafe id accepted: %q", invalid)
		}
	}
}

func TestCorruptRunStopsContextResolution(t *testing.T) {
	t.Setenv("WEECO_HOME", t.TempDir())
	path := filepath.Join(RunDir("r-corrupt"), "run.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"id":"r-other","lead_pane":"p","members":{}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Runs(); err == nil {
		t.Fatal("corrupt identity silently ignored")
	}
	if err := os.WriteFile(path, []byte(fmt.Sprintf(`{"id":%q,"lead_pane":"p","members":{"a":null}}`, "r-corrupt")), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadRun("r-corrupt"); err == nil {
		t.Fatal("null member accepted")
	}
}
