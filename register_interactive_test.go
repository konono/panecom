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
	var highlighted []string
	selected, ok, err := pickPane(panes, "p1", strings.NewReader("\x1b[B\r"), &out, 80, 24, func(p Pane) (string, error) {
		return "screen of " + p.ID, nil
	}, func(p Pane) error {
		highlighted = append(highlighted, p.ID)
		return nil
	})
	if err != nil || !ok || selected.ID != "p2" {
		t.Fatalf("selected=%+v ok=%v err=%v", selected, ok, err)
	}
	if !strings.Contains(out.String(), "screen of p2") || !strings.Contains(out.String(), "mcp") {
		t.Fatalf("selected pane details missing from picker: %q", out.String())
	}
	if strings.Join(highlighted, ",") != "p1,p2" {
		t.Fatalf("highlighted panes = %v", highlighted)
	}
	_, ok, err = pickPane(panes, "p1", strings.NewReader("q"), &out, 80, 24, nil, nil)
	if err != nil || ok {
		t.Fatalf("cancel: ok=%v err=%v", ok, err)
	}
	_, ok, err = pickPane(panes, "p1", strings.NewReader("\x1b"), &out, 80, 24, nil, nil)
	if err != nil || ok {
		t.Fatalf("Esc cancel: ok=%v err=%v", ok, err)
	}
}

func TestSpatialNavigationUsesLayoutNotListOrder(t *testing.T) {
	// Deliberately ordered bottom-right, top-left, bottom-left, top-right.
	panes := []Pane{
		{ID: "br", TabID: "1", X: 51, Y: 11, Width: 49, Height: 9},
		{ID: "tl", TabID: "1", X: 0, Y: 0, Width: 50, Height: 10},
		{ID: "bl", TabID: "1", X: 0, Y: 11, Width: 50, Height: 9},
		{ID: "tr", TabID: "1", X: 51, Y: 0, Width: 49, Height: 10},
		{ID: "other-tab", TabID: "2", X: 0, Y: 0, Width: 100, Height: 20},
	}
	if got := panes[spatialNeighbor(panes, 2, 'U')].ID; got != "tl" {
		t.Errorf("up from bottom-left = %s", got)
	}
	if got := panes[spatialNeighbor(panes, 2, 'R')].ID; got != "br" {
		t.Errorf("right from bottom-left = %s", got)
	}
	if got := panes[spatialNeighbor(panes, 2, 'D')].ID; got != "bl" {
		t.Errorf("down at edge wrapped to %s", got)
	}
	if got := panes[cycleTab(panes, 2, 1)].ID; got != "other-tab" {
		t.Errorf("Tab selected %s", got)
	}
	var out bytes.Buffer
	selected, ok, err := pickPane(panes, "bl", strings.NewReader("\x1b[1;2A\x1b[C\r"), &out, 100, 24, nil, nil)
	if err != nil || !ok || selected.ID != "tr" {
		t.Fatalf("modified Up then Right: selected=%+v ok=%v err=%v", selected, ok, err)
	}
	if strings.Contains(out.String(), "other-tab") {
		t.Error("other tab pane appeared in the current tab list")
	}
	first := renderPanePicker(panes, 2, "", 100, 24)
	if strings.Index(first, "tl") > strings.Index(first, "br") {
		t.Error("pane list is not ordered by screen position")
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
