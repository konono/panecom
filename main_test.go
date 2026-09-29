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
