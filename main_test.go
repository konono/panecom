package main

import (
	"io"
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

func inDir(t *testing.T, dir string, fn func()) {
	t.Helper()
	orig, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(orig) }()
	fn()
}

func anchorFrom(t *testing.T, dir string) string {
	t.Helper()
	var anchor string
	inDir(t, dir, func() { anchor = namespaceAnchor() })
	return anchor
}

func resolvedPath(t *testing.T, p string) string {
	t.Helper()
	r, err := filepath.EvalSymlinks(p)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestNamespaceAnchorWalksUpToGitRoot(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	repo := filepath.Join(home, "proj")
	sub := filepath.Join(repo, "a", "b")
	if err := os.MkdirAll(filepath.Join(repo, ".git"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(sub, 0755); err != nil {
		t.Fatal(err)
	}

	want := resolvedPath(t, repo)
	if got := anchorFrom(t, sub); got != want {
		t.Errorf("anchor from subdir = %s, want %s", got, want)
	}
	if got := anchorFrom(t, repo); got != want {
		t.Errorf("anchor from repo root = %s, want %s", got, want)
	}
}

func TestNamespaceAnchorAcceptsGitFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	repo := filepath.Join(home, "worktree")
	sub := filepath.Join(repo, "sub")
	if err := os.MkdirAll(sub, 0755); err != nil {
		t.Fatal(err)
	}
	// Worktrees and submodules carry a .git file instead of a directory.
	if err := os.WriteFile(filepath.Join(repo, ".git"), []byte("gitdir: /elsewhere\n"), 0644); err != nil {
		t.Fatal(err)
	}

	if got, want := anchorFrom(t, sub), resolvedPath(t, repo); got != want {
		t.Errorf("anchor = %s, want %s", got, want)
	}
}

func TestNamespaceAnchorFallsBackToPanecomDir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	proj := filepath.Join(home, "proj")
	sub := filepath.Join(proj, "deep", "deeper")
	if err := os.MkdirAll(filepath.Join(proj, ".panecom"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(sub, 0755); err != nil {
		t.Fatal(err)
	}

	if got, want := anchorFrom(t, sub), resolvedPath(t, proj); got != want {
		t.Errorf("anchor = %s, want %s", got, want)
	}
}

func TestNamespaceAnchorNeverAnchorsAtHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	// A catch-all ~/.panecom is a shared state store, not a project root.
	if err := os.MkdirAll(filepath.Join(home, ".panecom"), 0755); err != nil {
		t.Fatal(err)
	}
	a := filepath.Join(home, "a")
	b := filepath.Join(home, "b")
	for _, d := range []string{a, b} {
		if err := os.MkdirAll(d, 0755); err != nil {
			t.Fatal(err)
		}
	}

	anchorA := anchorFrom(t, a)
	anchorB := anchorFrom(t, b)
	if anchorA != resolvedPath(t, a) {
		t.Errorf("anchor = %s, want the directory itself %s", anchorA, resolvedPath(t, a))
	}
	if anchorA == anchorB {
		t.Errorf("unrelated directories under home collapsed into one anchor: %s", anchorA)
	}
	if anchorA == resolvedPath(t, home) {
		t.Error("home directory must not be used as a namespace anchor")
	}
}

func TestNamespaceAnchorKeepsNestedRepoSeparate(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	outer := filepath.Join(home, "outer")
	inner := filepath.Join(outer, "vendor", "inner")
	if err := os.MkdirAll(filepath.Join(outer, ".git"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(inner, ".git"), 0755); err != nil {
		t.Fatal(err)
	}

	if got, want := anchorFrom(t, inner), resolvedPath(t, inner); got != want {
		t.Errorf("nested repo anchor = %s, want %s", got, want)
	}
	if anchorFrom(t, inner) == anchorFrom(t, outer) {
		t.Error("nested repo should keep its own namespace")
	}
}

func TestNamespaceAnchorPrefersGitRootOverDeeperPanecom(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	repo := filepath.Join(home, "repo")
	sub := filepath.Join(repo, "sub")
	if err := os.MkdirAll(filepath.Join(repo, ".git"), 0755); err != nil {
		t.Fatal(err)
	}
	// A stale store left in a subdirectory by an older panecom.
	if err := os.MkdirAll(filepath.Join(sub, ".panecom"), 0755); err != nil {
		t.Fatal(err)
	}

	want := resolvedPath(t, repo)
	if got := anchorFrom(t, sub); got != want {
		t.Errorf("anchor from sub = %s, want the git root %s", got, want)
	}
	if got := anchorFrom(t, repo); got != want {
		t.Errorf("anchor from repo = %s, want %s", got, want)
	}
}

func TestNamespaceAnchorWithoutKnownHomeIgnoresPanecomMarker(t *testing.T) {
	root := t.TempDir()
	// Stands in for a catch-all ~/.panecom that must not capture the anchor.
	if err := os.MkdirAll(filepath.Join(root, ".panecom"), 0755); err != nil {
		t.Fatal(err)
	}
	a := filepath.Join(root, "a")
	b := filepath.Join(root, "b")
	for _, d := range []string{a, b} {
		if err := os.MkdirAll(d, 0755); err != nil {
			t.Fatal(err)
		}
	}

	anchorA := namespaceAnchorFrom(a, "")
	anchorB := namespaceAnchorFrom(b, "")
	if anchorA != a {
		t.Errorf("anchor = %s, want the directory itself %s", anchorA, a)
	}
	if anchorA == anchorB {
		t.Errorf("unknown home collapsed unrelated directories into %s", anchorA)
	}
	// A git root is still a safe anchor without a home boundary.
	repo := filepath.Join(root, "repo")
	if err := os.MkdirAll(filepath.Join(repo, ".git"), 0755); err != nil {
		t.Fatal(err)
	}
	deep := filepath.Join(repo, "x", "y")
	if err := os.MkdirAll(deep, 0755); err != nil {
		t.Fatal(err)
	}
	if got := namespaceAnchorFrom(deep, ""); got != repo {
		t.Errorf("anchor = %s, want %s", got, repo)
	}
}

func TestRegisterAndResolveAcrossRepoSubdir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("PANECOM_STATE_DIR", "")
	repo := filepath.Join(home, "repo")
	sub := filepath.Join(repo, "sub")
	if err := os.MkdirAll(filepath.Join(repo, ".git"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(repo, ".panecom"), 0755); err != nil {
		t.Fatal(err)
	}
	// A stale store in the subdirectory must not split the project.
	if err := os.MkdirAll(filepath.Join(sub, ".panecom"), 0755); err != nil {
		t.Fatal(err)
	}

	f := &fakeMux{}
	withFakeMux(t, f)

	inDir(t, repo, func() {
		f.pane = "p_repo"
		cmdRegister("r")
	})
	inDir(t, sub, func() {
		f.pane = "p_sub"
		cmdRegister("d")
	})

	// Both panes must land in one store and one namespace.
	var repoNs, subNs, sRoot string
	inDir(t, repo, func() {
		_, sRoot = requireSession()
		ns, err := namespaceForCurrentPane(sRoot, "p_repo")
		if err != nil {
			t.Fatal(err)
		}
		repoNs = ns
	})
	inDir(t, sub, func() {
		_, subRoot := requireSession()
		if subRoot != sRoot {
			t.Errorf("state root differs between repo (%s) and sub (%s)", sRoot, subRoot)
		}
		ns, err := namespaceForCurrentPane(subRoot, "p_sub")
		if err != nil {
			t.Fatal(err)
		}
		subNs = ns
	})
	if repoNs != subNs {
		t.Fatalf("namespace split: repo=%s sub=%s", repoNs, subNs)
	}

	// Each pane resolves the other's role.
	if got, err := resolveRole(repoNs, "d"); err != nil || got != "p_sub" {
		t.Errorf("resolve d = %q, %v; want p_sub", got, err)
	}
	if got, err := resolveRole(subNs, "r"); err != nil || got != "p_repo" {
		t.Errorf("resolve r = %q, %v; want p_repo", got, err)
	}
}

func TestDescribeNamespaceReportsRegisteredAnchor(t *testing.T) {
	nsDir := t.TempDir()
	if err := atomicWrite(filepath.Join(nsDir, ".cwd"), "/work/project"); err != nil {
		t.Fatal(err)
	}
	// The pane was registered under /work/project; the process has since
	// moved elsewhere, so the recorded anchor is what must be reported.
	if got := describeNamespace(nsDir); got != "/work/project" {
		t.Errorf("describeNamespace = %s, want /work/project", got)
	}

	empty := t.TempDir()
	if got := describeNamespace(empty); got != empty {
		t.Errorf("describeNamespace = %s, want the namespace path %s", got, empty)
	}
}

func TestWhoamiReportsRegisteredPaneAndNamespace(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("PANECOM_STATE_DIR", filepath.Join(home, "state"))
	a := filepath.Join(home, "a")
	b := filepath.Join(home, "b")
	for _, dir := range []string{a, b} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
	}
	f := &fakeMux{pane: "terminal_510"}
	withFakeMux(t, f)
	if err := registerPane("d", "terminal_510", resolvedPath(t, a), "fake"); err != nil {
		t.Fatal(err)
	}

	capture := func(roleOnly bool) string {
		reader, writer, err := os.Pipe()
		if err != nil {
			t.Fatal(err)
		}
		defer reader.Close()
		oldStdout := os.Stdout
		os.Stdout = writer
		defer func() { os.Stdout = oldStdout }()
		inDir(t, b, func() { cmdWhoami(roleOnly) })
		if err := writer.Close(); err != nil {
			t.Fatal(err)
		}
		output, err := io.ReadAll(reader)
		if err != nil {
			t.Fatal(err)
		}
		return string(output)
	}
	output := capture(false)
	want := "role: d\npane: terminal_510\nnamespace: " + resolvedPath(t, a) + "\n"
	if output != want {
		t.Fatalf("whoami = %q, want %q", output, want)
	}
	if output := capture(true); output != "d\n" {
		t.Fatalf("whoami --role = %q, want d", output)
	}
	nsDir := namespaceDir(sessionRootIn(stateRootFor(a), f.Kind(), "fake"), resolvedPath(t, a))
	if err := os.Remove(filepath.Join(nsDir, ".cwd")); err != nil {
		t.Fatal(err)
	}
	if output := capture(false); output != "role: d\npane: terminal_510\nnamespace: (unknown)\n" {
		t.Fatalf("whoami with missing anchor = %q", output)
	}
}

func TestSplitQualifiedRole(t *testing.T) {
	cases := []struct{ in, alias, role string }{
		{"r", "", "r"},
		{"mcp/reviewer", "mcp", "reviewer"},
		{"a/b/c", "a/b", "c"},
		{"/r", "", "r"},
	}
	for _, c := range cases {
		alias, role := splitQualifiedRole(c.in)
		if alias != c.alias || role != c.role {
			t.Errorf("split(%q) = (%q, %q), want (%q, %q)", c.in, alias, role, c.alias, c.role)
		}
	}
}

func TestExpandPathFrom(t *testing.T) {
	base := t.TempDir()
	sub := filepath.Join(base, "sub")
	if err := os.MkdirAll(sub, 0755); err != nil {
		t.Fatal(err)
	}
	got, err := expandPathFrom(base, "sub")
	if err != nil {
		t.Fatal(err)
	}
	if got != resolvedPath(t, sub) {
		t.Errorf("expandPathFrom = %s, want %s", got, resolvedPath(t, sub))
	}
	if _, err := expandPathFrom(base, "missing"); err == nil {
		t.Error("expected an error for a path that does not exist")
	}
	file := filepath.Join(base, "file")
	if err := os.WriteFile(file, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := expandPathFrom(base, "file"); err == nil {
		t.Error("expected an error for a path that is not a directory")
	}
}

// twoProjects sets up two sibling repositories with a registered pane each and
// leaves the process in the first one.
func twoProjects(t *testing.T) (a, b string, f *fakeMux) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("PANECOM_STATE_DIR", "")
	a = filepath.Join(home, "a")
	b = filepath.Join(home, "b")
	for _, d := range []string{a, b} {
		if err := os.MkdirAll(filepath.Join(d, ".git"), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(d, ".panecom"), 0755); err != nil {
			t.Fatal(err)
		}
	}
	f = &fakeMux{}
	withFakeMux(t, f)
	inDir(t, a, func() { f.pane = "pa"; cmdRegister("d") })
	inDir(t, b, func() { f.pane = "pb"; cmdRegister("rev") })
	f.pane = "pa"
	return resolvedPath(t, a), resolvedPath(t, b), f
}

func TestLinkAndResolveAcrossProjects(t *testing.T) {
	a, b, _ := twoProjects(t)

	inDir(t, a, func() {
		target, roles, err := linkProject("other", "../b", false)
		if err != nil {
			t.Fatal(err)
		}
		if target != b {
			t.Errorf("link target = %s, want %s", target, b)
		}
		if len(roles) != 1 || roles[0].Role != "rev" || roles[0].PaneID != "pb" {
			t.Fatalf("link reported roles %+v, want rev -> pb", roles)
		}

		// The whole point: address the other project's role.
		if got := resolveRoleForCommand("other/rev"); got != "pb" {
			t.Errorf("resolve other/rev = %s, want pb", got)
		}
		// The local namespace keeps working unqualified.
		if got := resolveRoleForCommand("d"); got != "pa" {
			t.Errorf("resolve d = %s, want pa", got)
		}
	})

	// The link is one-directional: b has not linked back.
	inDir(t, b, func() {
		_, sRoot := requireSession()
		nsHash := currentNamespaceHash(sRoot, "pb")
		if _, err := linkTarget(nsHash, "other"); err == nil {
			t.Error("link leaked into the target project")
		}
	})
}

func TestLinkNormalizesSubdirectoryToProjectRoot(t *testing.T) {
	a, b, _ := twoProjects(t)
	sub := filepath.Join(b, "deep", "deeper")
	if err := os.MkdirAll(sub, 0755); err != nil {
		t.Fatal(err)
	}

	inDir(t, a, func() {
		target, _, err := linkProject("other", filepath.Join("..", "b", "deep", "deeper"), false)
		if err != nil {
			t.Fatal(err)
		}
		if target != b {
			t.Errorf("link target = %s, want the project root %s", target, b)
		}
		if got := resolveRoleForCommand("other/rev"); got != "pb" {
			t.Errorf("resolve other/rev = %s, want pb", got)
		}
	})
}

func TestLinkRejectsSelfAndUnregisteredTarget(t *testing.T) {
	a, _, _ := twoProjects(t)

	inDir(t, a, func() {
		if _, _, err := linkProject("self", ".", false); err == nil {
			t.Error("expected an error when linking a project to itself")
		}
	})

	// A project with no registered role fails unless forced.
	empty := filepath.Join(filepath.Dir(a), "empty")
	if err := os.MkdirAll(filepath.Join(empty, ".git"), 0755); err != nil {
		t.Fatal(err)
	}
	inDir(t, a, func() {
		if _, _, err := linkProject("empty", "../empty", false); err == nil {
			t.Error("expected an error when the target has no roles")
		}
		if _, _, err := linkProject("empty", "../empty", true); err != nil {
			t.Errorf("--force should link anyway: %v", err)
		}
	})
}

func TestLinkedProjectHonoursStateDirOverride(t *testing.T) {
	a, b, _ := twoProjects(t)
	// Re-run the setup under an explicit store: every project, linked ones
	// included, must resolve inside it.
	store := filepath.Join(filepath.Dir(a), "store")
	t.Setenv("PANECOM_STATE_DIR", store)

	f := &fakeMux{}
	withFakeMux(t, f)
	inDir(t, a, func() { f.pane = "pa"; cmdRegister("d") })
	inDir(t, b, func() { f.pane = "pb"; cmdRegister("rev") })
	f.pane = "pa"

	inDir(t, a, func() {
		if _, _, err := linkProject("other", "../b", false); err != nil {
			t.Fatal(err)
		}
		if got := resolveRoleForCommand("other/rev"); got != "pb" {
			t.Errorf("resolve other/rev = %s, want pb", got)
		}
	})
	if _, err := os.Stat(store); err != nil {
		t.Errorf("state was not written to the override store: %v", err)
	}
}

func TestUnlinkRejectsAliasThatEscapesTheLinksDirectory(t *testing.T) {
	a, _, _ := twoProjects(t)

	inDir(t, a, func() {
		_, sRoot := requireSession()
		nsHash := currentNamespaceHash(sRoot, "pa")

		// A file that is not a link and must survive.
		victim := filepath.Join(sRoot, "panes", "pa")
		if _, err := os.Stat(victim); err != nil {
			t.Fatalf("expected a pane mapping to exist: %v", err)
		}
		escape := filepath.Join("..", "..", "sessions", filepath.Base(sRoot), "panes", "pa")
		if err := unlinkProject(escape); err == nil {
			t.Error("expected an error for an alias containing a path")
		}
		if _, err := os.Stat(victim); err != nil {
			t.Errorf("unlink deleted a file outside the links directory: %v", err)
		}

		// A link belonging to another namespace must also be out of reach.
		other := filepath.Join(stateRoot(), "externals", "deadbeef")
		if err := atomicWrite(filepath.Join(other, "keep"), "/somewhere"); err != nil {
			t.Fatal(err)
		}
		if err := unlinkProject(filepath.Join("..", "deadbeef", "keep")); err == nil {
			t.Error("expected an error for an alias reaching another namespace")
		}
		if _, err := os.Stat(filepath.Join(other, "keep")); err != nil {
			t.Errorf("unlink deleted another namespace's link: %v", err)
		}

		// The ordinary case still works.
		if _, _, err := linkProject("other", "../b", false); err != nil {
			t.Fatal(err)
		}
		if err := unlinkProject("other"); err != nil {
			t.Errorf("unlink of a real link failed: %v", err)
		}
		if _, err := linkTarget(nsHash, "other"); err == nil {
			t.Error("link survived unlink")
		}
	})
}

func TestLinkRequiresALivePane(t *testing.T) {
	a, b, f := twoProjects(t)
	f.dead = map[string]bool{"pb": true}

	inDir(t, a, func() {
		_, _, err := linkProject("other", "../b", false)
		if err == nil {
			t.Fatal("expected an error when every role in the target is stale")
		}
		if !strings.Contains(err.Error(), "no live roles") {
			t.Errorf("error should say the roles are stale, got: %v", err)
		}

		target, roles, err := linkProject("other", "../b", true)
		if err != nil {
			t.Fatalf("--force should link anyway: %v", err)
		}
		if target != b {
			t.Errorf("link target = %s, want %s", target, b)
		}
		// The stale role is still reported so the user can see why.
		if len(roles) != 1 || roles[0].Role != "rev" || roles[0].Alive {
			t.Errorf("roles = %+v, want rev reported as stale", roles)
		}
	})
}

func TestProfileExternalsLandInTheLaunchingProject(t *testing.T) {
	a, b, f := twoProjects(t)
	// A shared store is where this goes wrong: one store, several projects.
	store := filepath.Join(filepath.Dir(a), "store")
	t.Setenv("PANECOM_STATE_DIR", store)

	c := filepath.Join(filepath.Dir(a), "c")
	if err := os.MkdirAll(filepath.Join(c, ".git"), 0755); err != nil {
		t.Fatal(err)
	}
	inDir(t, c, func() { f.pane = "pc"; cmdRegister("rev") })

	// This pane is registered in B, but the profile is launched from A.
	inDir(t, b, func() { f.pane = "pb"; cmdRegister("d") })

	profile := profileConfig{
		Panes:            []paneConfig{{Role: "d"}},
		ExternalProjects: map[string]string{"other": "../c"},
	}
	inDir(t, a, func() { materializeExternalProjects(profile) })

	// The pane the profile starts registers in A, so the link has to be there.
	inDir(t, a, func() {
		f.pane = "pa"
		cmdRegister("d")
		if got := resolveRoleForCommand("other/rev"); got != "pc" {
			t.Errorf("resolve other/rev from the launched project = %s, want pc", got)
		}
	})

	// It must not have been written into the unrelated project B.
	inDir(t, b, func() {
		_, sRoot := requireSession()
		if _, err := linkTarget(currentNamespaceHash(sRoot, "pb"), "other"); err == nil {
			t.Error("link was written into the namespace of the already-registered pane")
		}
	})
}

func TestLinkPersistsAcrossSessions(t *testing.T) {
	a, b, f := twoProjects(t)

	inDir(t, a, func() {
		if _, _, err := linkProject("other", "../b", false); err != nil {
			t.Fatal(err)
		}
	})

	// Links record paths rather than pane IDs, so a new multiplexer session
	// must still see them.
	f.session = "later"
	inDir(t, a, func() {
		_, sRoot := requireSession()
		nsHash := currentNamespaceHash(sRoot, "")
		target, err := linkTarget(nsHash, "other")
		if err != nil {
			t.Fatal(err)
		}
		if target != b {
			t.Errorf("link target = %s, want %s", target, b)
		}
	})
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
	pane     string
	session  string
	dead     map[string]bool
}

func (f *fakeMux) Kind() string { return "fake" }
func (f *fakeMux) SessionName() (string, error) {
	if f.session != "" {
		return f.session, nil
	}
	return "fake", nil
}
func (f *fakeMux) CurrentPaneID() (string, error) {
	if f.pane != "" {
		return f.pane, nil
	}
	return "p1", nil
}
func (f *fakeMux) ListPanes(string) ([]Pane, error) {
	return []Pane{{ID: "p1"}}, nil
}
func (f *fakeMux) PaneExists(_ string, paneID string) (bool, error) {
	return !f.dead[paneID], nil
}
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
