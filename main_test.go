package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHashString(t *testing.T) {
	h1 := hashString("hello")
	h2 := hashString("hello")
	h3 := hashString("world")
	if h1 != h2 {
		t.Error("same input should produce same hash")
	}
	if h1 == h3 {
		t.Error("different input should produce different hash")
	}
	if len(h1) != 64 {
		t.Errorf("expected 64 char hex, got %d", len(h1))
	}
}

func TestValidateRole(t *testing.T) {
	valid := []string{"implementer", "reviewer", "claude", "reviewer-2", "code_review", "A1"}
	for _, r := range valid {
		if !rolePattern.MatchString(r) {
			t.Errorf("expected '%s' to be valid", r)
		}
	}
	invalid := []string{"", "-start", ".dot", "../traversal", "has/slash", "has space"}
	for _, r := range invalid {
		if rolePattern.MatchString(r) {
			t.Errorf("expected '%s' to be invalid", r)
		}
	}
}

func TestAtomicWrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sub", "file.txt")
	if err := atomicWrite(path, "hello"); err != nil {
		t.Fatal(err)
	}
	content, err := readFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if content != "hello" {
		t.Errorf("expected 'hello', got '%s'", content)
	}
}

func TestResolveRoleNotRegistered(t *testing.T) {
	dir := t.TempDir()
	_, err := resolveRole(dir, "nonexistent")
	if err == nil {
		t.Error("expected error for unregistered role")
	}
}

func TestNamespaceDir(t *testing.T) {
	sRoot := "/tmp/test-state"
	ns1 := namespaceDir(sRoot, "/work/project-a")
	ns2 := namespaceDir(sRoot, "/work/project-b")
	ns3 := namespaceDir(sRoot, "/work/project-a")
	if ns1 == ns2 {
		t.Error("different cwds should produce different namespace dirs")
	}
	if ns1 != ns3 {
		t.Error("same cwd should produce same namespace dir")
	}
}

func TestStateRootEnvOverride(t *testing.T) {
	t.Setenv("PANECOM_STATE_DIR", "/custom/state/dir")
	root := stateRoot()
	if root != "/custom/state/dir" {
		t.Errorf("expected /custom/state/dir, got %s", root)
	}
}

func TestStateRootDefaultCwd(t *testing.T) {
	t.Setenv("PANECOM_STATE_DIR", "")
	root := stateRoot()
	if !strings.HasSuffix(root, ".panecom") {
		t.Errorf("expected path ending in .panecom, got %s", root)
	}
}

func TestStateRootWalkUp(t *testing.T) {
	t.Setenv("PANECOM_STATE_DIR", "")
	dir := t.TempDir()
	panecomDir := filepath.Join(dir, ".panecom")
	_ = os.MkdirAll(panecomDir, 0755)
	subdir := filepath.Join(dir, "sub", "deep")
	_ = os.MkdirAll(subdir, 0755)
	origDir, _ := os.Getwd()
	_ = os.Chdir(subdir)
	defer func() { _ = os.Chdir(origDir) }()

	root := stateRoot()
	expectedResolved, _ := filepath.EvalSymlinks(panecomDir)
	rootResolved, _ := filepath.EvalSymlinks(root)
	if rootResolved != expectedResolved {
		t.Errorf("expected %s, got %s", expectedResolved, rootResolved)
	}
}

func TestResolveRoleRoundTrip(t *testing.T) {
	dir := t.TempDir()
	rolesDir := filepath.Join(dir, "roles")
	_ = os.MkdirAll(rolesDir, 0755)
	_ = atomicWrite(filepath.Join(rolesDir, "reviewer"), "terminal_5")

	paneID, err := resolveRole(dir, "reviewer")
	if err != nil {
		t.Fatal(err)
	}
	if paneID != "terminal_5" {
		t.Errorf("expected terminal_5, got %s", paneID)
	}
}

func setupTestState(t *testing.T) (stateDir string, cleanup func()) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("PANECOM_STATE_DIR", dir)
	return dir, func() {}
}

func TestRegisterRoleSwap(t *testing.T) {
	stateDir, cleanup := setupTestState(t)
	defer cleanup()

	session := "test-session"
	sRoot := filepath.Join(stateDir, "sessions", hashString(session))
	cwd := "/work/project"
	cwdHash := hashString(cwd)
	nsDir := filepath.Join(sRoot, "namespaces", cwdHash)

	_ = atomicWrite(filepath.Join(nsDir, "roles", "implementer"), "terminal_2")
	_ = atomicWrite(filepath.Join(sRoot, "panes", "terminal_2"), cwdHash)

	entries, _ := os.ReadDir(filepath.Join(nsDir, "roles"))
	for _, e := range entries {
		content, _ := readFile(filepath.Join(nsDir, "roles", e.Name()))
		if content == "terminal_2" {
			_ = os.Remove(filepath.Join(nsDir, "roles", e.Name()))
		}
	}
	_ = atomicWrite(filepath.Join(nsDir, "roles", "reviewer"), "terminal_2")
	_ = atomicWrite(filepath.Join(sRoot, "panes", "terminal_2"), cwdHash)

	_, err := resolveRole(nsDir, "implementer")
	if err == nil {
		t.Error("implementer should have been cleared")
	}

	paneID, err := resolveRole(nsDir, "reviewer")
	if err != nil {
		t.Fatal(err)
	}
	if paneID != "terminal_2" {
		t.Errorf("expected terminal_2, got %s", paneID)
	}
}

func TestRegisterLatestWins(t *testing.T) {
	stateDir, cleanup := setupTestState(t)
	defer cleanup()

	session := "test-session"
	sRoot := filepath.Join(stateDir, "sessions", hashString(session))
	cwd := "/work/project"
	cwdHash := hashString(cwd)
	nsDir := filepath.Join(sRoot, "namespaces", cwdHash)

	_ = atomicWrite(filepath.Join(nsDir, "roles", "reviewer"), "terminal_5")
	_ = atomicWrite(filepath.Join(sRoot, "panes", "terminal_5"), cwdHash)

	oldPaneID, _ := resolveRole(nsDir, "reviewer")
	if oldPaneID == "terminal_9" {
		t.Fatal("unexpected")
	}
	_ = os.Remove(filepath.Join(sRoot, "panes", oldPaneID))
	_ = atomicWrite(filepath.Join(nsDir, "roles", "reviewer"), "terminal_9")
	_ = atomicWrite(filepath.Join(sRoot, "panes", "terminal_9"), cwdHash)

	paneID, err := resolveRole(nsDir, "reviewer")
	if err != nil {
		t.Fatal(err)
	}
	if paneID != "terminal_9" {
		t.Errorf("expected terminal_9, got %s", paneID)
	}

	_, err = readFile(filepath.Join(sRoot, "panes", "terminal_5"))
	if err == nil {
		t.Error("old pane mapping should have been removed")
	}
}

func TestResolveRoleForCommandFallbackCwd(t *testing.T) {
	stateDir, cleanup := setupTestState(t)
	defer cleanup()

	session := "test-session"
	t.Setenv("ZELLIJ_SESSION_NAME", session)
	t.Setenv("ZELLIJ_PANE_ID", "")

	sRoot := filepath.Join(stateDir, "sessions", hashString(session))
	cwd := canonicalCwd()
	nsDir := namespaceDir(sRoot, cwd)

	_ = atomicWrite(filepath.Join(nsDir, "roles", "reviewer"), "terminal_99")

	paneID, err := resolveRole(nsDir, "reviewer")
	if err != nil {
		t.Fatal(err)
	}
	if paneID != "terminal_99" {
		t.Errorf("expected terminal_99, got %s", paneID)
	}
}

func TestParseConfig(t *testing.T) {
	data := []byte(`
profiles:
  dev:
    panes:
      - role: developer
        cmd: claude
        foreground: true
      - role: terminal
        direction: down
  review:
    panes:
      - role: developer
        cmd: claude
      - role: reviewer
        cmd: codex
`)
	cfg, err := parseConfig(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Profiles) != 2 {
		t.Fatalf("expected 2 profiles, got %d", len(cfg.Profiles))
	}
	dev := cfg.Profiles["dev"]
	if len(dev.Panes) != 2 {
		t.Fatalf("expected 2 panes in dev, got %d", len(dev.Panes))
	}
	if dev.Panes[0].Role != "developer" || dev.Panes[0].Cmd != "claude" || !dev.Panes[0].Foreground {
		t.Error("dev pane 0 mismatch")
	}
	if dev.Panes[1].Direction != "down" {
		t.Errorf("expected direction 'down', got '%s'", dev.Panes[1].Direction)
	}
}

func TestConfigMerge(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PANECOM_STATE_DIR", filepath.Join(dir, "state"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "config"))

	globalDir := filepath.Join(dir, "config", "panecom")
	_ = os.MkdirAll(globalDir, 0755)
	_ = os.WriteFile(filepath.Join(globalDir, "config.yaml"), []byte(`
profiles:
  global-only:
    panes:
      - role: developer
  shared:
    panes:
      - role: developer
        cmd: global-cmd
`), 0644)

	_ = os.MkdirAll(filepath.Join(dir, "state"), 0755)

	origDir, _ := os.Getwd()
	_ = os.MkdirAll(filepath.Join(dir, "project", ".panecom"), 0755)
	_ = os.WriteFile(filepath.Join(dir, "project", ".panecom", "config.yaml"), []byte(`
profiles:
  project-only:
    panes:
      - role: terminal
  shared:
    panes:
      - role: developer
        cmd: project-cmd
`), 0644)
	_ = os.Chdir(filepath.Join(dir, "project"))
	defer func() { _ = os.Chdir(origDir) }()

	cfg, err := loadConfig()
	if err != nil {
		t.Fatal(err)
	}

	if _, ok := cfg.Profiles["global-only"]; !ok {
		t.Error("global-only profile missing")
	}
	if _, ok := cfg.Profiles["project-only"]; !ok {
		t.Error("project-only profile missing")
	}
	shared := cfg.Profiles["shared"]
	if shared.Panes[0].Cmd != "project-cmd" {
		t.Errorf("expected project-cmd, got %s", shared.Panes[0].Cmd)
	}
}

func TestParseConfigEmpty(t *testing.T) {
	cfg, err := parseConfig([]byte("{}"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Profiles == nil {
		t.Error("Profiles should be initialized")
	}
	if len(cfg.Profiles) != 0 {
		t.Errorf("expected 0 profiles, got %d", len(cfg.Profiles))
	}
}

func TestNamespaceForCurrentPane(t *testing.T) {
	stateDir, cleanup := setupTestState(t)
	defer cleanup()

	sRoot := filepath.Join(stateDir, "sessions", "abc")
	cwdHash := hashString("/work/project")
	_ = atomicWrite(filepath.Join(sRoot, "panes", "terminal_3"), cwdHash)

	nsDir, err := namespaceForCurrentPane(sRoot, "terminal_3")
	if err != nil {
		t.Fatal(err)
	}
	expected := filepath.Join(sRoot, "namespaces", cwdHash)
	if nsDir != expected {
		t.Errorf("expected %s, got %s", expected, nsDir)
	}

	_, err = namespaceForCurrentPane(sRoot, "terminal_999")
	if err == nil {
		t.Error("expected error for unregistered pane")
	}
}

func TestShellQuote(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"simple", "simple"},
		{"/usr/bin/panecom", "/usr/bin/panecom"},
		{"path with spaces", "'path with spaces'"},
		{"it's", "'it'\"'\"'s'"},
		{"", "''"},
	}
	for _, tt := range tests {
		got := shellQuote(tt.input)
		if got != tt.expected {
			t.Errorf("shellQuote(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestNormalizeScreen(t *testing.T) {
	got := normalizeScreen("a  \nb\t\n\n")
	if got != "a\nb" {
		t.Errorf("unexpected normalized screen: %q", got)
	}
}

func TestVerifyFragment(t *testing.T) {
	if f := verifyFragment("short"); f != "" {
		t.Errorf("short message should have no fragment, got %q", f)
	}
	if f := verifyFragment("please review the PR"); f != "please review the PR" {
		t.Errorf("unexpected fragment: %q", f)
	}
	if f := verifyFragment("first line\nsecond line is the marker\n"); f != "second line is the marker" {
		t.Errorf("should use last non-empty line, got %q", f)
	}
	long := strings.Repeat("x", 100)
	if f := verifyFragment(long); len([]rune(f)) != 40 {
		t.Errorf("fragment should be capped at 40 runes, got %d", len([]rune(f)))
	}
}

func TestFragmentAtInputEdge(t *testing.T) {
	screen := "> please review the PR\n\n"
	if !fragmentAtInputEdge(screen, "please review the PR") {
		t.Error("expected fragment to be found in input line")
	}
	if fragmentAtInputEdge(screen, "") {
		t.Error("empty fragment should never match")
	}
	deep := "> please review the PR\na\nb\nc\nd\n"
	if fragmentAtInputEdge(deep, "please review the PR") {
		t.Error("fragment far above the bottom should not match")
	}
}

// fakeMux serves a scripted sequence of screens so the send path can be
// exercised without a real multiplexer.
type fakeMux struct {
	screens  []string
	idx      int
	sentText []string
	enters   int
}

func (f *fakeMux) Kind() string                   { return "fake" }
func (f *fakeMux) SessionName() (string, error)   { return "fake", nil }
func (f *fakeMux) CurrentPaneID() (string, error) { return "p1", nil }
func (f *fakeMux) ListPanes(string) ([]Pane, error) {
	return []Pane{{ID: "p1"}}, nil
}
func (f *fakeMux) PaneExists(string, string) (bool, error) { return true, nil }
func (f *fakeMux) NewPane(string, string, string) (string, error) {
	return "p2", nil
}
func (f *fakeMux) NewTab(string, string) (string, string, error) { return "t1", "p2", nil }
func (f *fakeMux) NewPaneInTab(string, string, string) (string, error) {
	return "p2", nil
}
func (f *fakeMux) DumpPane(string, bool) (string, error) {
	s := f.screens[f.idx]
	if f.idx < len(f.screens)-1 {
		f.idx++
	}
	return s, nil
}
func (f *fakeMux) SendKeys(_ string, text string) error {
	f.sentText = append(f.sentText, text)
	return nil
}
func (f *fakeMux) SendEnter(string) error          { f.enters++; return nil }
func (f *fakeMux) RenamePane(string, string) error { return nil }
func (f *fakeMux) FocusPane(string) error          { return nil }

func withFakeMux(t *testing.T, f *fakeMux) {
	t.Helper()
	prev := mux
	mux = f
	t.Cleanup(func() { mux = prev })
}

func TestFragmentAtInputEdgeWrapped(t *testing.T) {
	// The input line wrapped across two screen lines in a narrow pane.
	screen := "> please review the\nPR before merging\n"
	if !fragmentAtInputEdge(screen, "please review the PR before merging") {
		t.Error("wrapped input line should still match the fragment")
	}
}

func TestFragmentAtInputEdgeWrappedWithBoxDecoration(t *testing.T) {
	screen := "│ > please review the │\n│ PR before merging    │\n"
	if !fragmentAtInputEdge(screen, "please review the PR before merging") {
		t.Error("wrapped input line inside a box should still match the fragment")
	}
}

const wrappedMessage = "please review the PR before merging"

// Narrow pane: the message wraps onto a second line and Enter never took.
func TestSendAndVerifyWrappedNotSubmitted(t *testing.T) {
	f := &fakeMux{screens: []string{
		"earlier output\n> \n",
		"earlier output\n> please review the\nPR before merging\n",
	}}
	withFakeMux(t, f)

	code, _, reason, err := sendAndVerify("reviewer", "p1", wrappedMessage, true, 0.3)
	if err != nil {
		t.Fatal(err)
	}
	if code != exitSendNotSubmitted {
		t.Errorf("expected exit %d for wrapped unsubmitted input, got %d (%s)",
			exitSendNotSubmitted, code, reason)
	}
}

// Same message, but the recipient accepted it: the input line is clear again.
func TestSendAndVerifyWrappedSubmitted(t *testing.T) {
	f := &fakeMux{screens: []string{
		"earlier output\n> \n",
		"earlier output\n● please review the\nPR before merging\n\nWorking…\n\n> \n",
	}}
	withFakeMux(t, f)

	code, _, reason, err := sendAndVerify("reviewer", "p1", wrappedMessage, true, 0.3)
	if err != nil {
		t.Fatal(err)
	}
	if code != 0 {
		t.Errorf("expected exit 0 for accepted message, got %d (%s)", code, reason)
	}
}

func TestSendAndVerifyUnchangedScreen(t *testing.T) {
	f := &fakeMux{screens: []string{"Showing detailed transcript\n"}}
	withFakeMux(t, f)

	code, _, _, err := sendAndVerify("reviewer", "p1", wrappedMessage, true, 0.3)
	if err != nil {
		t.Fatal(err)
	}
	if code != exitSendUnconfirmed {
		t.Errorf("expected exit %d when the screen never changes, got %d", exitSendUnconfirmed, code)
	}
}

func TestSendAndVerifyDisabled(t *testing.T) {
	f := &fakeMux{screens: []string{"anything\n"}}
	withFakeMux(t, f)

	code, _, _, err := sendAndVerify("reviewer", "p1", wrappedMessage, false, 0.3)
	if err != nil {
		t.Fatal(err)
	}
	if code != 0 {
		t.Errorf("expected exit 0 with verify disabled, got %d", code)
	}
	if len(f.sentText) != 1 || f.sentText[0] != wrappedMessage || f.enters != 1 {
		t.Errorf("message should still be sent: %v enters=%d", f.sentText, f.enters)
	}
}

// Known limitation: when the recipient's UI draws hints below the input line,
// the residual message is no longer at the screen edge and exit 4 is not
// raised. This errs toward not reporting a false failure; the operator still
// sees the screen via exit 3 or a dump.
func TestFragmentAtInputEdgeMissesResidualUnderFooterHint(t *testing.T) {
	screen := "> please review the\nPR before merging\n? for shortcuts\n"
	if fragmentAtInputEdge(screen, "please review the PR before merging") {
		t.Error("documented limitation changed: residual under a footer hint now matches")
	}
}

// An accepted message is echoed in the transcript with an empty prompt below
// it. The prompt characters normalize away, so the input line must be
// recognised as empty rather than treated as the end of the echo.
func TestSendAndVerifyWrappedSubmittedEmptyPrompt(t *testing.T) {
	f := &fakeMux{screens: []string{
		"earlier output\n> \n",
		"earlier output\n● please review the\nPR before merging\n\n> \n",
	}}
	withFakeMux(t, f)

	code, _, reason, err := sendAndVerify("reviewer", "p1", wrappedMessage, true, 0.3)
	if err != nil {
		t.Fatal(err)
	}
	if code != 0 {
		t.Errorf("expected exit 0 when the input line is an empty prompt, got %d (%s)", code, reason)
	}
}

// Same, with the prompt drawn inside a box: the bottom border must not be
// mistaken for the input line.
func TestSendAndVerifyWrappedSubmittedEmptyPromptInBox(t *testing.T) {
	f := &fakeMux{screens: []string{
		"earlier output\n╭────────────╮\n│ >          │\n╰────────────╯\n",
		"earlier output\n● please review the\nPR before merging\n╭────────────╮\n│ >          │\n╰────────────╯\n",
	}}
	withFakeMux(t, f)

	code, _, reason, err := sendAndVerify("reviewer", "p1", wrappedMessage, true, 0.3)
	if err != nil {
		t.Fatal(err)
	}
	if code != 0 {
		t.Errorf("expected exit 0 for an empty boxed prompt, got %d (%s)", code, reason)
	}
}

// Residual text inside a boxed input line is still detected.
func TestSendAndVerifyWrappedNotSubmittedInBox(t *testing.T) {
	f := &fakeMux{screens: []string{
		"earlier output\n╭────────────╮\n│ >          │\n╰────────────╯\n",
		"earlier output\n╭────────────╮\n│ > please review the │\n│ PR before merging   │\n╰────────────╯\n",
	}}
	withFakeMux(t, f)

	code, _, reason, err := sendAndVerify("reviewer", "p1", wrappedMessage, true, 0.3)
	if err != nil {
		t.Fatal(err)
	}
	if code != exitSendNotSubmitted {
		t.Errorf("expected exit %d for residual text in a boxed input line, got %d (%s)",
			exitSendNotSubmitted, code, reason)
	}
}

func TestIsHorizontalBorder(t *testing.T) {
	for _, l := range []string{"╭────────────╮", "╰──╯", "+------+", "═══"} {
		if !isHorizontalBorder(l) {
			t.Errorf("expected %q to be a horizontal border", l)
		}
	}
	// A textless line with side borders is an empty input row, not an edge:
	// skipping it would let the echoed message above be read as the input.
	for _, l := range []string{"", "│   │", "│ > hello │", "? for shortcuts"} {
		if isHorizontalBorder(l) {
			t.Errorf("expected %q not to be a horizontal border", l)
		}
	}
}

// An empty boxed input line without a prompt character must still count as
// empty rather than being skipped as framing.
func TestSendAndVerifySubmittedEmptyBoxedLineWithoutPrompt(t *testing.T) {
	f := &fakeMux{screens: []string{
		"earlier output\n╭────────────╮\n│            │\n╰────────────╯\n",
		"earlier output\n● please review the\nPR before merging\n╭────────────╮\n│            │\n╰────────────╯\n",
	}}
	withFakeMux(t, f)

	code, _, reason, err := sendAndVerify("reviewer", "p1", wrappedMessage, true, 0.3)
	if err != nil {
		t.Fatal(err)
	}
	if code != 0 {
		t.Errorf("expected exit 0 for an empty boxed input line, got %d (%s)", code, reason)
	}
}
