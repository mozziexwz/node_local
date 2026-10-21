package relays

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLabelsPersist(t *testing.T) {
	path := filepath.Join(t.TempDir(), "relays.json")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Add(Label{ID: "a1r", ServerID: "la", Kind: Friend, Remark: "小明", Target: "192.0.2.77:30512", Entry: "198.51.100.20:41877", CreatedAt: 2},
		Label{ID: "b2r", ServerID: "la", Kind: Self, Remark: "自备中转", CreatedAt: 1},
		Label{ID: "b2f", ServerID: "front", Kind: Front, CreatedAt: 1}); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := reopened.ForServer("la"); len(got) != 2 || got[0].ID != "b2r" || got[1].Remark != "小明" {
		t.Fatalf("for server: %+v", got)
	}
	if _, ok := reopened.Find("la", "b2f"); ok {
		t.Fatal("a label from another server matched")
	}
	if err := reopened.Remove("la", "a1r"); err != nil {
		t.Fatal(err)
	}
	if _, ok := reopened.Find("la", "a1r"); ok {
		t.Fatal("not removed")
	}
	if err := reopened.RemoveServer("front"); err != nil {
		t.Fatal(err)
	}
	if len(reopened.ForServer("front")) != 0 || len(reopened.ForServer("la")) != 1 {
		t.Fatal("RemoveServer removed the wrong labels")
	}
	// Adding the same relay again replaces its label.
	reopened.Add(Label{ID: "b2r", ServerID: "la", Kind: Self, Remark: "改名"})
	if got := reopened.ForServer("la"); len(got) != 1 || got[0].Remark != "改名" {
		t.Fatalf("replace: %+v", got)
	}
}

func TestBrokenFileStartsEmpty(t *testing.T) {
	path := filepath.Join(t.TempDir(), "relays.json")
	os.WriteFile(path, []byte("{oops"), 0o644)
	s, err := Open(path)
	if err == nil {
		t.Fatal("a broken file was not reported")
	}
	if err := s.Add(Label{ID: "x", ServerID: "y"}); err != nil {
		t.Fatal(err)
	}
	if _, ok := s.Find("y", "x"); !ok {
		t.Fatal("cannot record after a broken file")
	}
}
