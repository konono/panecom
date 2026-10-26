package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPickPaneSelectsNextAndCanCancel(t *testing.T) {
	panes := []Pane{
		{ID: "p1", TabName: "app", Title: "claude", Cwd: "/app"},
		{ID: "p2", TabName: "mcp", Title: "codex", Cwd: "/mcp"},
	}
	var out bytes.Buffer
	selected, ok, err := pickPane(panes, "p1", strings.NewReader("\x1b[B\r"), &out, 80, 24, func(p Pane) (string, error) {
		return "screen of " + p.ID, nil
	})
	if err != nil || !ok || selected.ID != "p2" {
		t.Fatalf("selected=%+v ok=%v err=%v", selected, ok, err)
	}
	if !strings.Contains(out.String(), "screen of p2") || !strings.Contains(out.String(), "mcp") {
		t.Fatalf("selected pane details missing from picker: %q", out.String())
	}
	_, ok, err = pickPane(panes, "p1", strings.NewReader("q"), &out, 80, 24, nil)
	if err != nil || ok {
		t.Fatalf("cancel: ok=%v err=%v", ok, err)
	}
}

func TestRegisterSelectedPaneUsesTargetsProjectAndStore(t *testing.T) {
	a, b, f := twoProjects(t)
	// The selected pane has never run panecom. It is in a subdirectory of B.
	sub := filepath.Join(b, "sub")
	if err := os.MkdirAll(sub, 0755); err != nil {
		t.Fatal(err)
	}
	store := filepath.Join(t.TempDir(), "state")
	t.Setenv("PANECOM_STATE_DIR", store)
	inDir(t, a, func() {
		anchor, err := registerSelectedPane("m", Pane{ID: "pc", Cwd: sub}, "fake")
		if err != nil {
			t.Fatal(err)
		}
		if anchor != b {
			t.Fatalf("anchor=%s, want %s", anchor, b)
		}
		if _, err := resolveRole(namespaceDir(sessionRootIn(store, "fake", "fake"), a), "m"); err == nil {
			t.Error("remote role appeared in caller's namespace")
		}
		if got, err := resolveRole(namespaceDir(sessionRootIn(store, "fake", "fake"), b), "m"); err != nil || got != "pc" {
			t.Fatalf("remote role=%s err=%v", got, err)
		}
	})
	if len(f.sentText) != 0 || f.enters != 0 {
		t.Fatalf("registration typed into the target pane: sends=%v enters=%d", f.sentText, f.enters)
	}
	f.dead = map[string]bool{"gone": true}
	if _, err := registerSelectedPane("dead", Pane{ID: "gone", Cwd: b}, "fake"); err == nil {
		t.Error("closed pane was registered")
	}
}
