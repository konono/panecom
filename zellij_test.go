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
	r.Stub([]byte(`[{"id":1,"is_plugin":false,"tab_id":0,"tab_name":"Tab #1","title":"agent","pane_cwd":"/work/repo","pane_command":"codex","pane_x":10,"pane_y":2,"pane_columns":40,"pane_rows":20},{"id":2,"is_plugin":true,"tab_id":0,"tab_name":"Tab #1"}]`),
		"zellij", "action", "list-panes", "--json", "--all")

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
	if panes[0].Title != "agent" || panes[0].Cwd != "/work/repo" || panes[0].Command != "codex" {
		t.Errorf("missing pane metadata: %+v", panes[0])
	}
	if panes[0].X != 10 || panes[0].Y != 2 || panes[0].Width != 40 || panes[0].Height != 20 {
		t.Errorf("missing pane geometry: %+v", panes[0])
	}

	call := r.findCall("zellij", "list-panes")
	if call == nil {
		t.Fatal("expected zellij action list-panes call")
	}
	expected := []string{"action", "list-panes", "--json", "--all"}
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
		"zellij", "action", "list-panes", "--json", "--all")

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
		"zellij", "action", "list-panes", "--json", "--all")
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
		"zellij", "action", "list-panes", "--json", "--all")

	r.Stub([]byte("1\n"), "zellij", "action", "new-tab", "--name", "dev", "--cwd", "/work")

	callCount := 0
	origStub := r.stubs[r.stubKey("zellij", "action", "list-panes", "--json", "--all")]
	r.stubs[r.stubKey("zellij", "action", "list-panes", "--json", "--all")] = fakeStub{}
	delete(r.stubs, r.stubKey("zellij", "action", "list-panes", "--json", "--all"))

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

func TestZellijListPanesIsCachedWithinOneCommand(t *testing.T) {
	r := newFakeRunner()
	r.Stub([]byte(`[{"id":1,"tab_id":0,"pane_command":"zsh"},{"id":2,"tab_id":0,"pane_command":"zsh"}]`),
		"zellij", "action", "list-panes", "--json", "--all")
	z := &ZellijMux{runner: r}

	// Checking several roles is the common case and must not pay one
	// multiplexer round trip per role.
	for _, id := range []string{"terminal_1", "terminal_2", "terminal_9"} {
		if _, err := z.PaneExists("s", id); err != nil {
			t.Fatal(err)
		}
	}
	if n := countCalls(r, "list-panes"); n != 1 {
		t.Errorf("list-panes called %d times, want 1", n)
	}

	// Creating a pane changes the set, so the next read must go out again.
	r.Stub([]byte("terminal_3\n"), "zellij", "action", "new-pane", "--near-current-pane", "--cwd", "/tmp", "--direction", "right")
	if _, err := z.NewPane("terminal_1", "/tmp", "right"); err != nil {
		t.Fatal(err)
	}
	if _, err := z.PaneExists("s", "terminal_1"); err != nil {
		t.Fatal(err)
	}
	if n := countCalls(r, "list-panes"); n != 2 {
		t.Errorf("list-panes called %d times after NewPane, want 2", n)
	}
}

func countCalls(r *fakeRunner, argSubstring string) int {
	n := 0
	for _, c := range r.calls {
		if strings.Contains(strings.Join(c.Args, " "), argSubstring) {
			n++
		}
	}
	return n
}

func TestZellijListSessionsSkipsExited(t *testing.T) {
	r := newFakeRunner()
	r.Stub([]byte("quiet-zebra [Created 25days ago] (EXITED - attach to resurrect)\n"+
		"ope [Created 7days ago] (current)\n"+
		"sparkling-bee [Created 1h ago]\n"),
		"zellij", "list-sessions", "--no-formatting")
	z := &ZellijMux{runner: r}

	got, err := z.ListSessions()
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"ope", "sparkling-bee"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("sessions = %v, want %v (exited sessions have no live panes)", got, want)
	}
}

func TestZellijListSessionsRejectsUnreadableOutput(t *testing.T) {
	r := newFakeRunner()
	r.Stub([]byte("\n"), "zellij", "list-sessions", "--no-formatting")
	z := &ZellijMux{runner: r}
	// We run inside a session, so "nothing is running" means the output was
	// not understood. Reporting none would invite deleting live state.
	if got, err := z.ListSessions(); err == nil {
		t.Errorf("want an error for unparseable output, got %v", got)
	}
}

// seedRealMuxSession lays out the state a registered role leaves behind, so a
// cleanup run against a real multiplexer implementation has something to lose.
func seedRealMuxSession(t *testing.T, kind, session, paneID string) string {
	t.Helper()
	sRoot := sessionRoot(kind, session)
	cwd := canonicalCwd()
	nsDir := namespaceDir(sRoot, cwd)
	for path, value := range map[string]string{
		filepath.Join(sRoot, ".session"):          session,
		filepath.Join(nsDir, ".cwd"):              cwd,
		filepath.Join(nsDir, "roles", "reviewer"): paneID,
		filepath.Join(sRoot, "panes", paneID):     hashString(cwd),
	} {
		if err := atomicWrite(path, value); err != nil {
			t.Fatal(err)
		}
	}
	return nsDir
}

// An empty or unreadable pane list used to come back as "no panes exist",
// which made every role look dead. cleanup acts on that judgement, so the
// whole namespace could be deleted because one query came back blank.
func TestZellijEmptyPaneListDoesNotRetireRoles(t *testing.T) {
	for _, tc := range []struct {
		name   string
		output string
	}{
		{"null", "null"},
		{"empty array", "[]"},
		{"blank", ""},
		{"only plugin panes", `[{"id":1,"is_plugin":true}]`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("ZELLIJ_SESSION_NAME", "test-session")
			t.Setenv("ZELLIJ_PANE_ID", "3")
			t.Setenv("PANECOM_STATE_DIR", t.TempDir())

			r := newFakeRunner()
			r.Stub([]byte(tc.output), "zellij", "action", "list-panes", "--json", "--all")
			r.Stub([]byte("test-session [Created 1h ago] (current)\n"), "zellij", "list-sessions", "--no-formatting")
			z := &ZellijMux{runner: r}
			prev := mux
			mux = z
			t.Cleanup(func() { mux = prev })

			nsDir := seedRealMuxSession(t, "zellij", "test-session", "terminal_3")
			if _, err := z.PaneExists("test-session", "terminal_3"); err == nil {
				t.Error("an empty pane list must be reported as an error, not as absence")
			}

			cmdCleanup(false, false)
			if _, err := resolveRole(nsDir, "reviewer"); err != nil {
				t.Errorf("cleanup deleted a role on an unreadable pane list: %v", err)
			}
		})
	}
}

func TestZellijListSessionsKeepsASessionNamedExited(t *testing.T) {
	r := newFakeRunner()
	r.Stub([]byte("EXITED-work [Created 1h ago] (current)\n"+
		"old-one [Created 25days ago] (EXITED - attach to resurrect)\n"),
		"zellij", "list-sessions", "--no-formatting")
	z := &ZellijMux{runner: r}

	got, err := z.ListSessions()
	if err != nil {
		t.Fatal(err)
	}
	// The marker belongs to the status, not to the name.
	if len(got) != 1 || got[0] != "EXITED-work" {
		t.Fatalf("sessions = %v, want [EXITED-work]", got)
	}
}

func TestCleanupKeepsTheStateOfASessionNamedExited(t *testing.T) {
	t.Setenv("ZELLIJ_SESSION_NAME", "ours")
	t.Setenv("ZELLIJ_PANE_ID", "3")
	t.Setenv("PANECOM_STATE_DIR", t.TempDir())

	r := newFakeRunner()
	r.Stub([]byte(`[{"id":3,"tab_id":0,"pane_command":"zsh"}]`), "zellij", "action", "list-panes", "--json", "--all")
	r.Stub([]byte("ours [Created 1h ago] (current)\n"+
		"EXITED-work [Created 2h ago]\n"),
		"zellij", "list-sessions", "--no-formatting")
	z := &ZellijMux{runner: r}
	prev := mux
	mux = z
	t.Cleanup(func() { mux = prev })

	seedRealMuxSession(t, "zellij", "ours", "terminal_3")
	running := sessionRoot("zellij", "EXITED-work")
	if err := atomicWrite(filepath.Join(running, ".session"), "EXITED-work"); err != nil {
		t.Fatal(err)
	}

	cmdCleanup(false, false)
	if _, err := os.Stat(running); err != nil {
		t.Errorf("the state of a running session named EXITED-work was deleted: %v", err)
	}
}
