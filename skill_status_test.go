package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// oldSkill stands in for a skill written by an earlier release.
const oldSkill = "---\nname: panecom\ndescription: old\n---\n\nuse panecom send developer\n"

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func readSkill(t *testing.T, path string) (string, bool) {
	t.Helper()
	content, exists, err := readOptional(path)
	if err != nil {
		t.Fatal(err)
	}
	return content, exists
}

// recordInstall writes path and records it as an install made by an earlier
// release, the way that release's 'skill install' would have.
func recordInstall(t *testing.T, home string, install skillInstall, content string) {
	t.Helper()
	writeFile(t, install.Path, content)
	manifest, err := readManifest(home)
	if err != nil {
		t.Fatal(err)
	}
	if install.Agent == "claude" {
		install.Installed = contentHash(content)
	} else {
		start, end, err := sectionBounds(content)
		if err != nil || start < 0 {
			t.Fatalf("no block in %q", content)
		}
		install.Installed = contentHash(content[start:end])
	}
	manifest.Installs = append(manifest.Installs, install)
	if err := writeManifest(home, manifest); err != nil {
		t.Fatal(err)
	}
}

func userClaudePaths(home string) (current, legacy string) {
	current = claudeSkillPath(home)
	return current, legacyClaudeSkillPath(current)
}

func TestClaudeSkillUsesTheSkillDirectoryLayout(t *testing.T) {
	targets, err := skillTargets("/h", "/p", "claude", "user")
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join("/h", ".claude", "skills", "panecom", "SKILL.md"); targets[0].Path != want {
		t.Fatalf("path = %s, want %s", targets[0].Path, want)
	}
	if got := legacyClaudeSkillPath(targets[0].Path); got != filepath.Join("/h", ".claude", "skills", "panecom.md") {
		t.Fatalf("legacy path = %s", got)
	}
	if got := currentClaudeSkillPath(legacyClaudeSkillPath(targets[0].Path)); got != targets[0].Path {
		t.Fatalf("round trip = %s", got)
	}
	if got := displaySkillPath(targets[0]); got != "~/.claude/skills/panecom/SKILL.md" {
		t.Fatalf("display = %s", got)
	}
}

func TestSkillUpdateMovesAnOldLayoutInstall(t *testing.T) {
	home := t.TempDir()
	current, legacy := userClaudePaths(home)
	recordInstall(t, home, skillInstall{Agent: "claude", Scope: "user", Path: legacy}, oldSkill)

	updated, skipped, err := updateSkills(home)
	if err != nil {
		t.Fatal(err)
	}
	if len(updated) != 1 || updated[0] != current || len(skipped) != 0 {
		t.Fatalf("updated=%v skipped=%v", updated, skipped)
	}
	if got, _ := readSkill(t, current); got != embeddedSkill {
		t.Fatalf("new path does not hold the skill: %q", got)
	}
	if _, exists := readSkill(t, legacy); exists {
		t.Fatal("old-layout file was left behind")
	}
	manifest, _ := readManifest(home)
	if len(manifest.Installs) != 1 || manifest.Installs[0].Path != current || manifest.Installs[0].Installed != contentHash(embeddedSkill) {
		t.Fatalf("manifest not moved: %+v", manifest.Installs)
	}
	// The moved record still uninstalls cleanly.
	if err := uninstallSkills(home, []string{current}); err != nil {
		t.Fatal(err)
	}
	if _, exists := readSkill(t, current); exists {
		t.Fatal("uninstall left the new file behind")
	}
}

func TestSkillInstallMovesAnOldLayoutInstall(t *testing.T) {
	home := t.TempDir()
	_, legacy := userClaudePaths(home)
	recordInstall(t, home, skillInstall{Agent: "claude", Scope: "user", Path: legacy}, oldSkill)
	targets, _ := skillTargets(home, "", "claude", "user")
	if err := installSkills(home, targets); err != nil {
		t.Fatal(err)
	}
	if _, exists := readSkill(t, legacy); exists {
		t.Fatal("install did not move the old-layout file")
	}
	manifest, _ := readManifest(home)
	if len(manifest.Installs) != 1 || manifest.Installs[0].Path != targets[0].Path {
		t.Fatalf("manifest = %+v", manifest.Installs)
	}
}

func TestSkillUpdateRestoresWhatTheOldLayoutReplaced(t *testing.T) {
	home := t.TempDir()
	current, legacy := userClaudePaths(home)
	userFile := "my own notes, not a panecom skill\n"
	recordInstall(t, home, skillInstall{Agent: "claude", Scope: "user", Path: legacy, Existed: true, Previous: userFile, Mode: 0600}, oldSkill)
	if _, _, err := updateSkills(home); err != nil {
		t.Fatal(err)
	}
	if got, _ := readSkill(t, legacy); got != userFile {
		t.Fatalf("the file panecom replaced was not restored: %q", got)
	}
	if info, _ := os.Stat(legacy); info.Mode().Perm() != 0600 {
		t.Fatalf("restored mode = %v", info.Mode().Perm())
	}
	if got, _ := readSkill(t, current); got != embeddedSkill {
		t.Fatal("new path not installed")
	}
}

func TestSkillUpdateLeavesACurrentLayoutInstallAlone(t *testing.T) {
	home := t.TempDir()
	targets, _ := skillTargets(home, "", "claude", "user")
	if err := installSkills(home, targets); err != nil {
		t.Fatal(err)
	}
	updated, skipped, err := updateSkills(home)
	if err != nil || len(updated) != 0 || len(skipped) != 0 {
		t.Fatalf("updated=%v skipped=%v err=%v", updated, skipped, err)
	}
}

func TestSkillUpdateWithBothLayoutsRetiresTheOldOne(t *testing.T) {
	home := t.TempDir()
	current, legacy := userClaudePaths(home)
	recordInstall(t, home, skillInstall{Agent: "claude", Scope: "user", Path: legacy}, oldSkill)
	recordInstall(t, home, skillInstall{Agent: "claude", Scope: "user", Path: current}, embeddedSkill)

	updated, _, err := updateSkills(home)
	if err != nil {
		t.Fatal(err)
	}
	if len(updated) != 1 || updated[0] != current {
		t.Fatalf("updated = %v", updated)
	}
	if _, exists := readSkill(t, legacy); exists {
		t.Fatal("old-layout file was left behind")
	}
	manifest, _ := readManifest(home)
	if len(manifest.Installs) != 1 || manifest.Installs[0].Path != current {
		t.Fatalf("manifest = %+v", manifest.Installs)
	}
}

func TestSkillMigrationKeepsAnEditedOldLayoutFile(t *testing.T) {
	home := t.TempDir()
	current, legacy := userClaudePaths(home)
	recordInstall(t, home, skillInstall{Agent: "claude", Scope: "user", Path: legacy}, oldSkill)
	edited := oldSkill + "my own addition\n"
	writeFile(t, legacy, edited)
	before, _ := os.ReadFile(manifestPath(home))

	updated, skipped, err := updateSkills(home)
	if err != nil {
		t.Fatal(err)
	}
	if len(updated) != 0 || len(skipped) != 1 || skipped[0] != legacy {
		t.Fatalf("updated=%v skipped=%v", updated, skipped)
	}
	// An explicit install must refuse rather than discard the edits.
	targets, _ := skillTargets(home, "", "claude", "user")
	err = installSkills(home, targets)
	if err == nil || !strings.Contains(err.Error(), "changed since installation") {
		t.Fatalf("install err = %v", err)
	}
	if got, _ := readSkill(t, legacy); got != edited {
		t.Fatalf("edited old-layout file was touched: %q", got)
	}
	if _, exists := readSkill(t, current); exists {
		t.Fatal("new path written although the install was refused")
	}
	after, _ := os.ReadFile(manifestPath(home))
	if string(before) != string(after) {
		t.Fatal("manifest changed although the install was refused")
	}
}

func TestSkillInstallLeavesAnUntrackedOldLayoutFile(t *testing.T) {
	home := t.TempDir()
	current, legacy := userClaudePaths(home)
	writeFile(t, legacy, oldSkill) // e.g. copied by an old deploy-skill.sh
	untracked, err := untrackedSkills(home, t.TempDir(), skillManifest{})
	if err != nil {
		t.Fatal(err)
	}
	if len(untracked) != 1 || untracked[0].Path != legacy || !isLegacyInstall(untracked[0]) {
		t.Fatalf("untracked = %+v", untracked)
	}
	targets, _ := skillTargets(home, "", "claude", "user")
	if err := installSkills(home, targets); err != nil {
		t.Fatal(err)
	}
	if got, _ := readSkill(t, legacy); got != oldSkill {
		t.Fatal("a file panecom did not install was changed")
	}
	if got, _ := readSkill(t, current); got != embeddedSkill {
		t.Fatal("new path not installed")
	}
}

func TestInstallStateReportsCurrentOutdatedAndModified(t *testing.T) {
	home := t.TempDir()
	current, legacy := userClaudePaths(home)
	codex := filepath.Join(home, ".codex", "instructions.md")
	recordInstall(t, home, skillInstall{Agent: "claude", Scope: "user", Path: current}, embeddedSkill)
	cases := []struct {
		name    string
		install func() skillInstall
		want    skillState
	}{
		{"current", func() skillInstall {
			m, _ := readManifest(home)
			return m.Installs[0]
		}, skillCurrent},
		{"outdated claude", func() skillInstall {
			path := filepath.Join(home, "project", ".claude", "skills", "panecom", "SKILL.md")
			recordInstall(t, home, skillInstall{Agent: "claude", Scope: "project", Path: path}, oldSkill)
			m, _ := readManifest(home)
			return m.Installs[findInstall(m, path)]
		}, skillOutdated},
		{"outdated codex", func() skillInstall {
			recordInstall(t, home, skillInstall{Agent: "codex", Scope: "user", Path: codex}, "mine\n\n"+skillStart+"\nold\n"+skillEnd+"\n")
			m, _ := readManifest(home)
			return m.Installs[findInstall(m, codex)]
		}, skillOutdated},
		{"old layout", func() skillInstall {
			recordInstall(t, home, skillInstall{Agent: "claude", Scope: "user", Path: legacy}, oldSkill)
			m, _ := readManifest(home)
			return m.Installs[findInstall(m, legacy)]
		}, skillLegacy},
		{"modified", func() skillInstall {
			m, _ := readManifest(home)
			install := m.Installs[findInstall(m, current)]
			writeFile(t, current, embeddedSkill+"edit\n")
			return install
		}, skillModified},
		{"missing", func() skillInstall {
			m, _ := readManifest(home)
			install := m.Installs[findInstall(m, current)]
			if err := os.Remove(current); err != nil {
				t.Fatal(err)
			}
			return install
		}, skillMissing},
	}
	for _, c := range cases {
		got, err := installState(c.install())
		if err != nil || got != c.want {
			t.Errorf("%s: state = %q err = %v, want %q", c.name, got, err, c.want)
		}
	}
}

func TestSkillUpdateRefreshesOutdatedAndSkipsModified(t *testing.T) {
	home := t.TempDir()
	current, _ := userClaudePaths(home)
	codex := filepath.Join(home, ".codex", "instructions.md")
	recordInstall(t, home, skillInstall{Agent: "claude", Scope: "user", Path: current}, oldSkill)
	recordInstall(t, home, skillInstall{Agent: "codex", Scope: "user", Path: codex}, "mine\n\n"+skillStart+"\nold\n"+skillEnd+"\n")
	writeFile(t, codex, "mine\n\n"+skillStart+"\nold, edited\n"+skillEnd+"\n")

	updated, skipped, err := updateSkills(home)
	if err != nil {
		t.Fatal(err)
	}
	if len(updated) != 1 || updated[0] != current || len(skipped) != 1 || skipped[0] != codex {
		t.Fatalf("updated=%v skipped=%v", updated, skipped)
	}
	if got, _ := readSkill(t, current); got != embeddedSkill {
		t.Fatal("outdated skill not refreshed")
	}
	if got, _ := readSkill(t, codex); !strings.Contains(got, "old, edited") {
		t.Fatalf("edited install overwritten: %q", got)
	}
	m, _ := readManifest(home)
	if state, _ := installState(m.Installs[findInstall(m, current)]); state != skillCurrent {
		t.Fatalf("state after update = %q", state)
	}
}

func TestStaleSkillNoticeIsRateLimited(t *testing.T) {
	home := t.TempDir()
	current, _ := userClaudePaths(home)
	now := time.Date(2026, 11, 19, 12, 0, 0, 0, time.UTC)

	recordInstall(t, home, skillInstall{Agent: "claude", Scope: "user", Path: current}, embeddedSkill)
	if got := staleSkillNotice(home, now); got != "" {
		t.Fatalf("notice for an up-to-date skill: %q", got)
	}
	if _, err := os.Stat(skillNoticePath(home)); !os.IsNotExist(err) {
		t.Fatal("an up-to-date check should not use up the reminder")
	}

	recordInstall(t, home, skillInstall{Agent: "claude", Scope: "user", Path: current}, oldSkill)
	m, _ := readManifest(home)
	m.Installs[0].Installed = contentHash(oldSkill)
	if err := writeManifest(home, m); err != nil {
		t.Fatal(err)
	}
	got := staleSkillNotice(home, now)
	if !strings.Contains(got, "panecom skill update") || !strings.Contains(got, current) {
		t.Fatalf("notice = %q", got)
	}
	if got := staleSkillNotice(home, now.Add(time.Hour)); got != "" {
		t.Fatalf("repeated within the interval: %q", got)
	}
	if got := staleSkillNotice(home, now.Add(25*time.Hour)); got == "" {
		t.Fatal("no reminder after the interval")
	}
}

func TestStaleSkillNoticeIgnoresModifiedInstalls(t *testing.T) {
	home := t.TempDir()
	current, _ := userClaudePaths(home)
	recordInstall(t, home, skillInstall{Agent: "claude", Scope: "user", Path: current}, oldSkill)
	writeFile(t, current, oldSkill+"edit\n")
	if got := staleSkillNotice(home, time.Now()); got != "" {
		t.Fatalf("'skill update' cannot fix an edited file, so do not nag: %q", got)
	}
}
