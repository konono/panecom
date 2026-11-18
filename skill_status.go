package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Claude Code loads a skill from <skills dir>/<name>/SKILL.md. Releases before
// this layout wrote the skill straight into the skills directory as
// panecom.md, where Claude Code never sees it; those installs are migrated.
func claudeSkillPath(base string) string {
	return filepath.Join(base, ".claude", "skills", "panecom", "SKILL.md")
}

// legacyClaudeSkillPath maps a current Claude skill path to the file the old
// layout used in the same skills directory.
func legacyClaudeSkillPath(path string) string {
	return filepath.Join(filepath.Dir(filepath.Dir(path)), "panecom.md")
}

// currentClaudeSkillPath is the inverse of legacyClaudeSkillPath.
func currentClaudeSkillPath(legacy string) string {
	return filepath.Join(filepath.Dir(legacy), "panecom", "SKILL.md")
}

func isLegacyInstall(install skillInstall) bool {
	return install.Agent == "claude" && filepath.Base(install.Path) == "panecom.md"
}

func isPanecomClaudeSkill(content string) bool {
	return strings.HasPrefix(content, "---\nname: panecom\n")
}

// legacyRetirement puts an old-layout install back the way it was before
// panecom wrote it, as uninstall would.
type legacyRetirement struct {
	snapshot fileSnapshot
	restore  string
	remove   bool
	mode     os.FileMode
}

func (r legacyRetirement) apply() error {
	if !r.snapshot.exists {
		return nil // Already gone; only the record is dropped.
	}
	if r.remove {
		if err := os.Remove(r.snapshot.path); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	return writeSkillFile(r.snapshot.path, r.restore, r.mode)
}

// planLegacyRetirement decides what happens to a tracked old-layout install
// next to the Claude skill at path. It refuses one the user has edited, so the
// move never discards their changes.
func planLegacyRetirement(manifest skillManifest, path string) (*legacyRetirement, error) {
	legacy := legacyClaudeSkillPath(path)
	index := findInstall(manifest, legacy)
	if index < 0 {
		return nil, nil
	}
	install := manifest.Installs[index]
	current, exists, err := readOptional(legacy)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", legacy, err)
	}
	mode, err := skillFileMode(legacy, exists)
	if err != nil {
		return nil, fmt.Errorf("checking %s: %w", legacy, err)
	}
	retirement := &legacyRetirement{snapshot: fileSnapshot{legacy, current, exists, mode}}
	if !exists {
		return retirement, nil
	}
	if !matchesInstalled(current, exists, install) {
		return nil, fmt.Errorf("%s (old layout, which Claude Code does not load) changed since installation; carry your edits over, delete it, and run this again", legacy)
	}
	// What was there before was either nothing or an older copy of this skill
	// (a script deployment that install adopted). Restoring the latter would
	// just leave another stale copy behind, so remove it.
	if !install.Existed || install.Previous == "" || isPanecomClaudeSkill(install.Previous) {
		retirement.remove = true
	} else {
		retirement.restore = install.Previous
		retirement.mode = restoredSkillMode(install, mode)
	}
	return retirement, nil
}

type skillState string

const (
	skillCurrent  skillState = "up to date"
	skillOutdated skillState = "outdated"
	skillLegacy   skillState = "old layout"
	skillModified skillState = "modified"
	skillMissing  skillState = "missing"
)

// installState compares a recorded install with the file on disk and with the
// skill embedded in this binary.
func installState(install skillInstall) (skillState, error) {
	current, exists, err := readOptional(install.Path)
	if err != nil {
		return "", err
	}
	if !exists {
		return skillMissing, nil
	}
	if !matchesInstalled(current, exists, install) {
		return skillModified, nil
	}
	if isLegacyInstall(install) {
		return skillLegacy, nil
	}
	want := contentHash(embeddedSkill)
	if install.Agent != "claude" {
		section, err := skillSection()
		if err != nil {
			return "", err
		}
		want = contentHash(section)
	}
	if install.Installed != want {
		return skillOutdated, nil
	}
	return skillCurrent, nil
}

func stateNote(state skillState) string {
	switch state {
	case skillOutdated:
		return " [outdated: run 'panecom skill update']"
	case skillLegacy:
		return " [old layout, not loaded by Claude Code: run 'panecom skill update']"
	case skillModified:
		return " [edited since installation: not updated automatically]"
	case skillMissing:
		return " [file missing]"
	}
	return ""
}

// staleInstalls lists the recorded installs 'skill update' would rewrite.
func staleInstalls(manifest skillManifest) ([]skillInstall, error) {
	var stale []skillInstall
	for _, install := range manifest.Installs {
		state, err := installState(install)
		if err != nil {
			return nil, err
		}
		if state == skillOutdated || state == skillLegacy {
			stale = append(stale, install)
		}
	}
	return stale, nil
}

// updateSkills rewrites every recorded install that is outdated or uses the
// old layout. Installs the user edited are left alone and returned, so the
// caller can say why they were skipped.
func updateSkills(home string) (updated []string, skipped []string, err error) {
	manifest, err := readManifest(home)
	if err != nil {
		return nil, nil, err
	}
	var targets []skillInstall
	seen := map[string]bool{}
	for _, install := range manifest.Installs {
		state, err := installState(install)
		if err != nil {
			return nil, nil, err
		}
		switch state {
		case skillModified:
			skipped = append(skipped, install.Path)
			continue
		case skillOutdated:
		case skillLegacy:
			install = skillInstall{Agent: install.Agent, Scope: install.Scope, Path: currentClaudeSkillPath(install.Path)}
		default:
			continue
		}
		key := skillPathKey(install.Path)
		if seen[key] {
			continue // An old-layout install and its replacement both recorded.
		}
		seen[key] = true
		targets = append(targets, skillInstall{Agent: install.Agent, Scope: install.Scope, Path: install.Path})
		updated = append(updated, install.Path)
	}
	if len(targets) == 0 {
		return nil, skipped, nil
	}
	if err := installSkills(home, targets); err != nil {
		return nil, skipped, err
	}
	return updated, skipped, nil
}

func cmdSkillUpdate(check bool) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	if check {
		manifest, err := readManifest(home)
		if err != nil {
			return err
		}
		stale, err := staleInstalls(manifest)
		if err != nil {
			return err
		}
		if len(stale) == 0 {
			return nil
		}
		paths := make([]string, len(stale))
		for i, install := range stale {
			paths[i] = install.Path
		}
		return fmt.Errorf("installed panecom skill is out of date: %s\n       run 'panecom skill update' to update", strings.Join(paths, ", "))
	}
	updated, skipped, err := updateSkills(home)
	if err != nil {
		return err
	}
	for _, path := range updated {
		fmt.Println("Updated " + path)
		if filepath.Base(path) == "SKILL.md" {
			warnUntrackedLegacy(home, path)
		}
	}
	for _, path := range skipped {
		fmt.Fprintf(os.Stderr, "panecom: skipped %s: edited since installation; inspect it, then reinstall with 'panecom skill install'\n", path)
	}
	if len(updated) == 0 && len(skipped) == 0 {
		fmt.Println("All tracked panecom skill installs are up to date.")
	}
	return nil
}

// skillNoticeInterval keeps the stale-skill reminder to about once a day. It
// is printed by commands agents run all the time, and repeating it on every
// call would bury their real output.
const skillNoticeInterval = 24 * time.Hour

func skillNoticePath(home string) string {
	return filepath.Join(home, ".config", "panecom", "skill-notice")
}

// staleSkillNotice returns a one-line reminder when a recorded install is
// stale and no reminder was given within skillNoticeInterval, and records
// that it was given. Any failure yields no reminder: this must never get in
// the way of the command it rides along with.
func staleSkillNotice(home string, now time.Time) string {
	if info, err := os.Stat(skillNoticePath(home)); err == nil && now.Sub(info.ModTime()) < skillNoticeInterval {
		return ""
	}
	manifest, err := readManifest(home)
	if err != nil {
		return ""
	}
	stale, err := staleInstalls(manifest)
	if err != nil || len(stale) == 0 {
		return ""
	}
	if err := os.MkdirAll(filepath.Dir(skillNoticePath(home)), 0700); err == nil {
		if err := os.WriteFile(skillNoticePath(home), []byte(now.UTC().Format(time.RFC3339)+"\n"), 0600); err == nil {
			_ = os.Chtimes(skillNoticePath(home), now, now)
		}
	}
	paths := make([]string, len(stale))
	for i, install := range stale {
		paths[i] = install.Path
	}
	sort.Strings(paths)
	return fmt.Sprintf("panecom: the installed panecom skill is older than this panecom (%s); run 'panecom skill update'", strings.Join(paths, ", "))
}

func noticeStaleSkills() {
	home, err := os.UserHomeDir()
	if err != nil {
		return
	}
	if notice := staleSkillNotice(home, time.Now()); notice != "" {
		fmt.Fprintln(os.Stderr, notice)
	}
}

// checkSkillsWithNewBinary runs 'skill update --check' with the binary that
// 'panecom update' just installed. This process still embeds the old skill,
// so only the new binary can tell whether the installed skill is behind it.
// Its message reaches stderr directly; its exit status is not an error here.
func checkSkillsWithNewBinary() {
	exe, err := os.Executable()
	if err != nil {
		return
	}
	cmd := exec.Command(exe, "skill", "update", "--check")
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	_ = cmd.Run()
}
