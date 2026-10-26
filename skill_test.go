package main

import (
	"bufio"
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSkillInstallAndUninstallSelectedTargets(t *testing.T) {
	home, project := t.TempDir(), t.TempDir()
	targets, err := skillTargets(home, project, "both", "project")
	if err != nil {
		t.Fatal(err)
	}
	claudePath, codexPath := targets[0].Path, targets[1].Path
	if err := os.MkdirAll(filepath.Dir(claudePath), 0755); err != nil {
		t.Fatal(err)
	}
	oldClaude := "custom Claude instructions\n"
	oldCodex := "Other instructions\n\n" + skillStart + "\nold panecom text\n" + skillEnd + "\n\nLast line\n"
	if err := os.WriteFile(claudePath, []byte(oldClaude), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(codexPath, []byte(oldCodex), 0644); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := installSkills(home, targets); err != nil {
			t.Fatal(err)
		}
	}
	manifest, err := readManifest(home)
	if err != nil || len(manifest.Installs) != 2 {
		t.Fatalf("manifest=%+v err=%v", manifest, err)
	}
	info, err := os.Stat(manifestPath(home))
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("manifest permissions=%v err=%v", info.Mode().Perm(), err)
	}
	installedCodex, err := os.ReadFile(codexPath)
	if err != nil || strings.Count(string(installedCodex), skillStart) != 1 {
		t.Fatalf("Codex install failed: %v, %q", err, installedCodex)
	}
	// Uninstall only Claude, leaving Codex's selected installation in place.
	if err := uninstallSkills(home, []string{claudePath}); err != nil {
		t.Fatal(err)
	}
	gotClaude, err := os.ReadFile(claudePath)
	if err != nil || string(gotClaude) != oldClaude {
		t.Fatalf("Claude original was not restored: %q %v", gotClaude, err)
	}
	manifest, _ = readManifest(home)
	if len(manifest.Installs) != 1 || manifest.Installs[0].Path != codexPath {
		t.Fatalf("wrong remaining installs: %+v", manifest.Installs)
	}
	if err := uninstallSkills(home, []string{codexPath}); err != nil {
		t.Fatal(err)
	}
	gotCodex, err := os.ReadFile(codexPath)
	if err != nil || string(gotCodex) != oldCodex {
		t.Fatalf("Codex original was not restored: %q %v", gotCodex, err)
	}
}

func TestSkillUninstallPreservesOtherCodexEdits(t *testing.T) {
	home := t.TempDir()
	targets, _ := skillTargets(home, "", "codex", "user")
	path := targets[0].Path
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("existing instructions\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := installSkills(home, targets); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	if err := os.WriteFile(path, append(data, []byte("new unrelated note\n")...), 0644); err != nil {
		t.Fatal(err)
	}
	if err := uninstallSkills(home, []string{path}); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(path)
	if string(got) != "existing instructions\nnew unrelated note\n" {
		t.Fatalf("unrelated changes lost: %q", got)
	}
}

func TestSkillInstallAdoptsMatchingScriptDeployment(t *testing.T) {
	home := t.TempDir()
	targets, _ := skillTargets(home, "", "both", "user")
	section, err := skillSection()
	if err != nil {
		t.Fatal(err)
	}
	for _, target := range targets {
		if err := os.MkdirAll(filepath.Dir(target.Path), 0755); err != nil {
			t.Fatal(err)
		}
		content := embeddedSkill
		if target.Agent == "codex" {
			content = section + "\n"
		}
		if err := os.WriteFile(target.Path, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
	if err := installSkills(home, targets); err != nil {
		t.Fatal(err)
	}
	manifest, _ := readManifest(home)
	for _, install := range manifest.Installs {
		if !install.Adopted {
			t.Fatalf("existing script deployment was not adopted: %+v", install)
		}
	}
	if err := uninstallSkills(home, []string{targets[0].Path, targets[1].Path}); err != nil {
		t.Fatal(err)
	}
	for _, target := range targets {
		if _, err := os.Stat(target.Path); !os.IsNotExist(err) {
			t.Fatalf("adopted install remains at %s: %v", target.Path, err)
		}
	}
}

func TestSkillUninstallRefusesModifiedClaudeSkill(t *testing.T) {
	home := t.TempDir()
	targets, _ := skillTargets(home, "", "claude", "user")
	if err := installSkills(home, targets); err != nil {
		t.Fatal(err)
	}
	path := targets[0].Path
	if err := os.WriteFile(path, []byte("user edited skill"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := uninstallSkills(home, []string{path}); err == nil {
		t.Fatal("expected modified skill to be preserved")
	}
	data, _ := os.ReadFile(path)
	if string(data) != "user edited skill" {
		t.Fatalf("edited skill was overwritten: %q", data)
	}
}

func TestSkillInstallRejectsMalformedCodexSectionBeforeWriting(t *testing.T) {
	home := t.TempDir()
	targets, _ := skillTargets(home, "", "both", "user")
	path := targets[1].Path
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	broken := "keep this\n" + skillStart + "\nno end"
	if err := os.WriteFile(path, []byte(broken), 0644); err != nil {
		t.Fatal(err)
	}
	if err := installSkills(home, targets); err == nil {
		t.Fatal("expected malformed marker error")
	}
	if _, err := os.Stat(targets[0].Path); !os.IsNotExist(err) {
		t.Fatalf("Claude was written despite invalid Codex file: %v", err)
	}
}

func TestSkillPromptChoiceRetries(t *testing.T) {
	var out bytes.Buffer
	got, err := promptChoice(bufio.NewReader(strings.NewReader("invalid\n2\n")), &out, "Select: ", map[string]string{"2": "codex"})
	if err != nil || got != "codex" || !strings.Contains(out.String(), "Choose one") {
		t.Fatalf("choice=%q err=%v output=%q", got, err, out.String())
	}
}

func TestSkillPickerShowsDirectoryPresence(t *testing.T) {
	home, project := t.TempDir(), t.TempDir()
	if err := os.Mkdir(filepath.Join(home, ".claude"), 0755); err != nil {
		t.Fatal(err)
	}
	targets, err := allSkillTargets(home, project)
	if err != nil {
		t.Fatal(err)
	}
	screen := renderSkillPicker(home, project, targets, make([]bool, len(targets)), 0, "")
	if !strings.Contains(screen, "Claude settings: found: "+filepath.Join(home, ".claude")) {
		t.Fatalf("Claude directory missing from picker: %q", screen)
	}
	if !strings.Contains(screen, "Codex settings:  not found: "+filepath.Join(home, ".codex")) {
		t.Fatalf("Codex absence missing from picker: %q", screen)
	}
}

func TestPickSkillTargetsSelectsIndependentAgentScopes(t *testing.T) {
	home, project := t.TempDir(), t.TempDir()
	targets, err := allSkillTargets(home, project)
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	// Space checks Claude/user; three Down arrows reach Codex/project;
	// Space checks it, then Down and Enter submit the two choices.
	selected, ok, err := pickSkillTargets(home, project, targets, strings.NewReader(" \x1b[B\x1b[B\x1b[B \x1b[B\r"), &out)
	if err != nil || !ok || len(selected) != 2 {
		t.Fatalf("selected=%+v ok=%v err=%v", selected, ok, err)
	}
	if selected[0].Agent != "claude" || selected[0].Scope != "user" || selected[1].Agent != "codex" || selected[1].Scope != "project" {
		t.Fatalf("wrong choices: %+v", selected)
	}
	if !strings.Contains(out.String(), "[x] Claude / User") || !strings.Contains(out.String(), "[x] Codex  / Project") || !strings.Contains(out.String(), "Install selected (2)") {
		t.Fatalf("selected state not visible: %q", out.String())
	}
}

func TestPickSkillTargetsRequiresSelectionAndCanCancel(t *testing.T) {
	home, project := t.TempDir(), t.TempDir()
	targets, _ := allSkillTargets(home, project)
	var out bytes.Buffer
	selected, ok, err := pickSkillTargets(home, project, targets, strings.NewReader("\t\r\x1b"), &out)
	if err != nil || ok || selected != nil || !strings.Contains(out.String(), "Select at least one destination") {
		t.Fatalf("empty submission/cancel: selected=%+v ok=%v err=%v output=%q", selected, ok, err, out.String())
	}
}

func TestUntrackedSkillsShowsScriptDeployments(t *testing.T) {
	home, project := t.TempDir(), t.TempDir()
	userTargets, _ := skillTargets(home, project, "both", "user")
	projectTargets, _ := skillTargets(home, project, "codex", "project")
	for _, target := range append(userTargets, projectTargets...) {
		if err := os.MkdirAll(filepath.Dir(target.Path), 0755); err != nil {
			t.Fatal(err)
		}
		content := embeddedSkill
		if target.Agent == "codex" {
			content = "other instructions\n" + skillStart + "\nold panecom\n" + skillEnd
		}
		if err := os.WriteFile(target.Path, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
	found, err := untrackedSkills(home, project, skillManifest{})
	if err != nil || len(found) != 3 {
		t.Fatalf("untracked=%+v err=%v", found, err)
	}
	tracked := skillManifest{Installs: []skillInstall{{Path: userTargets[0].Path}}}
	found, err = untrackedSkills(home, project, tracked)
	if err != nil || len(found) != 2 {
		t.Fatalf("untracked after tracking=%+v err=%v", found, err)
	}
}
