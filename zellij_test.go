package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type fakeRunner struct {
	calls []fakeCall
	stubs map[string]fakeStub
}

type fakeCall struct {
	Name string
	Args []string
}

type fakeStub struct {
	Output []byte
	Err    error
}

func newFakeRunner() *fakeRunner {
	return &fakeRunner{stubs: make(map[string]fakeStub)}
}

func (f *fakeRunner) stubKey(name string, args ...string) string {
	return name + " " + strings.Join(args, " ")
}

func (f *fakeRunner) Stub(output []byte, name string, args ...string) {
	f.stubs[f.stubKey(name, args...)] = fakeStub{Output: output}
}

func (f *fakeRunner) Output(name string, args ...string) ([]byte, error) {
	f.calls = append(f.calls, fakeCall{Name: name, Args: args})
	key := f.stubKey(name, args...)
	if stub, ok := f.stubs[key]; ok {
		return stub.Output, stub.Err
	}
	return []byte{}, nil
}

func (f *fakeRunner) Run(name string, args ...string) error {
	f.calls = append(f.calls, fakeCall{Name: name, Args: args})
	key := f.stubKey(name, args...)
	if stub, ok := f.stubs[key]; ok {
		return stub.Err
	}
	return nil
}

func (f *fakeRunner) findCall(name string, argSubstring string) *fakeCall {
	for _, c := range f.calls {
		if c.Name == name && strings.Contains(strings.Join(c.Args, " "), argSubstring) {
			return &c
		}
	}
	return nil
}

func TestZellijSessionName(t *testing.T) {
	t.Setenv("ZELLIJ_SESSION_NAME", "test-session")
	z := &ZellijMux{runner: newFakeRunner()}
	name, err := z.SessionName()
	if err != nil {
		t.Fatal(err)
	}
	if name != "test-session" {
		t.Errorf("expected test-session, got %s", name)
	}
}

func TestZellijSessionNameMissing(t *testing.T) {
	t.Setenv("ZELLIJ_SESSION_NAME", "")
	z := &ZellijMux{runner: newFakeRunner()}
	_, err := z.SessionName()
	if err == nil {
		t.Error("expected error when ZELLIJ_SESSION_NAME not set")
	}
}

func TestZellijCurrentPaneID(t *testing.T) {
	t.Setenv("ZELLIJ_PANE_ID", "7")
	z := &ZellijMux{runner: newFakeRunner()}
	id, err := z.CurrentPaneID()
	if err != nil {
		t.Fatal(err)
	}
	if id != "terminal_7" {
		t.Errorf("expected terminal_7, got %s", id)
	}
}

func TestZellijListPanesCommand(t *testing.T) {
	r := newFakeRunner()
	r.Stub([]byte(`[{"id":1,"is_plugin":false,"tab_id":0,"tab_name":"Tab #1"},{"id":2,"is_plugin":true,"tab_id":0,"tab_name":"Tab #1"}]`),
		"zellij", "action", "list-panes", "--json", "-t")

	z := &ZellijMux{runner: r}
	panes, err := z.ListPanes("")
	if err != nil {
		t.Fatal(err)
	}
	if len(panes) != 1 {
		t.Fatalf("expected 1 terminal pane, got %d", len(panes))
	}
	if panes[0].ID != "terminal_1" {
		t.Errorf("expected terminal_1, got %s", panes[0].ID)
	}
	if panes[0].TabID != "0" {
		t.Errorf("expected tab_id '0', got '%s'", panes[0].TabID)
	}

	call := r.findCall("zellij", "list-panes")
	if call == nil {
		t.Fatal("expected zellij action list-panes call")
	}
	expected := []string{"action", "list-panes", "--json", "-t"}
	if strings.Join(call.Args, " ") != strings.Join(expected, " ") {
		t.Errorf("expected args %v, got %v", expected, call.Args)
	}
}

func TestZellijDumpPaneCommand(t *testing.T) {
	r := newFakeRunner()
	r.Stub([]byte("screen content\n"), "zellij", "action", "dump-screen", "--pane-id", "terminal_5")
	r.Stub([]byte("full screen content\n"), "zellij", "action", "dump-screen", "--pane-id", "terminal_5", "--full")

	z := &ZellijMux{runner: r}

	out, err := z.DumpPane("terminal_5", false)
	if err != nil {
		t.Fatal(err)
	}
	if out != "screen content\n" {
		t.Errorf("unexpected output: %q", out)
	}

	out, err = z.DumpPane("terminal_5", true)
	if err != nil {
		t.Fatal(err)
	}
	if out != "full screen content\n" {
		t.Errorf("unexpected output: %q", out)
	}
}

func TestZellijSendKeysCommand(t *testing.T) {
	r := newFakeRunner()
	z := &ZellijMux{runner: r}

	_ = z.SendKeys("terminal_3", "hello world")

	call := r.findCall("zellij", "write-chars")
	if call == nil {
		t.Fatal("expected zellij action write-chars call")
	}
	expected := []string{"action", "write-chars", "--pane-id", "terminal_3", "hello world"}
	if strings.Join(call.Args, " ") != strings.Join(expected, " ") {
		t.Errorf("expected args %v, got %v", expected, call.Args)
	}
}

func TestZellijSendEnterCommand(t *testing.T) {
	r := newFakeRunner()
	z := &ZellijMux{runner: r}

	_ = z.SendEnter("terminal_3")

	call := r.findCall("zellij", "write")
	if call == nil {
		t.Fatal("expected zellij action write call")
	}
	expected := []string{"action", "write", "--pane-id", "terminal_3", "13"}
	if strings.Join(call.Args, " ") != strings.Join(expected, " ") {
		t.Errorf("expected args %v, got %v", expected, call.Args)
	}
}

func TestZellijRenamePaneCommand(t *testing.T) {
	r := newFakeRunner()
	z := &ZellijMux{runner: r}

	_ = z.RenamePane("terminal_3", "panecom:reviewer")

	call := r.findCall("zellij", "rename-pane")
	if call == nil {
		t.Fatal("expected zellij action rename-pane call")
	}
	expected := []string{"action", "rename-pane", "--pane-id", "terminal_3", "panecom:reviewer"}
	if strings.Join(call.Args, " ") != strings.Join(expected, " ") {
		t.Errorf("expected args %v, got %v", expected, call.Args)
	}
}

func TestZellijFocusPaneCommand(t *testing.T) {
	r := newFakeRunner()
	z := &ZellijMux{runner: r}

	_ = z.FocusPane("terminal_3")

	call := r.findCall("zellij", "focus-pane-id")
	if call == nil {
		t.Fatal("expected zellij action focus-pane-id call")
	}
	expected := []string{"action", "focus-pane-id", "terminal_3"}
	if strings.Join(call.Args, " ") != strings.Join(expected, " ") {
		t.Errorf("expected args %v, got %v", expected, call.Args)
	}
}

func TestZellijNewPaneCommand(t *testing.T) {
	r := newFakeRunner()
	r.Stub([]byte("terminal_10\n"), "zellij", "action", "new-pane", "--near-current-pane", "--cwd", "/work", "--direction", "down")

	z := &ZellijMux{runner: r}
	paneID, err := z.NewPane("terminal_1", "/work", "down")
	if err != nil {
		t.Fatal(err)
	}
	if paneID != "terminal_10" {
		t.Errorf("expected terminal_10, got %s", paneID)
	}
}

func TestZellijNewPaneInTabCommand(t *testing.T) {
	r := newFakeRunner()
	r.Stub([]byte("terminal_11\n"), "zellij", "action", "new-pane", "--direction", "right", "--cwd", "/work", "--tab-id", "2")

	z := &ZellijMux{runner: r}
	paneID, err := z.NewPaneInTab("2", "/work", "right")
	if err != nil {
		t.Fatal(err)
	}
	if paneID != "terminal_11" {
		t.Errorf("expected terminal_11, got %s", paneID)
	}
}

func TestZellijPaneExists(t *testing.T) {
	r := newFakeRunner()
	r.Stub([]byte(`[{"id":5,"is_plugin":false,"tab_id":0,"tab_name":"Tab"},{"id":7,"is_plugin":false,"tab_id":0,"tab_name":"Tab"}]`),
		"zellij", "action", "list-panes", "--json", "-t")

	z := &ZellijMux{runner: r}
	exists, err := z.PaneExists("", "terminal_5")
	if err != nil {
		t.Fatal(err)
	}
	if !exists {
		t.Error("expected terminal_5 to exist")
	}

	r.calls = nil
	r.Stub([]byte(`[{"id":5,"is_plugin":false,"tab_id":0,"tab_name":"Tab"}]`),
		"zellij", "action", "list-panes", "--json", "-t")
	exists, err = z.PaneExists("", "terminal_99")
	if err != nil {
		t.Fatal(err)
	}
	if exists {
		t.Error("expected terminal_99 to not exist")
	}
}

func TestZellijNewTabCommand(t *testing.T) {
	r := newFakeRunner()

	r.Stub([]byte(`[{"id":1,"is_plugin":false,"tab_id":0,"tab_name":"Tab #1"}]`),
		"zellij", "action", "list-panes", "--json", "-t")

	r.Stub([]byte("1\n"), "zellij", "action", "new-tab", "--name", "dev", "--cwd", "/work")

	callCount := 0
	origStub := r.stubs[r.stubKey("zellij", "action", "list-panes", "--json", "-t")]
	r.stubs[r.stubKey("zellij", "action", "list-panes", "--json", "-t")] = fakeStub{}
	delete(r.stubs, r.stubKey("zellij", "action", "list-panes", "--json", "-t"))

	r2 := &fakeRunnerWithListPanesCounting{
		inner:     r,
		callCount: &callCount,
		responses: [][]byte{
			[]byte(`[{"id":1,"is_plugin":false,"tab_id":0,"tab_name":"Tab #1"}]`),
			[]byte(`[{"id":1,"is_plugin":false,"tab_id":0,"tab_name":"Tab #1"},{"id":2,"is_plugin":false,"tab_id":1,"tab_name":"dev"}]`),
		},
	}
	_ = origStub

	z2 := &ZellijMux{runner: r2}
	tabID, initialPaneID, err := z2.NewTab("dev", "/work")
	if err != nil {
		t.Fatal(err)
	}
	if tabID != "1" {
		t.Errorf("expected tab ID '1', got '%s'", tabID)
	}
	if initialPaneID != "terminal_2" {
		t.Errorf("expected initial pane 'terminal_2', got '%s'", initialPaneID)
	}
}

type fakeRunnerWithListPanesCounting struct {
	inner     *fakeRunner
	callCount *int
	responses [][]byte
}

func (f *fakeRunnerWithListPanesCounting) Output(name string, args ...string) ([]byte, error) {
	if name == "zellij" && len(args) >= 2 && args[1] == "list-panes" {
		idx := *f.callCount
		*f.callCount++
		if idx < len(f.responses) {
			return f.responses[idx], nil
		}
		return []byte("[]"), nil
	}
	return f.inner.Output(name, args...)
}

func (f *fakeRunnerWithListPanesCounting) Run(name string, args ...string) error {
	return f.inner.Run(name, args...)
}

func TestZellijOpenRegistersWithMuxKind(t *testing.T) {
	r := newFakeRunner()
	t.Setenv("ZELLIJ_SESSION_NAME", "test-session")
	t.Setenv("ZELLIJ_PANE_ID", "3")
	stateDir := t.TempDir()
	t.Setenv("PANECOM_STATE_DIR", stateDir)

	session := "test-session"
	sRoot := sessionRoot("zellij", session)
	cwd := canonicalCwd()
	cwdHash := hashString(cwd)
	nsDir := namespaceDir(sRoot, cwd)

	_ = atomicWrite(filepath.Join(sRoot, ".session"), session)
	_ = atomicWrite(filepath.Join(nsDir, ".cwd"), cwd)
	_ = atomicWrite(filepath.Join(nsDir, "roles", "implementer"), "terminal_3")
	_ = atomicWrite(filepath.Join(sRoot, "panes", "terminal_3"), cwdHash)

	r.Stub([]byte("terminal_10\n"), "zellij", "action", "new-pane", "--near-current-pane", "--cwd", cwd)

	z := &ZellijMux{runner: r}
	mux = z

	cmdOpen("newrole", "", "")

	sendCall := r.findCall("zellij", "write-chars")
	if sendCall == nil {
		t.Fatal("expected zellij write-chars call from cmdOpen, but none found")
	}
	cmdStr := strings.Join(sendCall.Args, " ")
	if !strings.Contains(cmdStr, "PANECOM_MUX=zellij") {
		t.Errorf("expected PANECOM_MUX=zellij in register command, got: %s", cmdStr)
	}
	if !strings.Contains(cmdStr, "panecom register newrole") {
		t.Errorf("expected 'panecom register newrole' in command, got: %s", cmdStr)
	}
}

func TestZellijProfileRegistersWithMuxKind(t *testing.T) {
	stateDir := t.TempDir()
	t.Setenv("PANECOM_STATE_DIR", stateDir)
	t.Setenv("ZELLIJ_SESSION_NAME", "test-session")
	t.Setenv("ZELLIJ_PANE_ID", "1")

	configDir := filepath.Join(stateDir, "project", ".panecom")
	_ = os.MkdirAll(configDir, 0755)
	_ = os.WriteFile(filepath.Join(configDir, "config.yaml"), []byte(`
profiles:
  test:
    panes:
      - role: developer
      - role: terminal
        direction: down
`), 0644)

	origDir, _ := os.Getwd()
	_ = os.Chdir(filepath.Join(stateDir, "project"))
	defer func() { _ = os.Chdir(origDir) }()

	callCount := 0
	r := &fakeRunnerWithListPanesCounting{
		inner:     newFakeRunner(),
		callCount: &callCount,
		responses: [][]byte{
			[]byte(`[{"id":1,"is_plugin":false,"tab_id":0,"tab_name":"old"}]`),
			[]byte(`[{"id":1,"is_plugin":false,"tab_id":0,"tab_name":"old"},{"id":5,"is_plugin":false,"tab_id":1,"tab_name":"test"}]`),
		},
	}
	r.inner.Stub([]byte("1\n"), "zellij", "action", "new-tab", "--name", "test", "--cwd", canonicalCwd())
	r.inner.Stub([]byte("terminal_6\n"), "zellij", "action", "new-pane", "--direction", "down", "--cwd", canonicalCwd(), "--tab-id", "1")

	z := &ZellijMux{runner: r}
	mux = z

	cmdProfile("test", "", false)

	var muxPropagated bool
	for _, c := range r.inner.calls {
		if c.Name == "zellij" && len(c.Args) > 0 && c.Args[0] == "action" {
			for _, arg := range c.Args {
				if strings.Contains(arg, "PANECOM_MUX=zellij") {
					muxPropagated = true
					break
				}
			}
		}
	}
	if !muxPropagated {
		t.Error("expected PANECOM_MUX=zellij propagation in profile register commands")
	}
}

var _ = os.PathSeparator
var _ = fmt.Sprint
var _ = filepath.Join
