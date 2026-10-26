package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLegacyRegistrationWarningAndReregister(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("PANECOM_STATE_DIR", "")
	repo := filepath.Join(home, "repo")
	oldDir := filepath.Join(repo, "sub")
	for _, dir := range []string{filepath.Join(repo, ".git"), oldDir} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
	}
	f := &fakeMux{pane: "p_old", dead: map[string]bool{"p_stale": true}}
	withFakeMux(t, f)
	anchor := resolvedPath(t, repo)
	oldAnchor := resolvedPath(t, oldDir)
	sRoot := sessionRootIn(stateRootFor(anchor), f.Kind(), "fake")
	oldNs := namespaceDir(sRoot, oldAnchor)
	for path, value := range map[string]string{
		filepath.Join(oldNs, ".cwd"):           oldAnchor,
		filepath.Join(oldNs, "roles", "d"):     "p_old",
		filepath.Join(oldNs, "roles", "x"):     "p_stale",
		filepath.Join(sRoot, "panes", "p_old"): hashString(oldAnchor),
	} {
		if err := atomicWrite(path, value); err != nil {
			t.Fatal(err)
		}
	}
	inDir(t, repo, func() {
		var warning bytes.Buffer
		warnLegacyRegistrations(&warning, sRoot, "fake", namespaceAnchor())
		if got := warning.String(); !strings.Contains(got, "old namespace "+oldAnchor) || !strings.Contains(got, "panecom register d") || strings.Contains(got, "p_stale") {
			t.Fatalf("warning = %q", got)
		}
		if got := currentNamespaceHash(sRoot, "p_old"); got != hashString(oldAnchor) {
			t.Fatalf("detection modified the pane mapping: %s", got)
		}
		rootRole := filepath.Join(namespaceDir(sRoot, anchor), "roles", "d")
		if err := atomicWrite(rootRole, "p_new"); err != nil {
			t.Fatal(err)
		}
		warning.Reset()
		warnLegacyRegistrations(&warning, sRoot, "fake", namespaceAnchor())
		if got := warning.String(); !strings.Contains(got, "it belongs to p_new") || strings.Contains(got, "run 'panecom register d'") {
			t.Fatalf("role collision warning = %q", got)
		}
		f.dead["p_new"] = true
		warning.Reset()
		warnLegacyRegistrations(&warning, sRoot, "fake", namespaceAnchor())
		if got := warning.String(); !strings.Contains(got, "panecom register d") || strings.Contains(got, "it belongs to") {
			t.Fatalf("stale role should not block re-registering: %q", got)
		}
		if err := os.Remove(rootRole); err != nil {
			t.Fatal(err)
		}
		if err := registerPane("d", "p_old", namespaceAnchor(), "fake"); err != nil {
			t.Fatal(err)
		}
		warning.Reset()
		warnLegacyRegistrations(&warning, sRoot, "fake", namespaceAnchor())
		if warning.Len() != 0 {
			t.Fatalf("warning remains after register: %q", warning.String())
		}
		if got := currentNamespaceHash(sRoot, "p_old"); got != hashString(anchor) {
			t.Fatalf("pane mapping was not moved: %s", got)
		}
	})
}

func TestLegacyRegistrationIgnoresOtherRepository(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("PANECOM_STATE_DIR", filepath.Join(home, "shared-state"))
	a := filepath.Join(home, "a")
	b := filepath.Join(home, "b")
	for _, repo := range []string{a, b} {
		if err := os.MkdirAll(filepath.Join(repo, ".git"), 0755); err != nil {
			t.Fatal(err)
		}
	}
	f := &fakeMux{}
	withFakeMux(t, f)
	sRoot := sessionRootIn(stateRootFor(a), f.Kind(), "fake")
	bNs := namespaceDir(sRoot, resolvedPath(t, b))
	for path, value := range map[string]string{
		filepath.Join(bNs, ".cwd"):       resolvedPath(t, b),
		filepath.Join(bNs, "roles", "d"): "p_other",
	} {
		if err := atomicWrite(path, value); err != nil {
			t.Fatal(err)
		}
	}
	if got := legacyRegistrations(sRoot, "fake", resolvedPath(t, a)); len(got) != 0 {
		t.Fatalf("other repository was flagged: %+v", got)
	}
}
