package main

import (
	"bufio"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"golang.org/x/term"
)

//go:embed .claude/skills/panecom.md
var embeddedSkill string

const (
	skillStart = "<!-- panecom-skill:start -->"
	skillEnd   = "<!-- panecom-skill:end -->"
)

type skillInstall struct {
	Agent      string `json:"agent"`
	Scope      string `json:"scope"`
	Path       string `json:"path"`
	Previous   string `json:"previous"`
	Existed    bool   `json:"existed"`
	Installed  string `json:"installed"`
	OldSection string `json:"old_section,omitempty"`
	AddedBlock string `json:"added_block,omitempty"`
	Adopted    bool   `json:"adopted,omitempty"`
}

type skillManifest struct {
	Installs []skillInstall `json:"installs"`
}

type fileSnapshot struct {
	path    string
	content string
	exists  bool
}

func restoreSnapshot(snapshot fileSnapshot) error {
	if snapshot.exists {
		return atomicWrite(snapshot.path, snapshot.content)
	}
	if err := os.Remove(snapshot.path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func rollbackSnapshots(snapshots []fileSnapshot) error {
	var first error
	for i := len(snapshots) - 1; i >= 0; i-- {
		if err := restoreSnapshot(snapshots[i]); err != nil && first == nil {
			first = err
		}
	}
	return first
}

func rollbackError(cause error, snapshots []fileSnapshot) error {
	if err := rollbackSnapshots(snapshots); err != nil {
		return fmt.Errorf("%w; restoring earlier files also failed: %v", cause, err)
	}
	return cause
}

func skillSection() (string, error) {
	const fence = "---\n"
	if !strings.HasPrefix(embeddedSkill, fence) {
		return "", fmt.Errorf("embedded panecom skill has no frontmatter")
	}
	end := strings.Index(embeddedSkill[len(fence):], "\n---\n")
	if end < 0 {
		return "", fmt.Errorf("embedded panecom skill has invalid frontmatter")
	}
	body := strings.TrimSpace(embeddedSkill[len(fence)+end+len("\n---\n"):])
	return skillStart + "\n\n" + body + "\n" + skillEnd, nil
}

func sectionBounds(content string) (int, int, error) {
	start := strings.Index(content, skillStart)
	if start < 0 {
		return -1, -1, nil
	}
	end := strings.Index(content[start+len(skillStart):], skillEnd)
	if end < 0 {
		return -1, -1, fmt.Errorf("panecom skill start marker has no end marker")
	}
	return start, start + len(skillStart) + end + len(skillEnd), nil
}

func replaceSkillSection(existing, section string) (string, string, string, error) {
	start, end, err := sectionBounds(existing)
	if err != nil {
		return "", "", "", err
	}
	if start >= 0 {
		return existing[:start] + section + existing[end:], existing[start:end], "", nil
	}
	separator := ""
	if existing != "" {
		separator = "\n\n"
		if strings.HasSuffix(existing, "\n") {
			separator = "\n"
		}
	}
	block := separator + section + "\n"
	return existing + block, "", block, nil
}

func manifestPath(home string) string {
	return filepath.Join(home, ".config", "panecom", "skill-installs.json")
}

func readManifest(home string) (skillManifest, error) {
	var m skillManifest
	data, err := os.ReadFile(manifestPath(home))
	if os.IsNotExist(err) {
		return m, nil
	}
	if err != nil {
		return m, err
	}
	if err := json.Unmarshal(data, &m); err != nil {
		return m, err
	}
	return m, nil
}

func writeManifest(home string, m skillManifest) error {
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	dir := filepath.Dir(manifestPath(home))
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, "skill-installs-*")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	if err := tmp.Chmod(0600); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(append(data, '\n')); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), manifestPath(home))
}

func skillTargets(home, project, agent, scope string) ([]skillInstall, error) {
	if home == "" || scope == "project" && project == "" {
		return nil, fmt.Errorf("skill install target directory is unavailable")
	}
	if scope != "user" && scope != "project" {
		return nil, fmt.Errorf("invalid skill scope %q", scope)
	}
	if agent != "claude" && agent != "codex" && agent != "both" {
		return nil, fmt.Errorf("invalid coding agent %q", agent)
	}
	base := home
	if scope == "project" {
		base = project
	}
	var targets []skillInstall
	if agent == "claude" || agent == "both" {
		targets = append(targets, skillInstall{Agent: "claude", Scope: scope, Path: filepath.Join(base, ".claude", "skills", "panecom.md")})
	}
	if agent == "codex" || agent == "both" {
		path := filepath.Join(base, ".codex", "instructions.md")
		if scope == "project" {
			path = filepath.Join(base, "AGENTS.md")
		}
		targets = append(targets, skillInstall{Agent: "codex", Scope: scope, Path: path})
	}
	return targets, nil
}

func contentHash(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

func readOptional(path string) (string, bool, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return string(data), true, nil
}

func installSkills(home string, targets []skillInstall) error {
	manifest, err := readManifest(home)
	if err != nil {
		return err
	}
	section, err := skillSection()
	if err != nil {
		return err
	}
	planned := make([]skillInstall, 0, len(targets))
	contents := make([]string, 0, len(targets))
	snapshots := make([]fileSnapshot, 0, len(targets))
	for _, target := range targets {
		current, exists, err := readOptional(target.Path)
		if err != nil {
			return fmt.Errorf("reading %s: %w", target.Path, err)
		}
		snapshots = append(snapshots, fileSnapshot{target.Path, current, exists})
		if index := findInstall(manifest, target.Path); index >= 0 {
			prior := manifest.Installs[index]
			if prior.Agent != target.Agent || prior.Scope != target.Scope {
				return fmt.Errorf("%s is already tracked for another target", target.Path)
			}
			if !matchesInstalled(current, exists, prior) {
				return fmt.Errorf("%s changed since installation; inspect it before reinstalling", target.Path)
			}
			target = prior // Keep the original content for uninstall.
		} else {
			target.Existed = exists
			if target.Agent == "claude" {
				if current == embeddedSkill {
					target.Adopted = true
					target.Existed = false
				} else {
					target.Previous = current
				}
			}
		}
		if target.Agent == "claude" {
			contents = append(contents, embeddedSkill)
			target.Installed = contentHash(embeddedSkill)
		} else {
			updated, oldSection, addedBlock, err := replaceSkillSection(current, section)
			if err != nil {
				return fmt.Errorf("updating %s: %w", target.Path, err)
			}
			contents = append(contents, updated)
			if findInstall(manifest, target.Path) < 0 {
				target.OldSection, target.AddedBlock = oldSection, addedBlock
				if oldSection == section {
					target.Adopted = true
					if strings.TrimSpace(strings.Replace(current, section, "", 1)) == "" {
						target.Existed = false
					}
				}
			} else if target.AddedBlock != "" {
				// Reinstallation may change the embedded section, so refresh the
				// block to remove while keeping the original separator.
				prefix := target.AddedBlock[:strings.Index(target.AddedBlock, skillStart)]
				target.AddedBlock = prefix + section + "\n"
			}
			target.Installed = contentHash(section)
		}
		planned = append(planned, target)
	}
	for i, target := range planned {
		if err := atomicWrite(target.Path, contents[i]); err != nil {
			return rollbackError(fmt.Errorf("installing %s: %w", target.Path, err), snapshots[:i])
		}
		if index := findInstall(manifest, target.Path); index >= 0 {
			manifest.Installs[index] = target
		} else {
			manifest.Installs = append(manifest.Installs, target)
		}
	}
	if err := writeManifest(home, manifest); err != nil {
		return rollbackError(fmt.Errorf("saving skill installations: %w", err), snapshots)
	}
	return nil
}

func findInstall(m skillManifest, path string) int {
	for i, install := range m.Installs {
		if install.Path == path {
			return i
		}
	}
	return -1
}

func matchesInstalled(current string, exists bool, install skillInstall) bool {
	if !exists {
		return false
	}
	if install.Agent == "claude" {
		return contentHash(current) == install.Installed
	}
	start, end, err := sectionBounds(current)
	return err == nil && start >= 0 && contentHash(current[start:end]) == install.Installed
}

func uninstallSkills(home string, paths []string) error {
	manifest, err := readManifest(home)
	if err != nil {
		return err
	}
	selected := make(map[string]bool, len(paths))
	for _, path := range paths {
		selected[path] = true
		if findInstall(manifest, path) < 0 {
			return fmt.Errorf("%s is not tracked as installed", path)
		}
	}
	restored := make(map[string]string, len(paths))
	var snapshots []fileSnapshot
	for _, install := range manifest.Installs {
		if !selected[install.Path] {
			continue
		}
		current, exists, err := readOptional(install.Path)
		if err != nil {
			return err
		}
		snapshots = append(snapshots, fileSnapshot{install.Path, current, exists})
		if !matchesInstalled(current, exists, install) {
			return fmt.Errorf("%s changed since installation; inspect it before uninstalling", install.Path)
		}
		if install.Agent == "claude" {
			restored[install.Path] = install.Previous
		} else if install.AddedBlock != "" {
			if !strings.Contains(current, install.AddedBlock) {
				return fmt.Errorf("%s no longer contains the installed block", install.Path)
			}
			restored[install.Path] = strings.Replace(current, install.AddedBlock, "", 1)
		} else if install.Adopted {
			start, end, _ := sectionBounds(current)
			restored[install.Path] = current[:start] + current[end:]
			if strings.TrimSpace(restored[install.Path]) == "" {
				restored[install.Path] = ""
			}
		} else {
			start, end, _ := sectionBounds(current)
			restored[install.Path] = current[:start] + install.OldSection + current[end:]
		}
	}
	changed := 0
	for _, install := range manifest.Installs {
		if !selected[install.Path] {
			continue
		}
		if !install.Existed && restored[install.Path] == "" {
			if err := os.Remove(install.Path); err != nil {
				return rollbackError(err, snapshots[:changed])
			}
		} else if err := atomicWrite(install.Path, restored[install.Path]); err != nil {
			return rollbackError(err, snapshots[:changed])
		}
		changed++
	}
	kept := manifest.Installs[:0]
	for _, install := range manifest.Installs {
		if !selected[install.Path] {
			kept = append(kept, install)
		}
	}
	manifest.Installs = kept
	if err := writeManifest(home, manifest); err != nil {
		return rollbackError(fmt.Errorf("saving skill installations: %w", err), snapshots)
	}
	return nil
}

func promptChoice(in *bufio.Reader, out io.Writer, label string, choices map[string]string) (string, error) {
	for {
		if _, err := fmt.Fprint(out, label); err != nil {
			return "", err
		}
		line, err := in.ReadString('\n')
		if err != nil {
			return "", err
		}
		choice := strings.ToLower(strings.TrimSpace(line))
		if value, ok := choices[choice]; ok {
			return value, nil
		}
		_, _ = fmt.Fprintln(out, "Choose one of the listed options.")
	}
}

func skillPromptFile() (*os.File, error) {
	return os.OpenFile("/dev/tty", os.O_RDWR, 0)
}

func agentDirectoryLabel(path string) string {
	info, err := os.Stat(path)
	if err == nil && info.IsDir() {
		return "found: " + path
	}
	if os.IsNotExist(err) || err == nil {
		return "not found: " + path
	}
	return "cannot check: " + path
}

func allSkillTargets(home, project string) ([]skillInstall, error) {
	var targets []skillInstall
	for _, agent := range []string{"claude", "codex"} {
		for _, scope := range []string{"user", "project"} {
			one, err := skillTargets(home, project, agent, scope)
			if err != nil {
				return nil, err
			}
			targets = append(targets, one...)
		}
	}
	return targets, nil
}

func displaySkillPath(target skillInstall) string {
	if target.Scope == "user" {
		if target.Agent == "claude" {
			return "~/.claude/skills/panecom.md"
		}
		return "~/.codex/instructions.md"
	}
	if target.Agent == "claude" {
		return "<project>/.claude/skills/panecom.md"
	}
	return "<project>/AGENTS.md"
}

func renderSkillPicker(home, project string, targets []skillInstall, checked []bool, cursor int, notice string) string {
	var b strings.Builder
	b.WriteString("\x1b[H\x1b[2Jpanecom skill install\r\n")
	b.WriteString("Arrows/jk: move  Space: check  Enter: toggle/submit  Esc/q: cancel\r\n\r\n")
	b.WriteString("Claude settings: " + agentDirectoryLabel(filepath.Join(home, ".claude")) + "\r\n")
	b.WriteString("Codex settings:  " + agentDirectoryLabel(filepath.Join(home, ".codex")) + "\r\n")
	b.WriteString("Project: " + project + "\r\n")
	b.WriteString("User = all projects; Project = this project\r\n\r\n")
	count := 0
	for i, target := range targets {
		pointer, mark := " ", " "
		if cursor == i {
			pointer = ">"
		}
		if checked[i] {
			mark = "x"
			count++
		}
		label := "Claude"
		if target.Agent == "codex" {
			label = "Codex"
		}
		scope := "User"
		if target.Scope == "project" {
			scope = "Project"
		}
		fmt.Fprintf(&b, "%s [%s] %-6s / %-7s  %s\r\n", pointer, mark, label, scope, displaySkillPath(target))
	}
	pointer := " "
	if cursor == len(targets) {
		pointer = ">"
	}
	fmt.Fprintf(&b, "\r\n%s [ Install selected (%d) ]\r\n", pointer, count)
	if notice != "" {
		b.WriteString(notice + "\r\n")
	}
	return b.String()
}

func readSkillKey(in io.Reader) (byte, error) {
	var b [1]byte
	if _, err := io.ReadFull(in, b[:]); err != nil {
		return 0, err
	}
	if b[0] != 27 {
		return b[0], nil
	}
	ready, err := inputReady(in, escapeSequenceTimeoutMS)
	if err != nil || !ready {
		return 27, err
	}
	if _, err := io.ReadFull(in, b[:]); err != nil {
		if err == io.EOF {
			return 27, nil
		}
		return 0, err
	}
	if b[0] != '[' && b[0] != 'O' {
		return 0, nil
	}
	for i := 0; i < 16; i++ {
		ready, err := inputReady(in, escapeSequenceTimeoutMS)
		if err != nil || !ready {
			return 0, err
		}
		if _, err := io.ReadFull(in, b[:]); err != nil {
			return 0, err
		}
		if b[0] >= '0' && b[0] <= '9' || b[0] == ';' {
			continue
		}
		return b[0], nil
	}
	return 0, nil
}

func pickSkillTargets(home, project string, targets []skillInstall, in io.Reader, out io.Writer) ([]skillInstall, bool, error) {
	checked := make([]bool, len(targets))
	cursor, notice := 0, ""
	for {
		if _, err := io.WriteString(out, renderSkillPicker(home, project, targets, checked, cursor, notice)); err != nil {
			return nil, false, err
		}
		key, err := readSkillKey(in)
		if err != nil {
			return nil, false, err
		}
		notice = ""
		switch key {
		case 'q', 3, 27:
			return nil, false, nil
		case 'j', 'B':
			if cursor < len(targets) {
				cursor++
			}
		case 'k', 'A':
			if cursor > 0 {
				cursor--
			}
		case '\t':
			cursor = len(targets)
		case ' ':
			if cursor < len(targets) {
				checked[cursor] = !checked[cursor]
			}
		case '\r', '\n':
			if cursor < len(targets) {
				checked[cursor] = !checked[cursor]
				break
			}
			var selected []skillInstall
			for i, target := range targets {
				if checked[i] {
					selected = append(selected, target)
				}
			}
			if len(selected) == 0 {
				notice = "Select at least one destination with Space."
				break
			}
			return selected, true, nil
		}
	}
}

func pickSkillTargetsTTY(tty *os.File, home, project string) ([]skillInstall, bool, error) {
	targets, err := allSkillTargets(home, project)
	if err != nil {
		return nil, false, err
	}
	oldState, err := term.MakeRaw(int(tty.Fd()))
	if err != nil {
		return nil, false, err
	}
	_, _ = fmt.Fprint(tty, "\x1b[?1049h\x1b[?25l")
	defer func() {
		_, _ = fmt.Fprint(tty, "\x1b[?25h\x1b[?1049l")
		_ = term.Restore(int(tty.Fd()), oldState)
	}()
	return pickSkillTargets(home, project, targets, tty, tty)
}

func cmdSkillInstall() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	tty, err := skillPromptFile()
	if err != nil {
		return fmt.Errorf("interactive skill install needs a terminal: %w", err)
	}
	defer func() { _ = tty.Close() }()
	targets, ok, err := pickSkillTargetsTTY(tty, home, namespaceAnchor())
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}
	if err := installSkills(home, targets); err != nil {
		return err
	}
	for _, target := range targets {
		fmt.Println("Installed " + target.Path)
	}
	return nil
}

func cmdSkillList() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	manifest, err := readManifest(home)
	if err != nil {
		return err
	}
	if len(manifest.Installs) == 0 {
		fmt.Println("No panecom skill installs tracked by this command.")
	}
	sort.Slice(manifest.Installs, func(i, j int) bool { return manifest.Installs[i].Path < manifest.Installs[j].Path })
	for _, install := range manifest.Installs {
		fmt.Printf("managed: %s (%s): %s\n", install.Agent, install.Scope, install.Path)
	}
	untracked, err := untrackedSkills(home, namespaceAnchor(), manifest)
	if err != nil {
		return err
	}
	for _, target := range untracked {
		fmt.Printf("untracked: %s (%s): %s\n", target.Agent, target.Scope, target.Path)
	}
	return nil
}

func untrackedSkills(home, project string, manifest skillManifest) ([]skillInstall, error) {
	var found []skillInstall
	seen := map[string]bool{}
	for _, scope := range []string{"user", "project"} {
		targets, err := skillTargets(home, project, "both", scope)
		if err != nil {
			return nil, err
		}
		for _, target := range targets {
			if seen[target.Path] || findInstall(manifest, target.Path) >= 0 {
				continue
			}
			seen[target.Path] = true
			content, exists, err := readOptional(target.Path)
			if err != nil {
				return nil, err
			}
			if !exists {
				continue
			}
			if target.Agent == "claude" {
				if strings.HasPrefix(content, "---\nname: panecom\n") {
					found = append(found, target)
				}
			} else if strings.Contains(content, skillStart) {
				found = append(found, target)
			}
		}
	}
	return found, nil
}

func cmdSkillUninstall() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	manifest, err := readManifest(home)
	if err != nil {
		return err
	}
	if len(manifest.Installs) == 0 {
		fmt.Println("No panecom skills tracked as installed.")
		return nil
	}
	tty, err := skillPromptFile()
	if err != nil {
		return fmt.Errorf("interactive skill uninstall needs a terminal: %w", err)
	}
	defer func() { _ = tty.Close() }()
	in := bufio.NewReader(tty)
	_, _ = fmt.Fprintln(tty, "Tracked installations:")
	choices := map[string]string{"a": "all", "all": "all", "": "cancel"}
	for i, install := range manifest.Installs {
		n := fmt.Sprint(i + 1)
		choices[n] = install.Path
		_, _ = fmt.Fprintf(tty, "  %s) %s (%s): %s\n", n, install.Agent, install.Scope, install.Path)
	}
	_, _ = fmt.Fprintln(tty, "  a) All")
	selection, err := promptChoice(in, tty, "Uninstall which? [Enter cancels]: ", choices)
	if err != nil || selection == "cancel" {
		return err
	}
	paths := []string{selection}
	if selection == "all" {
		paths = paths[:0]
		for _, install := range manifest.Installs {
			paths = append(paths, install.Path)
		}
	}
	for _, path := range paths {
		_, _ = fmt.Fprintln(tty, "  "+path)
	}
	confirm, err := promptChoice(in, tty, "Remove these installations? [y/N]: ", map[string]string{"y": "yes", "yes": "yes", "": "no", "n": "no", "no": "no"})
	if err != nil || confirm != "yes" {
		return err
	}
	if err := uninstallSkills(home, paths); err != nil {
		return err
	}
	for _, path := range paths {
		fmt.Println("Uninstalled " + path)
	}
	return nil
}
