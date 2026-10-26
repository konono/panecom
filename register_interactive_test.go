package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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

func TestPromptRoleCompletion(t *testing.T) {
	tests := []struct {
		name, input, want string
		ok                bool
	}{
		{"first suggestion", "\t\r", "developer", true},
		{"cycle suggestion", "\t\t\r", "reviewer", true},
		{"prefix", "rev\t\r", "reviewer", true},
		{"backspace", "reviewx\x7fer\t\r", "reviewer", true},
		{"custom role", "architect\r", "architect", true},
		{"left arrow edits in place", "revieer\x1b[D\x1b[Dw\r", "reviewer", true},
		{"up arrow keeps prompt", "rev\x1b[A\t\r", "reviewer", true},
		{"cancel", "\x1b", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out bytes.Buffer
			got, ok, err := promptRole(strings.NewReader(tt.input), &out)
			if err != nil || ok != tt.ok || got != tt.want {
				t.Fatalf("role=%q ok=%v err=%v, want %q %v", got, ok, err, tt.want, tt.ok)
			}
			if !strings.Contains(out.String(), "Role name") {
				t.Errorf("missing prompt: %q", out.String())
			}
		})
	}
}

func TestPickPaneDelayedArrowSequence(t *testing.T) {
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = reader.Close() }()
	defer func() { _ = writer.Close() }()
	go func() {
		_, _ = writer.Write([]byte{27})
		time.Sleep(250 * time.Millisecond)
		_, _ = writer.Write([]byte("[B\r"))
	}()
	panes := []Pane{{ID: "p1"}, {ID: "p2"}}
	var out bytes.Buffer
	selected, ok, err := pickPane(panes, "p1", reader, &out, 80, 24, nil, nil)
	if err != nil || !ok || selected.ID != "p2" {
		t.Fatalf("delayed arrow: selected=%+v ok=%v err=%v", selected, ok, err)
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
	local, err := panesInCurrentTab(panes, "bl")
	if err != nil || len(local) != 4 {
		t.Fatalf("current tab panes=%v err=%v", local, err)
	}
	for _, pane := range local {
		if pane.TabID != "1" {
			t.Fatalf("other tab pane leaked into picker: %+v", pane)
		}
	}
	if _, err := panesInCurrentTab(panes, "missing"); err == nil {
		t.Error("missing current pane should fail")
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
