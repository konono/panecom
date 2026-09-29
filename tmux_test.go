package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTmuxSessionName(t *testing.T) {
	t.Setenv("TMUX", "/tmp/tmux-501/default,12345,0")
	r := newFakeRunner()
	r.Stub([]byte("my-session\n"), "tmux", "display-message", "-p", "#{session_name}")

	tm := &TmuxMux{runner: r}
	name, err := tm.SessionName()
	if err != nil {
		t.Fatal(err)
	}
	if name != "my-session" {
		t.Errorf("expected my-session, got %s", name)
	}
}

func TestTmuxSessionNameMissing(t *testing.T) {
	t.Setenv("TMUX", "")
	tm := &TmuxMux{runner: newFakeRunner()}
	_, err := tm.SessionName()
	if err == nil {
		t.Error("expected error when TMUX not set")
	}
}

func TestTmuxCurrentPaneID(t *testing.T) {
	t.Setenv("TMUX_PANE", "%3")
	tm := &TmuxMux{runner: newFakeRunner()}
	id, err := tm.CurrentPaneID()
	if err != nil {
		t.Fatal(err)
	}
	if id != "%3" {
		t.Errorf("expected %%3, got %s", id)
	}
}

func TestTmuxCurrentPaneIDMissing(t *testing.T) {
	t.Setenv("TMUX_PANE", "")
	tm := &TmuxMux{runner: newFakeRunner()}
	_, err := tm.CurrentPaneID()
	if err == nil {
		t.Error("expected error when TMUX_PANE not set")
	}
}

func TestTmuxListPanesCommand(t *testing.T) {
	t.Setenv("TMUX", "/tmp/tmux-501/default,12345,0")
	r := newFakeRunner()
	r.Stub([]byte("my-session\n"), "tmux", "display-message", "-p", "#{session_name}")
	r.Stub([]byte("%0\t@0\tshell\n%1\t@0\tshell\n%2\t@1\teditor\n"),
		"tmux", "list-panes", "-s", "-t", "my-session", "-F", "#{pane_id}\t#{window_id}\t#{window_name}")

	tm := &TmuxMux{runner: r}
	panes, err := tm.ListPanes("my-session")
	if err != nil {
		t.Fatal(err)
	}
	if len(panes) != 3 {
		t.Fatalf("expected 3 panes, got %d", len(panes))
	}
	if panes[0].ID != "%0" || panes[0].TabID != "@0" || panes[0].TabName != "shell" {
		t.Errorf("pane 0 mismatch: %+v", panes[0])
	}
	if panes[2].TabID != "@1" {
		t.Errorf("pane 2 should be in window @1, got %s", panes[2].TabID)
	}

	call := r.findCall("tmux", "list-panes")
	if call == nil {
		t.Fatal("expected tmux list-panes call")
	}
	if !strings.Contains(strings.Join(call.Args, " "), "-s -t my-session") {
		t.Errorf("expected session-scoped list-panes, got args: %v", call.Args)
	}
}

func TestTmuxListPanesSessionScope(t *testing.T) {
	t.Setenv("TMUX", "/tmp/tmux-501/default,12345,0")
	r := newFakeRunner()
	r.Stub([]byte("my-session\n"), "tmux", "display-message", "-p", "#{session_name}")
	r.Stub([]byte("%5\t@2\twork\n"),
		"tmux", "list-panes", "-s", "-t", "my-session", "-F", "#{pane_id}\t#{window_id}\t#{window_name}")

	tm := &TmuxMux{runner: r}
	exists, err := tm.PaneExists("my-session", "%5")
	if err != nil {
		t.Fatal(err)
	}
	if !exists {
		t.Error("expected %5 to exist in session")
	}

	r.calls = nil
	r.Stub([]byte("%5\t@2\twork\n"),
		"tmux", "list-panes", "-s", "-t", "my-session", "-F", "#{pane_id}\t#{window_id}\t#{window_name}")
	exists, err = tm.PaneExists("my-session", "%99")
	if err != nil {
		t.Fatal(err)
	}
	if exists {
		t.Error("expected %99 to not exist — it's in another session")
	}
}

func TestTmuxDumpPaneCommand(t *testing.T) {
	r := newFakeRunner()
	r.Stub([]byte("line1\nline2\n"), "tmux", "capture-pane", "-t", "%3", "-p")
	r.Stub([]byte("full\ncontent\n"), "tmux", "capture-pane", "-t", "%3", "-p", "-S", "-")

	tm := &TmuxMux{runner: r}

	out, err := tm.DumpPane("%3", false)
	if err != nil {
		t.Fatal(err)
	}
	if out != "line1\nline2\n" {
		t.Errorf("unexpected visible dump: %q", out)
	}

	out, err = tm.DumpPane("%3", true)
	if err != nil {
		t.Fatal(err)
	}
	if out != "full\ncontent\n" {
		t.Errorf("unexpected full dump: %q", out)
	}
}

func TestTmuxSendKeysCommand(t *testing.T) {
	r := newFakeRunner()
	tm := &TmuxMux{runner: r}

	_ = tm.SendKeys("%3", "hello world")

	call := r.findCall("tmux", "send-keys")
	if call == nil {
		t.Fatal("expected tmux send-keys call")
	}
	expected := []string{"send-keys", "-t", "%3", "-l", "--", "hello world"}
	if strings.Join(call.Args, " ") != strings.Join(expected, " ") {
		t.Errorf("expected args %v, got %v", expected, call.Args)
	}
}

func TestTmuxSendKeysDashXNotInterpretedAsFlag(t *testing.T) {
	r := newFakeRunner()
	tm := &TmuxMux{runner: r}

	_ = tm.SendKeys("%3", "-X copy-mode")

	call := r.findCall("tmux", "send-keys")
	if call == nil {
		t.Fatal("expected tmux send-keys call")
	}
	expected := []string{"send-keys", "-t", "%3", "-l", "--", "-X copy-mode"}
	if strings.Join(call.Args, " ") != strings.Join(expected, " ") {
		t.Errorf("-X should be after -- separator, got args %v", call.Args)
	}
}

func TestTmuxSendKeysDashDashHelpNotInterpretedAsFlag(t *testing.T) {
	r := newFakeRunner()
	tm := &TmuxMux{runner: r}

	_ = tm.SendKeys("%3", "--help")

	call := r.findCall("tmux", "send-keys")
	if call == nil {
		t.Fatal("expected tmux send-keys call")
	}
	expected := []string{"send-keys", "-t", "%3", "-l", "--", "--help"}
	if strings.Join(call.Args, " ") != strings.Join(expected, " ") {
		t.Errorf("--help should be after -- separator, got args %v", call.Args)
	}
}

func TestTmuxSendEnterCommand(t *testing.T) {
	r := newFakeRunner()
	tm := &TmuxMux{runner: r}

	_ = tm.SendEnter("%3")

	call := r.findCall("tmux", "send-keys")
	if call == nil {
		t.Fatal("expected tmux send-keys call for Enter")
	}
	expected := []string{"send-keys", "-t", "%3", "Enter"}
	if strings.Join(call.Args, " ") != strings.Join(expected, " ") {
		t.Errorf("expected args %v, got %v", expected, call.Args)
	}
}

func TestTmuxRenamePaneCommand(t *testing.T) {
	r := newFakeRunner()
	tm := &TmuxMux{runner: r}

	_ = tm.RenamePane("%3", "panecom:reviewer")

	call := r.findCall("tmux", "select-pane")
	if call == nil {
		t.Fatal("expected tmux select-pane call")
	}
	expected := []string{"select-pane", "-t", "%3", "-T", "panecom:reviewer"}
	if strings.Join(call.Args, " ") != strings.Join(expected, " ") {
		t.Errorf("expected args %v, got %v", expected, call.Args)
	}
}

func TestTmuxFocusPaneCommand(t *testing.T) {
	r := newFakeRunner()
	tm := &TmuxMux{runner: r}

	_ = tm.FocusPane("%3")

	call := r.findCall("tmux", "switch-client")
	if call == nil {
		t.Fatal("expected tmux switch-client call (not select-pane)")
	}
	expected := []string{"switch-client", "-t", "%3"}
	if strings.Join(call.Args, " ") != strings.Join(expected, " ") {
		t.Errorf("expected args %v, got %v", expected, call.Args)
	}
}

func TestTmuxNewPaneCommand(t *testing.T) {
	r := newFakeRunner()
	r.Stub([]byte("%5\n"), "tmux", "split-window", "-t", "%3", "-c", "/work", "-P", "-F", "#{pane_id}", "-h")

	tm := &TmuxMux{runner: r}
	paneID, err := tm.NewPane("%3", "/work", "right")
	if err != nil {
		t.Fatal(err)
	}
	if paneID != "%5" {
		t.Errorf("expected %%5, got %s", paneID)
	}

	call := r.findCall("tmux", "split-window")
	if call == nil {
		t.Fatal("expected tmux split-window call")
	}
	if !strings.Contains(strings.Join(call.Args, " "), "-t %3") {
		t.Errorf("expected explicit target pane, got args: %v", call.Args)
	}
}

func TestTmuxNewPaneDownDirection(t *testing.T) {
	r := newFakeRunner()
	r.Stub([]byte("%6\n"), "tmux", "split-window", "-t", "%3", "-c", "/work", "-P", "-F", "#{pane_id}", "-v")

	tm := &TmuxMux{runner: r}
	paneID, err := tm.NewPane("%3", "/work", "down")
	if err != nil {
		t.Fatal(err)
	}
	if paneID != "%6" {
		t.Errorf("expected %%6, got %s", paneID)
	}
}

func TestTmuxNewTabCommand(t *testing.T) {
	r := newFakeRunner()
	r.Stub([]byte("@3\t%10\n"), "tmux", "new-window", "-n", "dev", "-c", "/work", "-P", "-F", "#{window_id}\t#{pane_id}")

	tm := &TmuxMux{runner: r}
	tabID, initialPaneID, err := tm.NewTab("dev", "/work")
	if err != nil {
		t.Fatal(err)
	}
	if tabID != "@3" {
		t.Errorf("expected @3, got %s", tabID)
	}
	if initialPaneID != "%10" {
		t.Errorf("expected %%10, got %s", initialPaneID)
	}
}

func TestTmuxNewPaneInTabCommand(t *testing.T) {
	r := newFakeRunner()
	r.Stub([]byte("%11\n"), "tmux", "split-window", "-t", "@3", "-c", "/work", "-P", "-F", "#{pane_id}", "-h")

	tm := &TmuxMux{runner: r}
	paneID, err := tm.NewPaneInTab("@3", "/work", "right")
	if err != nil {
		t.Fatal(err)
	}
	if paneID != "%11" {
		t.Errorf("expected %%11, got %s", paneID)
	}
}

func TestTmuxOpenRegistersWithMuxKind(t *testing.T) {
	r := newFakeRunner()
	t.Setenv("TMUX", "/tmp/tmux-501/default,12345,0")
	t.Setenv("TMUX_PANE", "%3")
	t.Setenv("ZELLIJ_SESSION_NAME", "")
	t.Setenv("ZELLIJ_PANE_ID", "")
	stateDir := t.TempDir()
	t.Setenv("PANECOM_STATE_DIR", stateDir)

	r.Stub([]byte("my-session\n"), "tmux", "display-message", "-p", "#{session_name}")

	cwd := canonicalCwd()
	sRoot := sessionRoot("tmux", "my-session")
	cwdHash := hashString(cwd)
	nsDir := namespaceDir(sRoot, cwd)

	_ = atomicWrite(filepath.Join(sRoot, ".session"), "my-session")
	_ = atomicWrite(filepath.Join(nsDir, ".cwd"), cwd)
	_ = atomicWrite(filepath.Join(nsDir, "roles", "implementer"), "%3")
	_ = atomicWrite(filepath.Join(sRoot, "panes", "%3"), cwdHash)

	r.Stub([]byte("%10\n"), "tmux", "split-window", "-t", "%3", "-c", cwd, "-P", "-F", "#{pane_id}", "-h")

	tm := &TmuxMux{runner: r}
	mux = tm

	cmdOpen("newrole", "", "")

	sendCall := r.findCall("tmux", "send-keys")
	if sendCall == nil {
		t.Fatal("expected tmux send-keys call from cmdOpen, but none found")
	}
	cmdStr := strings.Join(sendCall.Args, " ")
	if !strings.Contains(cmdStr, "PANECOM_MUX=tmux") {
		t.Errorf("expected PANECOM_MUX=tmux in register command, got: %s", cmdStr)
	}
	if !strings.Contains(cmdStr, "panecom register newrole") {
		t.Errorf("expected 'panecom register newrole' in command, got: %s", cmdStr)
	}
}

var _ = os.PathSeparator
