package main

import (
	"bufio"
	"bytes"
	"encoding/json"
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
		if !install.Adopted || !install.Existed {
			t.Fatalf("existing script deployment was not adopted: %+v", install)
		}
	}
	if err := uninstallSkills(home, []string{targets[0].Path, targets[1].Path}); err != nil {
		t.Fatal(err)
	}
	for _, target := range targets {
		got, err := os.ReadFile(target.Path)
		want := embeddedSkill
		if target.Agent == "codex" {
			want = section + "\n"
		}
		if err != nil || string(got) != want {
			t.Fatalf("adopted file changed at %s: %q %v", target.Path, got, err)
		}
	}
}

func TestSkillUninstallPreservesAdoptedFileFromOlderManifest(t *testing.T) {
	home := t.TempDir()
	targets, _ := skillTargets(home, "", "claude", "user")
	path := targets[0].Path
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(embeddedSkill), 0600); err != nil {
		t.Fatal(err)
	}
	if err := writeManifest(home, skillManifest{Installs: []skillInstall{{Agent: "claude", Scope: "user", Path: path, Installed: contentHash(embeddedSkill), Adopted: true}}}); err != nil {
		t.Fatal(err)
	}
	if err := uninstallSkills(home, []string{path}); err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != embeddedSkill {
		t.Fatalf("adopted file lost: %q %v", got, err)
	}
}

func TestSkillInstallPreservesPrivateFileMode(t *testing.T) {
	home := t.TempDir()
	targets, _ := skillTargets(home, "", "codex", "user")
	path := targets[0].Path
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("private instructions\n"), 0600); err != nil {
		t.Fatal(err)
	}
	checkMode := func() {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0600 {
			t.Fatalf("mode=%v", info.Mode())
		}
	}
	if err := installSkills(home, targets); err != nil {
		t.Fatal(err)
	}
	checkMode()
	if err := os.Chmod(path, 0644); err != nil {
		t.Fatal(err)
	}
	if err := uninstallSkills(home, []string{path}); err != nil {
		t.Fatal(err)
	}
	checkMode()
}

func TestSkillUninstallPrunesMissingTarget(t *testing.T) {
	home := t.TempDir()
	a, b := t.TempDir(), t.TempDir()
	ta, _ := skillTargets(home, a, "codex", "project")
	tb, _ := skillTargets(home, b, "codex", "project")
	if err := installSkills(home, append(ta, tb...)); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(ta[0].Path); err != nil {
		t.Fatal(err)
	}
	if err := uninstallSkills(home, []string{ta[0].Path, tb[0].Path}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(tb[0].Path); !os.IsNotExist(err) {
		t.Fatalf("remaining target not removed: %v", err)
	}
	m, err := readManifest(home)
	if err != nil || len(m.Installs) != 0 {
		t.Fatalf("manifest=%+v err=%v", m, err)
	}
}

func TestSkillReinstallRejectsMalformedAddedBlock(t *testing.T) {
	home := t.TempDir()
	targets, _ := skillTargets(home, "", "codex", "user")
	if err := installSkills(home, targets); err != nil {
		t.Fatal(err)
	}
	m, err := readManifest(home)
	if err != nil {
		t.Fatal(err)
	}
	m.Installs[0].AddedBlock = "junk-without-marker"
	if err := writeManifest(home, m); err != nil {
		t.Fatal(err)
	}
	if err := installSkills(home, targets); err == nil || !strings.Contains(err.Error(), "missing start marker") {
		t.Fatalf("expected validation error, got %v", err)
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

func TestSkillPickerWarnsBeforeReplacingClaudeFile(t *testing.T) {
	home, project := t.TempDir(), t.TempDir()
	targets, err := allSkillTargets(home, project)
	if err != nil {
		t.Fatal(err)
	}
	path := targets[0].Path
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("custom skill"), 0600); err != nil {
		t.Fatal(err)
	}
	screen := renderSkillPicker(home, project, targets, make([]bool, len(targets)), 0, "")
	if !strings.Contains(screen, "existing file will be replaced") {
		t.Fatalf("replacement warning missing: %q", screen)
	}
}

func TestAllSkillTargetsDeduplicatesHomeProject(t *testing.T) {
	home := t.TempDir()
	targets, err := allSkillTargets(home, home)
	if err != nil || len(targets) != 3 {
		t.Fatalf("targets=%+v err=%v", targets, err)
	}
}

func TestAllSkillTargetsDeduplicatesSymlinkedHome(t *testing.T) {
	parent := t.TempDir()
	home := filepath.Join(parent, "home")
	alias := filepath.Join(parent, "home-link")
	if err := os.Mkdir(home, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(home, alias); err != nil {
		t.Fatal(err)
	}
	targets, err := allSkillTargets(alias, home)
	if err != nil || len(targets) != 3 {
		t.Fatalf("symlinked home produced duplicate targets: %+v err=%v", targets, err)
	}
	if targets[0].Path != filepath.Join(alias, ".claude", "skills", "panecom.md") {
		t.Fatalf("user target path changed: %s", targets[0].Path)
	}
	if err := installSkills(alias, targets[:1]); err != nil {
		t.Fatal(err)
	}
	manifest, err := readManifest(alias)
	if err != nil {
		t.Fatal(err)
	}
	untracked, err := untrackedSkills(alias, home, manifest)
	if err != nil || len(untracked) != 0 {
		t.Fatalf("installed symlinked target shown as untracked: %+v err=%v", untracked, err)
	}
}

func TestSkillInstallUsesOneManifestEntryAcrossHomeAliases(t *testing.T) {
	parent := t.TempDir()
	home := filepath.Join(parent, "home")
	alias := filepath.Join(parent, "home-link")
	if err := os.Mkdir(home, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(home, alias); err != nil {
		t.Fatal(err)
	}
	aliased, _ := skillTargets(alias, "", "codex", "user")
	canonical, _ := skillTargets(home, "", "codex", "user")
	path := canonical[0].Path
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	const original = "original instructions\n"
	if err := os.WriteFile(path, []byte(original), 0600); err != nil {
		t.Fatal(err)
	}
	if err := installSkills(alias, aliased); err != nil {
		t.Fatal(err)
	}
	if err := installSkills(home, canonical); err != nil {
		t.Fatal(err)
	}
	m, err := readManifest(home)
	if err != nil || len(m.Installs) != 1 {
		t.Fatalf("same destination tracked twice: %+v err=%v", m, err)
	}
	if err := uninstallSkills(home, []string{canonical[0].Path}); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil || string(got) != original {
		t.Fatalf("original not restored: %q err=%v", got, err)
	}
}

func TestSkillUninstallRecoversExistingDuplicateManifestEntries(t *testing.T) {
	home := t.TempDir()
	alias := filepath.Join(filepath.Dir(home), filepath.Base(home)+"-link")
	if err := os.Symlink(home, alias); err != nil {
		t.Fatal(err)
	}
	targets, _ := skillTargets(home, "", "codex", "user")
	path := targets[0].Path
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	const original = "original instructions\n"
	if err := os.WriteFile(path, []byte(original), 0644); err != nil {
		t.Fatal(err)
	}
	if err := installSkills(home, targets); err != nil {
		t.Fatal(err)
	}
	m, err := readManifest(home)
	if err != nil {
		t.Fatal(err)
	}
	duplicate := m.Installs[0]
	duplicate.Path = filepath.Join(alias, ".codex", "instructions.md")
	section, err := skillSection()
	if err != nil {
		t.Fatal(err)
	}
	duplicate.OldSection = section
	duplicate.AddedBlock = ""
	m.Installs = append(m.Installs, duplicate)
	data, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manifestPath(home), data, 0600); err != nil {
		t.Fatal(err)
	}
	m, err = readManifest(home)
	if err != nil || len(m.Installs) != 1 {
		t.Fatalf("duplicate manifest was not normalized: %+v err=%v", m, err)
	}
	if err := uninstallSkills(home, []string{m.Installs[0].Path}); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil || string(got) != original {
		t.Fatalf("original not restored: %q err=%v", got, err)
	}
}

func TestSkillPickerSurvivesBlockedClaudeDirectory(t *testing.T) {
	home, project := t.TempDir(), t.TempDir()
	if err := os.WriteFile(filepath.Join(home, ".claude"), []byte("not a directory"), 0644); err != nil {
		t.Fatal(err)
	}
	targets, err := allSkillTargets(home, project)
	if err != nil || len(targets) != 4 {
		t.Fatalf("picker destinations unavailable: %+v err=%v", targets, err)
	}
	screen := renderSkillPicker(home, project, targets, make([]bool, len(targets)), 0, "")
	if !strings.Contains(screen, "Codex  / User") {
		t.Fatalf("Codex destination missing: %q", screen)
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
