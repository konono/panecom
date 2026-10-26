package main

import (
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/konono/panecom/update"
	"gopkg.in/yaml.v3"
)

var rolePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

var mux Multiplexer

func die(msg string) {
	fmt.Fprintf(os.Stderr, "panecom: %s\n", msg)
	os.Exit(1)
}

func hashString(s string) string {
	h := sha256.Sum256([]byte(s))
	return fmt.Sprintf("%x", h[:])
}

func stateRoot() string {
	return stateRootFor(namespaceAnchor())
}

// stateRootFor returns the store that serves the project rooted at anchor.
// Starting at the project root rather than the working directory keeps every
// pane of one project on the same store: a stale .panecom in a subdirectory
// would otherwise shadow the project's own store and split it in two.
//
// PANECOM_STATE_DIR overrides every project, including linked ones. It says
// all state lives in one store, where projects are still separated by
// namespace, so honouring it only for the local project would send a link
// looking into a store nobody writes to.
func stateRootFor(anchor string) string {
	if dir := os.Getenv("PANECOM_STATE_DIR"); dir != "" {
		return dir
	}
	for dir := anchor; ; {
		candidate := filepath.Join(dir, ".panecom")
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			return candidate
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return filepath.Join(anchor, ".panecom")
}

func sessionRoot(muxKind, sessionName string) string {
	return sessionRootIn(stateRoot(), muxKind, sessionName)
}

// sessionRootIn locates a session inside an arbitrary store, which is how a
// linked project's panes are reached: the session key is shared, the store is
// not.
func sessionRootIn(store, muxKind, sessionName string) string {
	var key string
	if muxKind == "zellij" {
		key = sessionName
	} else {
		key = muxKind + ":" + sessionName
	}
	return filepath.Join(store, "sessions", hashString(key))
}

func canonicalCwd() string {
	cwd, err := os.Getwd()
	if err != nil {
		die("cannot get working directory: " + err.Error())
	}
	abs, err := filepath.EvalSymlinks(cwd)
	if err != nil {
		abs = cwd
	}
	return abs
}

// canonicalHome returns the resolved home directory, or "" when it cannot be
// determined. Callers must treat "" as "unknown" and stay on the safe side
// rather than assuming no boundary exists.
func canonicalHome() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		if u, uerr := user.Current(); uerr == nil {
			home = u.HomeDir
		}
	}
	if home == "" {
		return ""
	}
	if abs, err := filepath.EvalSymlinks(home); err == nil {
		return abs
	}
	return home
}

// findMarkerUpward walks from dir towards the filesystem root and returns the
// first directory holding marker. It stops before stopAt (exclusive) and before
// the filesystem root; "" means no match.
func findMarkerUpward(start, marker, stopAt string) string {
	for dir := start; ; {
		parent := filepath.Dir(dir)
		if dir == stopAt || parent == dir {
			return ""
		}
		if _, err := os.Stat(filepath.Join(dir, marker)); err == nil {
			return dir
		}
		dir = parent
	}
}

// namespaceAnchor returns the directory that identifies the namespace of the
// current pane. It walks up from the working directory looking for a project
// root, so the namespace stays the same no matter where inside the project the
// working directory has drifted to.
//
// A Git root always wins over a .panecom directory, and the search for it
// covers the whole chain before .panecom is considered. Otherwise a stale
// .panecom left in a subdirectory by an older panecom would split one
// repository into two namespaces.
//
// The home directory and the filesystem root are never treated as project
// roots: a catch-all ~/.panecom is a shared state store, not a project, and
// anchoring there would merge unrelated projects into one namespace. When no
// project root is found the working directory itself is the anchor, which is
// the behaviour panecom had before anchoring existed.
func namespaceAnchor() string {
	return namespaceAnchorFrom(canonicalCwd(), canonicalHome())
}

// namespaceAnchorFrom is the pure form of namespaceAnchor. An empty home means
// the boundary is unknown.
func namespaceAnchorFrom(cwd, home string) string {
	// .git is a directory in a normal clone and a file in a worktree or
	// submodule; both mark a project root.
	if root := findMarkerUpward(cwd, ".git", home); root != "" {
		return root
	}
	if home == "" {
		// Without a home boundary a .panecom search could climb into a
		// catch-all store and merge unrelated projects, so don't run it.
		return cwd
	}
	if root := findMarkerUpward(cwd, ".panecom", home); root != "" {
		return root
	}
	return cwd
}

func validateRole(role string) {
	if !rolePattern.MatchString(role) {
		die(fmt.Sprintf("invalid role name '%s' (must match [A-Za-z0-9][A-Za-z0-9._-]*)", role))
	}
}

func atomicWrite(path, content string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(content), 0644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func readFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

func namespaceDir(sRoot, anchor string) string {
	return filepath.Join(sRoot, "namespaces", hashString(anchor))
}

func namespaceForCurrentPane(sRoot, paneID string) (string, error) {
	path := filepath.Join(sRoot, "panes", paneID)
	anchorHash, err := readFile(path)
	if err != nil {
		return "", fmt.Errorf("pane '%s' is not registered", paneID)
	}
	return filepath.Join(sRoot, "namespaces", anchorHash), nil
}

// externalsDir holds this namespace's links. Links live outside sessions/
// because they record paths rather than pane IDs, so they stay valid across
// multiplexer sessions. They are keyed by namespace because one store can
// serve several projects (a catch-all ~/.panecom does).
func externalsDir(nsHash string) string {
	return filepath.Join(stateRoot(), "externals", nsHash)
}

// currentNamespaceHash identifies the namespace the current pane acts in. A
// registered pane keeps the namespace recorded at register time so that a
// moved working directory cannot silently switch it.
func currentNamespaceHash(sRoot, paneID string) string {
	if paneID != "" {
		if recorded, err := readFile(filepath.Join(sRoot, "panes", paneID)); err == nil && recorded != "" {
			return recorded
		}
	}
	return hashString(namespaceAnchor())
}

// splitQualifiedRole splits "alias/role" into its parts. A bare role has no
// alias, which is why role names may not contain a slash.
func splitQualifiedRole(qualified string) (alias, role string) {
	if i := strings.LastIndex(qualified, "/"); i >= 0 {
		return qualified[:i], qualified[i+1:]
	}
	return "", qualified
}

// linkTarget returns the project root recorded for alias.
func linkTarget(nsHash, alias string) (string, error) {
	if !rolePattern.MatchString(alias) {
		return "", fmt.Errorf("invalid project alias '%s'", alias)
	}
	target, err := readFile(filepath.Join(externalsDir(nsHash), alias))
	if err != nil || target == "" {
		return "", fmt.Errorf("project '%s' is not linked (use: panecom link %s <path>)", alias, alias)
	}
	return target, nil
}

// externalNamespaceDir locates a linked project's namespace. The session is
// shared with the caller because pane IDs are only meaningful within one
// multiplexer session; the store and the namespace come from the target.
func externalNamespaceDir(session, targetAnchor string) string {
	store := stateRootFor(targetAnchor)
	return filepath.Join(sessionRootIn(store, mux.Kind(), session), "namespaces", hashString(targetAnchor))
}

type roleEntry struct {
	Role   string
	PaneID string
	Alive  bool
}

// listRoles reports the roles of a namespace together with pane liveness.
func listRoles(session, nsDir string) []roleEntry {
	entries, err := os.ReadDir(filepath.Join(nsDir, "roles"))
	if err != nil {
		return nil
	}
	var out []roleEntry
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		paneID, err := readFile(filepath.Join(nsDir, "roles", e.Name()))
		if err != nil || paneID == "" {
			continue
		}
		alive, err := mux.PaneExists(session, paneID)
		out = append(out, roleEntry{Role: e.Name(), PaneID: paneID, Alive: err == nil && alive})
	}
	return out
}

// describeNamespace names the namespace that was actually searched. The anchor
// recorded at register time is authoritative: recomputing it from the current
// working directory would report the wrong namespace in exactly the case worth
// diagnosing, where the pane was registered elsewhere and the directory moved.
func describeNamespace(nsDir string) string {
	if anchor, err := readFile(filepath.Join(nsDir, ".cwd")); err == nil && anchor != "" {
		return anchor
	}
	return nsDir
}

func resolveRole(nsDir, role string) (string, error) {
	path := filepath.Join(nsDir, "roles", role)
	paneID, err := readFile(path)
	if err != nil {
		return "", fmt.Errorf("role '%s' is not registered in this namespace", role)
	}
	return paneID, nil
}

func requireSession() (string, string) {
	session, err := mux.SessionName()
	if err != nil {
		die(err.Error())
	}
	return session, sessionRoot(mux.Kind(), session)
}

func requirePaneID() string {
	id, err := mux.CurrentPaneID()
	if err != nil {
		die(err.Error())
	}
	return id
}

func currentPaneIDOrEmpty() string {
	id, err := mux.CurrentPaneID()
	if err != nil {
		return ""
	}
	return id
}

func sendToPane(paneID, message string) error {
	if err := mux.SendKeys(paneID, message); err != nil {
		return err
	}
	time.Sleep(100 * time.Millisecond)
	return mux.SendEnter(paneID)
}

func cmdRegister(role string) {
	validateRole(role)
	session, _ := requireSession()
	paneID := requirePaneID()
	if err := registerPane(role, paneID, namespaceAnchor(), session); err != nil {
		die(err.Error())
	}
	fmt.Println(role)
}

// registerPane writes a pane mapping in the store for the selected project's
// anchor. Keeping this separate from cmdRegister lets the interactive picker
// register another pane without sending text to whatever runs inside it.
func registerPane(role, paneID, anchor, session string) error {
	if !rolePattern.MatchString(role) {
		return fmt.Errorf("invalid role name '%s' (must match [A-Za-z0-9][A-Za-z0-9._-]*)", role)
	}
	sRoot := sessionRootIn(stateRootFor(anchor), mux.Kind(), session)
	anchorHash := hashString(anchor)
	nsDir := namespaceDir(sRoot, anchor)

	if err := atomicWrite(filepath.Join(sRoot, ".session"), session); err != nil {
		return fmt.Errorf("failed to write session state: %w", err)
	}
	if err := atomicWrite(filepath.Join(nsDir, ".cwd"), anchor); err != nil {
		return fmt.Errorf("failed to write namespace state: %w", err)
	}

	prevNsDir, err := namespaceForCurrentPane(sRoot, paneID)
	if err == nil {
		entries, _ := os.ReadDir(filepath.Join(prevNsDir, "roles"))
		for _, e := range entries {
			content, _ := readFile(filepath.Join(prevNsDir, "roles", e.Name()))
			if content == paneID {
				_ = os.Remove(filepath.Join(prevNsDir, "roles", e.Name()))
			}
		}
	}

	oldPaneID, err := resolveRole(nsDir, role)
	if err == nil && oldPaneID != paneID {
		_ = os.Remove(filepath.Join(sRoot, "panes", oldPaneID))
		_ = os.Remove(filepath.Join(sRoot, "panes", oldPaneID+".bin"))
	}

	if err := atomicWrite(filepath.Join(nsDir, "roles", role), paneID); err != nil {
		return fmt.Errorf("failed to write role: %w", err)
	}
	if err := atomicWrite(filepath.Join(sRoot, "panes", paneID), anchorHash); err != nil {
		return fmt.Errorf("failed to write pane mapping: %w", err)
	}

	_ = mux.RenamePane(paneID, fmt.Sprintf("panecom:%s", role))
	return nil
}

func cmdOpen(role string, direction string, binPath string) {
	validateRole(role)
	myPaneID := requirePaneID()
	cwd := canonicalCwd()

	newPaneID, err := mux.NewPane(myPaneID, cwd, direction)
	if err != nil {
		die(err.Error())
	}

	time.Sleep(1 * time.Second)

	registerCmd := fmt.Sprintf("PANECOM_MUX=%s panecom register %s", mux.Kind(), role)
	if err := sendToPane(newPaneID, registerCmd); err != nil {
		die(fmt.Sprintf("failed to register in new pane: %s", err.Error()))
	}

	time.Sleep(300 * time.Millisecond)
	_ = mux.FocusPane(myPaneID)

	fmt.Println(newPaneID)
}

func cmdWhoami() {
	_, sRoot := requireSession()
	paneID := requirePaneID()

	nsDir, err := namespaceForCurrentPane(sRoot, paneID)
	if err != nil {
		die("this pane is not registered")
	}

	entries, err := os.ReadDir(filepath.Join(nsDir, "roles"))
	if err != nil {
		die("this pane is not registered")
	}

	for _, e := range entries {
		content, _ := readFile(filepath.Join(nsDir, "roles", e.Name()))
		if content == paneID {
			fmt.Println(e.Name())
			return
		}
	}
	die("this pane is not registered")
}

// resolveRoleForCommand accepts a bare role in the caller's own namespace or
// "alias/role" in a linked project.
func resolveRoleForCommand(qualified string) string {
	alias, role := splitQualifiedRole(qualified)
	validateRole(role)
	session, sRoot := requireSession()
	paneID := currentPaneIDOrEmpty()
	nsHash := currentNamespaceHash(sRoot, paneID)

	var nsDir, where string
	if alias == "" {
		nsDir = filepath.Join(sRoot, "namespaces", nsHash)
		where = describeNamespace(nsDir)
	} else {
		target, err := linkTarget(nsHash, alias)
		if err != nil {
			die(err.Error())
		}
		nsDir = externalNamespaceDir(session, target)
		// The link records the project root, so report that rather than the
		// namespace path: a linked project that never registered has no
		// namespace directory and therefore no .cwd to read back.
		where = target
	}

	targetPaneID, err := resolveRole(nsDir, role)
	if err != nil {
		die(fmt.Sprintf("role '%s' is not registered in namespace %s", role, where))
	}

	exists, err := mux.PaneExists(session, targetPaneID)
	if err != nil {
		die(fmt.Sprintf("failed to check pane '%s': %s", targetPaneID, err.Error()))
	}
	if !exists {
		// The pane mapping lives beside the namespace, which for a linked
		// project is the target's store rather than ours.
		panesDir := filepath.Join(filepath.Dir(filepath.Dir(nsDir)), "panes")
		_ = os.Remove(filepath.Join(nsDir, "roles", role))
		_ = os.Remove(filepath.Join(panesDir, targetPaneID))
		_ = os.Remove(filepath.Join(panesDir, targetPaneID+".bin"))
		die(fmt.Sprintf("role '%s' points to stale pane '%s'", role, targetPaneID))
	}

	return targetPaneID
}

// expandPath turns a user-supplied path into an absolute, symlink-free one.
// Relative paths are taken from the working directory, which is what a shell
// user typing ../other expects.
func expandPath(path string) (string, error) {
	return expandPathFrom(canonicalCwd(), path)
}

// expandPathFrom resolves path against base when it is relative.
func expandPathFrom(base, path string) (string, error) {
	if path == "~" || strings.HasPrefix(path, "~/") {
		home := canonicalHome()
		if home == "" {
			return "", fmt.Errorf("cannot expand '~': home directory is unknown")
		}
		path = filepath.Join(home, strings.TrimPrefix(strings.TrimPrefix(path, "~"), "/"))
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(base, path)
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return "", fmt.Errorf("no such directory: %s", abs)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("not a directory: %s", abs)
	}
	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		return resolved, nil
	}
	return abs, nil
}

func countAlive(entries []roleEntry) int {
	n := 0
	for _, e := range entries {
		if e.Alive {
			n++
		}
	}
	return n
}

func printRoles(entries []roleEntry) {
	for _, e := range entries {
		state := ""
		if !e.Alive {
			state = " (stale)"
		}
		fmt.Printf("  %-12s %s%s\n", e.Role, e.PaneID, state)
	}
}

// linkProject records alias and reports what it reached. Verifying the target
// here is the point of the command: a link that cannot be used should fail now
// rather than at the first send.
func linkProject(alias, path string, force bool) (string, []roleEntry, error) {
	if !rolePattern.MatchString(alias) {
		return "", nil, fmt.Errorf("invalid project alias '%s' (must match [A-Za-z0-9][A-Za-z0-9._-]*)", alias)
	}
	session, sRoot := requireSession()
	nsHash := currentNamespaceHash(sRoot, currentPaneIDOrEmpty())

	resolved, err := expandPath(path)
	if err != nil {
		return "", nil, err
	}
	// Normalise to the project root so that linking any directory inside the
	// target project reaches the same namespace its panes registered into.
	target := namespaceAnchorFrom(resolved, canonicalHome())
	if hashString(target) == nsHash {
		return "", nil, fmt.Errorf("cannot link a project to itself: %s", target)
	}

	// A link is only useful if something is actually reachable, so require a
	// live pane. Dead roles still get reported, they just do not count.
	roles := listRoles(session, externalNamespaceDir(session, target))
	if !force && countAlive(roles) == 0 {
		detail := "no roles registered"
		if len(roles) > 0 {
			detail = "no live roles"
		}
		return "", nil, fmt.Errorf("%s in %s (session: %s)\n       pass --force to link anyway", detail, target, session)
	}
	if err := atomicWrite(filepath.Join(externalsDir(nsHash), alias), target); err != nil {
		return "", nil, fmt.Errorf("failed to write link: %w", err)
	}
	return target, roles, nil
}

func cmdLink(alias, path string, force bool) {
	target, roles, err := linkProject(alias, path, force)
	if err != nil {
		die(err.Error())
	}
	fmt.Printf("%s -> %s\n", alias, target)
	printRoles(roles)
}

func cmdLinkList() {
	_, sRoot := requireSession()
	nsHash := currentNamespaceHash(sRoot, currentPaneIDOrEmpty())
	entries, err := os.ReadDir(externalsDir(nsHash))
	if err != nil {
		return
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		target, err := readFile(filepath.Join(externalsDir(nsHash), e.Name()))
		if err != nil {
			continue
		}
		fmt.Printf("%-12s -> %s\n", e.Name(), target)
	}
}

func unlinkProject(alias string) error {
	// Without this the alias is pasted straight into a path, so "../.." would
	// reach outside the links directory and delete files that are not links.
	if !rolePattern.MatchString(alias) {
		return fmt.Errorf("invalid project alias '%s' (must match [A-Za-z0-9][A-Za-z0-9._-]*)", alias)
	}
	_, sRoot := requireSession()
	nsHash := currentNamespaceHash(sRoot, currentPaneIDOrEmpty())
	path := filepath.Join(externalsDir(nsHash), alias)
	if _, err := os.Stat(path); err != nil {
		return fmt.Errorf("project '%s' is not linked", alias)
	}
	if err := os.Remove(path); err != nil {
		return fmt.Errorf("failed to remove link: %w", err)
	}
	return nil
}

func cmdUnlink(alias string) {
	if err := unlinkProject(alias); err != nil {
		die(err.Error())
	}
	fmt.Println(alias)
}

// cmdList shows the roles of this namespace, of one linked project, or of all
// of them.
func cmdList(alias string, all bool) {
	session, sRoot := requireSession()
	nsHash := currentNamespaceHash(sRoot, currentPaneIDOrEmpty())

	if alias != "" {
		target, err := linkTarget(nsHash, alias)
		if err != nil {
			die(err.Error())
		}
		fmt.Printf("%s -> %s\n", alias, target)
		printRoles(listRoles(session, externalNamespaceDir(session, target)))
		return
	}

	own := filepath.Join(sRoot, "namespaces", nsHash)
	fmt.Println(describeNamespace(own))
	printRoles(listRoles(session, own))

	if !all {
		return
	}
	entries, err := os.ReadDir(externalsDir(nsHash))
	if err != nil {
		return
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		target, err := readFile(filepath.Join(externalsDir(nsHash), e.Name()))
		if err != nil {
			continue
		}
		fmt.Printf("\n%s -> %s\n", e.Name(), target)
		printRoles(listRoles(session, externalNamespaceDir(session, target)))
	}
}

func cmdResolve(role string) {
	paneID := resolveRoleForCommand(role)
	fmt.Println(paneID)
}

func cmdDump(role string, full bool, lines int) {
	paneID := resolveRoleForCommand(role)
	screen, err := mux.DumpPane(paneID, full)
	if err != nil {
		die(fmt.Sprintf("failed to dump screen for '%s': %s", role, err.Error()))
	}
	if lines > 0 {
		screen = tailLines(screen, lines)
	}
	fmt.Print(screen)
}

func tailLines(s string, n int) string {
	s = strings.TrimRight(s, "\n")
	all := strings.Split(s, "\n")
	if len(all) <= n {
		return s + "\n"
	}
	return strings.Join(all[len(all)-n:], "\n") + "\n"
}

// Exit codes for send verification. 0 = delivered, 1 = hard error.
const (
	exitSendUnconfirmed  = 3 // screen did not change at all after sending
	exitSendNotSubmitted = 4 // message text still sitting in the input line
)

// normalizeScreen strips trailing whitespace so that cosmetic differences
// (padding, trailing spaces) do not count as a screen change.
func normalizeScreen(s string) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	for i, l := range lines {
		lines[i] = strings.TrimRight(l, " \t\r")
	}
	return strings.Join(lines, "\n")
}

// verifyFragment picks a distinctive tail of the message to look for in the
// recipient's input line. Returns "" when the message is too short to be a
// reliable marker.
func verifyFragment(message string) string {
	lines := strings.Split(message, "\n")
	frag := ""
	for i := len(lines) - 1; i >= 0; i-- {
		if t := strings.TrimSpace(lines[i]); t != "" {
			frag = t
			break
		}
	}
	if len([]rune(frag)) < 8 {
		return ""
	}
	r := []rune(frag)
	if len(r) > 40 {
		r = r[len(r)-40:]
	}
	return string(r)
}

// bottomDecoration are prompt and box-drawing characters that terminal UIs put
// around the input line. They sit between the wrapped halves of a message and
// must be ignored when looking for the message text.
const bottomDecoration = "│┃|>❯›⟩"

// squeezeForMatch removes whitespace and input-box decoration so that a message
// split across several screen lines by wrapping still matches.
func squeezeForMatch(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r == ' ' || r == '\t' || r == '\r' || r == '\n' {
			continue
		}
		if strings.ContainsRune(bottomDecoration, r) {
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// boxBorder are the characters that make up the frame drawn around an input
// box. verticalBorder is the subset that runs down the sides, so a textless
// line containing one is a row *inside* the box (an empty input line) rather
// than the top or bottom edge of it.
const (
	boxBorder      = "─━│┃╭╮╰╯┌┐└┘├┤═║╔╗╚╝╠╣+-"
	verticalBorder = "│┃║|"
)

// isHorizontalBorder reports whether a line is the top or bottom edge of a box
// and therefore carries neither text nor an input line.
func isHorizontalBorder(line string) bool {
	t := strings.TrimSpace(line)
	if t == "" {
		return false
	}
	for _, r := range t {
		if r != ' ' && !strings.ContainsRune(boxBorder, r) {
			return false
		}
		if strings.ContainsRune(verticalBorder, r) {
			return false
		}
	}
	return true
}

// bottomVisibleLines is how many content lines count as "the input area". A
// wrapped message can occupy several lines in a narrow pane, so this has to be
// more than one.
const bottomVisibleLines = 6

// fragmentAtInputEdge reports whether frag is still sitting unsent in the
// recipient's input line. The bottom lines are joined and stripped of
// whitespace and box decoration before matching, so a message wrapped across
// several lines in a narrow pane is still recognised.
//
// Two rules keep an already-accepted message from being misreported:
//
//   - An empty input line (a bare prompt, with or without a box around it)
//     means nothing is pending, whatever is echoed above it.
//   - Otherwise the fragment must sit at the very end of the input area, not
//     merely appear somewhere in it: an accepted message is echoed with a
//     status line or a fresh prompt below it.
//
// The cost is missing the residual in UIs that draw footer hints under the
// input line. That direction is deliberate: a false "not delivered" would make
// the sender repeat a message the recipient already has.
func fragmentAtInputEdge(screen, frag string) bool {
	if frag == "" {
		return false
	}
	lines := strings.Split(normalizeScreen(screen), "\n")

	// Walk up past blank lines and the bottom edge of an input box.
	i := len(lines) - 1
	for ; i >= 0; i-- {
		if strings.TrimSpace(lines[i]) == "" || isHorizontalBorder(lines[i]) {
			continue
		}
		break
	}
	if i < 0 {
		return false
	}
	// The input line holds no text: the message was consumed.
	if squeezeForMatch(lines[i]) == "" {
		return false
	}

	bottom := []string{}
	for ; i >= 0 && len(bottom) < bottomVisibleLines; i-- {
		if strings.TrimSpace(lines[i]) == "" || isHorizontalBorder(lines[i]) {
			continue
		}
		bottom = append([]string{lines[i]}, bottom...)
	}
	return strings.HasSuffix(squeezeForMatch(strings.Join(bottom, "")), squeezeForMatch(frag))
}

func cmdSend(role, message string, verify bool, verifyTimeout float64) {
	paneID := resolveRoleForCommand(role)
	code, screen, reason, err := sendAndVerify(role, paneID, message, verify, verifyTimeout)
	if err != nil {
		die(fmt.Sprintf("failed to send message to '%s': %s", role, err.Error()))
	}
	if code != 0 {
		sendFailure(role, screen, code, reason)
	}
}

// sendAndVerify sends the message and, when verify is set, watches the
// recipient's screen to decide whether it was accepted. It returns the exit
// code to use (0 when delivery looks confirmed or could not be checked), the
// last screen it saw and the reason for a non-zero code.
func sendAndVerify(role, paneID, message string, verify bool, verifyTimeout float64) (int, string, string, error) {
	before := ""
	if verify {
		screen, err := mux.DumpPane(paneID, false)
		if err != nil {
			// Verification is best-effort: a failing dump must not block sending.
			fmt.Fprintf(os.Stderr, "panecom: warning: cannot verify delivery to '%s': %s\n", role, err.Error())
			verify = false
		} else {
			before = normalizeScreen(screen)
		}
	}

	if err := sendToPane(paneID, message); err != nil {
		return 1, "", "", err
	}
	if !verify {
		return 0, "", "", nil
	}

	frag := verifyFragment(message)
	fragWasVisible := fragmentAtInputEdge(before, frag)

	deadline := time.Now().Add(time.Duration(verifyTimeout * float64(time.Second)))
	after := before
	changed := false
	for {
		time.Sleep(200 * time.Millisecond)
		screen, err := mux.DumpPane(paneID, false)
		if err != nil {
			fmt.Fprintf(os.Stderr, "panecom: warning: cannot verify delivery to '%s': %s\n", role, err.Error())
			return 0, "", "", nil
		}
		after = normalizeScreen(screen)
		if after != before {
			changed = true
			// Once the screen moved, give the recipient a moment to consume the
			// Enter before judging whether the text is still in the input line.
			if !fragmentAtInputEdge(after, frag) {
				return 0, after, "", nil
			}
		}
		if !time.Now().Before(deadline) {
			break
		}
	}

	if !changed {
		return exitSendUnconfirmed, after,
			"screen did not change after sending; the message was likely not accepted " +
				"(the recipient may be in a non-input mode such as a detail view, pager or dialog)", nil
	}
	if !fragWasVisible && fragmentAtInputEdge(after, frag) {
		return exitSendNotSubmitted, after,
			"message text is still in the input line; Enter did not submit it", nil
	}
	return 0, after, "", nil
}

func sendFailure(role, screen string, code int, msg string) {
	fmt.Fprintf(os.Stderr, "panecom: send to '%s' unverified: %s\n", role, msg)
	fmt.Fprintf(os.Stderr, "panecom: last 20 lines of '%s' screen:\n%s", role, tailLines(screen, 20))
	os.Exit(code)
}

func cmdShare(toRole string, full bool, lines int) {
	fromPaneID := requirePaneID()
	toPaneID := resolveRoleForCommand(toRole)

	_, sRoot := requireSession()
	fromRole := "unknown"
	if nsDir, err := namespaceForCurrentPane(sRoot, fromPaneID); err == nil {
		entries, _ := os.ReadDir(filepath.Join(nsDir, "roles"))
		for _, e := range entries {
			content, _ := readFile(filepath.Join(nsDir, "roles", e.Name()))
			if content == fromPaneID {
				fromRole = e.Name()
				break
			}
		}
	}

	screen, err := mux.DumpPane(fromPaneID, full)
	if err != nil {
		die(fmt.Sprintf("failed to dump own screen: %s", err.Error()))
	}

	if lines > 0 {
		screen = tailLines(screen, lines)
	}
	msg := fmt.Sprintf("[panecom:share from=%s]\n%s\n[/panecom:share]", fromRole, strings.TrimRight(screen, "\n"))
	if err := sendToPane(toPaneID, msg); err != nil {
		die(fmt.Sprintf("failed to share screen to '%s': %s", toRole, err.Error()))
	}
}

func execLockPath() string {
	return filepath.Join(stateRoot(), "exec.lock")
}

func acquireExecLock(token string) {
	lockDir := execLockPath()
	if err := os.MkdirAll(filepath.Dir(lockDir), 0755); err != nil {
		die("failed to create state dir: " + err.Error())
	}

	if err := os.Mkdir(lockDir, 0755); err == nil {
		if wErr := os.WriteFile(filepath.Join(lockDir, "token"), []byte(token), 0644); wErr != nil {
			_ = os.RemoveAll(lockDir)
			die("failed to write lock token: " + wErr.Error())
		}
		return
	}

	oldToken, readErr := readFile(filepath.Join(lockDir, "token"))
	if readErr != nil {
		die("another exec is already running")
	}
	oldExitcode := filepath.Join(stateRoot(), "exec", oldToken, "exitcode")
	if _, statErr := os.Stat(oldExitcode); statErr != nil {
		die("another exec is already running")
	}

	recoverDir := lockDir + ".recover." + token
	if err := os.Rename(lockDir, recoverDir); err != nil {
		die("another exec is already running")
	}
	_ = os.RemoveAll(recoverDir)

	if err := os.Mkdir(lockDir, 0755); err != nil {
		die("another exec is already running")
	}
	if wErr := os.WriteFile(filepath.Join(lockDir, "token"), []byte(token), 0644); wErr != nil {
		_ = os.RemoveAll(lockDir)
		die("failed to write lock token: " + wErr.Error())
	}
}

func execDieWithCleanup(token, execDir, msg string) {
	exitcodeFile := filepath.Join(execDir, "exitcode")
	if _, err := os.Stat(exitcodeFile); err != nil {
		_ = os.MkdirAll(execDir, 0755)
		if wErr := os.WriteFile(exitcodeFile, []byte("1"), 0644); wErr != nil {
			fmt.Fprintf(os.Stderr, "panecom: failed to write exit code for cleanup: %s\n", wErr.Error())
		}
	}
	die(msg)
}

func cmdExecRemote(role, command string, timeoutSec float64) {
	paneID := resolveRoleForCommand(role)

	token := fmt.Sprintf("%d.%d", os.Getpid(), time.Now().UnixNano())

	acquireExecLock(token)

	execBase := filepath.Join(stateRoot(), "exec")
	if err := os.MkdirAll(execBase, 0755); err != nil {
		execDieWithCleanup(token, filepath.Join(execBase, token), "failed to create exec base: "+err.Error())
	}
	execDir := filepath.Join(execBase, token)
	if err := os.MkdirAll(execDir, 0755); err != nil {
		execDieWithCleanup(token, execDir, "failed to create exec dir: "+err.Error())
	}

	currentLink := filepath.Join(execBase, "current")
	_ = os.Remove(currentLink)
	if err := os.Symlink(token, currentLink); err != nil {
		fmt.Fprintf(os.Stderr, "panecom: failed to create progress symlink: %s\n", err.Error())
	}

	stdoutFile := filepath.Join(execDir, "stdout")
	stderrFile := filepath.Join(execDir, "stderr")
	exitcodeFile := filepath.Join(execDir, "exitcode")

	if err := atomicWrite(filepath.Join(execDir, "command"), command); err != nil {
		execDieWithCleanup(token, execDir, "failed to write command: "+err.Error())
	}

	entries, _ := os.ReadDir(execBase)
	for _, e := range entries {
		if e.Name() == token || e.Name() == "current" {
			continue
		}
		completed := filepath.Join(execBase, e.Name(), "exitcode")
		if _, err := os.Stat(completed); err == nil {
			_ = os.RemoveAll(filepath.Join(execBase, e.Name()))
		}
	}

	escaped := strings.ReplaceAll(command, `\`, `\\`)
	escaped = strings.ReplaceAll(escaped, "'", `\'`)
	quotedCmd := "$'" + escaped + "'"
	if err := sendToPane(paneID, fmt.Sprintf(" PANECOM_TOKEN=%s panecom exec -- %s", token, quotedCmd)); err != nil {
		execDieWithCleanup(token, execDir, fmt.Sprintf("failed to send command to '%s': %s", role, err.Error()))
	}

	timeout := time.Duration(timeoutSec * float64(time.Second))
	deadline := time.Now().Add(timeout)
	pollInterval := 200 * time.Millisecond

	for {
		if _, err := os.Stat(exitcodeFile); err == nil {
			break
		}
		if time.Now().After(deadline) {
			die(fmt.Sprintf("timeout waiting for command to complete on '%s' (%.0fs)", role, timeoutSec))
		}
		time.Sleep(pollInterval)
	}

	time.Sleep(100 * time.Millisecond)

	exitCodeStr, err := readFile(exitcodeFile)
	if err != nil {
		die("failed to read exit code: " + err.Error())
	}
	exitCode, _ := strconv.Atoi(exitCodeStr)

	if data, err := os.ReadFile(stdoutFile); err == nil {
		_, _ = os.Stdout.Write(data)
	}
	if data, err := os.ReadFile(stderrFile); err == nil {
		_, _ = os.Stderr.Write(data)
	}

	if exitCode != 0 {
		os.Exit(exitCode)
	}
}

func runnerDie(exitcodeFile string, code int, msg string) {
	if err := os.WriteFile(exitcodeFile, []byte(strconv.Itoa(code)), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "panecom: failed to write exit code: %s\n", err.Error())
	}
	die(msg)
}

func cmdExecRun(token string) {
	if token == "" {
		die("PANECOM_TOKEN not set")
	}

	execDir := filepath.Join(stateRoot(), "exec", token)
	exitcodeFile := filepath.Join(execDir, "exitcode")

	if _, err := os.Stat(execDir); err != nil {
		_ = os.MkdirAll(execDir, 0755)
		runnerDie(exitcodeFile, 1, fmt.Sprintf("exec dir not found for token: %s", token))
	}

	lockToken, err := readFile(filepath.Join(execLockPath(), "token"))
	if err != nil || lockToken != token {
		runnerDie(exitcodeFile, 1, "exec token mismatch — stale or conflicting request")
	}

	command, err := readFile(filepath.Join(execDir, "command"))
	if err != nil {
		runnerDie(exitcodeFile, 1, "failed to read command: "+err.Error())
	}

	stdoutFile := filepath.Join(execDir, "stdout")
	stderrFile := filepath.Join(execDir, "stderr")

	outFile, err := os.Create(stdoutFile)
	if err != nil {
		runnerDie(exitcodeFile, 1, "failed to create stdout file: "+err.Error())
	}
	defer func() { _ = outFile.Close() }()

	errFile, err := os.Create(stderrFile)
	if err != nil {
		runnerDie(exitcodeFile, 1, "failed to create stderr file: "+err.Error())
	}
	defer func() { _ = errFile.Close() }()

	cmd := exec.Command("sh", "-c", command)
	cmd.Env = append(os.Environ(),
		"PAGER=cat",
		"SYSTEMD_PAGER=cat",
		"GIT_PAGER=cat",
		"DEBIAN_FRONTEND=noninteractive",
		"GIT_TERMINAL_PROMPT=0",
		"SSH_BATCH_MODE=yes",
		"PYTHONDONTWRITEBYTECODE=1",
	)
	cmd.Stdin = nil

	stdoutPR, stdoutPW, err := os.Pipe()
	if err != nil {
		runnerDie(exitcodeFile, 1, "failed to create stdout pipe: "+err.Error())
	}
	stderrPR, stderrPW, err := os.Pipe()
	if err != nil {
		_ = stdoutPW.Close()
		_ = stdoutPR.Close()
		runnerDie(exitcodeFile, 1, "failed to create stderr pipe: "+err.Error())
	}

	cmd.Stdout = stdoutPW
	cmd.Stderr = stderrPW

	if err := cmd.Start(); err != nil {
		_ = stdoutPW.Close()
		_ = stderrPW.Close()
		_ = stdoutPR.Close()
		_ = stderrPR.Close()
		fmt.Fprintf(os.Stderr, "panecom: failed to start command: %s\n", err.Error())
		if wErr := os.WriteFile(exitcodeFile, []byte("127"), 0644); wErr != nil {
			fmt.Fprintf(os.Stderr, "panecom: failed to write exit code: %s\n", wErr.Error())
		}
		return
	}

	_ = stdoutPW.Close()
	_ = stderrPW.Close()

	stdoutErrCh := make(chan error, 1)
	stderrErrCh := make(chan error, 1)

	go func() {
		var writeErr error
		buf := make([]byte, 4096)
		for {
			n, readErr := stdoutPR.Read(buf)
			if n > 0 {
				_, _ = os.Stdout.Write(buf[:n])
				if _, err := outFile.Write(buf[:n]); err != nil && writeErr == nil {
					writeErr = err
				}
			}
			if readErr != nil {
				break
			}
		}
		_ = stdoutPR.Close()
		stdoutErrCh <- writeErr
	}()

	go func() {
		var writeErr error
		buf := make([]byte, 4096)
		for {
			n, readErr := stderrPR.Read(buf)
			if n > 0 {
				_, _ = os.Stderr.Write(buf[:n])
				if _, err := errFile.Write(buf[:n]); err != nil && writeErr == nil {
					writeErr = err
				}
			}
			if readErr != nil {
				break
			}
		}
		_ = stderrPR.Close()
		stderrErrCh <- writeErr
	}()

	stdoutWriteErr := <-stdoutErrCh
	stderrWriteErr := <-stderrErrCh

	exitCode := 0
	if err := cmd.Wait(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = 1
		}
	}

	protocolErr := false
	if err := outFile.Close(); err != nil {
		fmt.Fprintf(os.Stderr, "panecom: failed to close stdout file: %s\n", err.Error())
		protocolErr = true
	}
	if err := errFile.Close(); err != nil {
		fmt.Fprintf(os.Stderr, "panecom: failed to close stderr file: %s\n", err.Error())
		protocolErr = true
	}
	if stdoutWriteErr != nil {
		fmt.Fprintf(os.Stderr, "panecom: stdout write error: %s\n", stdoutWriteErr.Error())
		protocolErr = true
	}
	if stderrWriteErr != nil {
		fmt.Fprintf(os.Stderr, "panecom: stderr write error: %s\n", stderrWriteErr.Error())
		protocolErr = true
	}
	if protocolErr && exitCode == 0 {
		exitCode = 1
	}
	if err := os.WriteFile(exitcodeFile, []byte(strconv.Itoa(exitCode)), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "panecom: failed to write exit code: %s\n", err.Error())
	}
}

func shellQuote(s string) string {
	if s == "" {
		return "''"
	}
	for _, c := range s {
		if (c < 'a' || c > 'z') && (c < 'A' || c > 'Z') && (c < '0' || c > '9') && c != '/' && c != '.' && c != '_' && c != '-' {
			return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'"
		}
	}
	return s
}

type paneConfig struct {
	Role       string `yaml:"role"`
	Cmd        string `yaml:"cmd"`
	Direction  string `yaml:"direction"`
	Foreground bool   `yaml:"foreground"`
}

type profileConfig struct {
	Panes []paneConfig `yaml:"panes"`
	// ExternalProjects are materialised into links when the profile launches,
	// so a recurring collaboration does not have to be linked by hand each
	// time. Paths are resolved relative to the config file's project root.
	ExternalProjects map[string]string `yaml:"external_projects"`
}

type config struct {
	Profiles map[string]profileConfig `yaml:"profiles"`
}

var configFileNames = []string{"config.yaml", "config.yml"}

func globalConfigDir() string {
	if dir := os.Getenv("XDG_CONFIG_HOME"); dir != "" {
		return filepath.Join(dir, "panecom")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "panecom")
}

func findConfigFile(dir string) string {
	for _, name := range configFileNames {
		p := filepath.Join(dir, name)
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

// findProjectConfig searches from the project root rather than the working
// directory, matching stateRoot so that one project always reads one config
// even when the working directory has moved into a subdirectory.
func findProjectConfig() string {
	for dir := namespaceAnchor(); ; {
		panecomDir := filepath.Join(dir, ".panecom")
		if path := findConfigFile(panecomDir); path != "" {
			return path
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return ""
}

func parseConfig(data []byte) (*config, error) {
	var cfg config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}
	if cfg.Profiles == nil {
		cfg.Profiles = make(map[string]profileConfig)
	}
	return &cfg, nil
}

func loadConfig() (*config, error) {
	var globalCfg *config
	if path := findConfigFile(globalConfigDir()); path != "" {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("reading global config %s: %w", path, err)
		}
		globalCfg, err = parseConfig(data)
		if err != nil {
			return nil, fmt.Errorf("parsing global config %s: %w", path, err)
		}
	}

	var projectCfg *config
	if path := findProjectConfig(); path != "" {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("reading project config %s: %w", path, err)
		}
		projectCfg, err = parseConfig(data)
		if err != nil {
			return nil, fmt.Errorf("parsing project config %s: %w", path, err)
		}
	}

	merged := &config{Profiles: make(map[string]profileConfig)}
	if globalCfg != nil {
		for k, v := range globalCfg.Profiles {
			merged.Profiles[k] = v
		}
	}
	if projectCfg != nil {
		for k, v := range projectCfg.Profiles {
			merged.Profiles[k] = v
		}
	}

	if len(merged.Profiles) == 0 {
		locations := []string{
			filepath.Join(globalConfigDir(), "config.yaml"),
			".panecom/config.yaml",
		}
		return nil, fmt.Errorf("no config found (searched: %s)", strings.Join(locations, ", "))
	}

	return merged, nil
}

// materializeExternalProjects turns a profile's declared collaborations into
// links. Targets are usually not running yet when a profile launches, so a
// missing role is not an error here; resolution still verifies at send time.
// Failures are reported but never block the launch.
func materializeExternalProjects(profile profileConfig) {
	if len(profile.ExternalProjects) == 0 {
		return
	}
	// Anchor, not the launching pane's namespace: the panes this profile is
	// about to start register from this working directory, so the links have
	// to land where they will look for them. A pane registered in another
	// project would otherwise divert them to that project's namespace.
	anchor := namespaceAnchor()
	nsHash := hashString(anchor)

	for _, alias := range sortedKeys(profile.ExternalProjects) {
		path := profile.ExternalProjects[alias]
		if !rolePattern.MatchString(alias) {
			fmt.Fprintf(os.Stderr, "panecom: skipping invalid project alias '%s'\n", alias)
			continue
		}
		resolved, err := expandPathFrom(anchor, path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "panecom: skipping project '%s': %s\n", alias, err.Error())
			continue
		}
		target := namespaceAnchorFrom(resolved, canonicalHome())
		if hashString(target) == nsHash {
			fmt.Fprintf(os.Stderr, "panecom: skipping project '%s': refers to this project\n", alias)
			continue
		}
		if err := atomicWrite(filepath.Join(externalsDir(nsHash), alias), target); err != nil {
			fmt.Fprintf(os.Stderr, "panecom: skipping project '%s': %s\n", alias, err.Error())
			continue
		}
		fmt.Printf("%s -> %s\n", alias, target)
	}
}

func sortedKeys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func cmdProfile(name string, binPath string, focus bool) {
	cfg, err := loadConfig()
	if err != nil {
		die(err.Error())
	}

	profile, ok := cfg.Profiles[name]
	if !ok {
		available := make([]string, 0, len(cfg.Profiles))
		for k := range cfg.Profiles {
			available = append(available, k)
		}
		die(fmt.Sprintf("profile '%s' not found (available: %s)", name, strings.Join(available, ", ")))
	}

	if len(profile.Panes) == 0 {
		die(fmt.Sprintf("profile '%s' has no panes defined", name))
	}

	_ = binPath

	materializeExternalProjects(profile)

	cwd := canonicalCwd()
	myPaneID := currentPaneIDOrEmpty()

	seen := make(map[string]bool)
	for _, p := range profile.Panes {
		validateRole(p.Role)
		if seen[p.Role] {
			die(fmt.Sprintf("duplicate role '%s' in profile '%s'", p.Role, name))
		}
		seen[p.Role] = true
	}

	tabID, firstPaneID, err := mux.NewTab(name, cwd)
	if err != nil {
		die(err.Error())
	}

	type createdPane struct {
		paneID string
		cfg    paneConfig
	}
	var panes []createdPane
	var foregroundPaneID string

	for i, p := range profile.Panes {
		var paneID string
		if i == 0 {
			paneID = firstPaneID
		} else {
			dir := p.Direction
			if dir == "" {
				dir = "right"
			}
			newPaneID, err := mux.NewPaneInTab(tabID, cwd, dir)
			if err != nil {
				die(fmt.Sprintf("failed to create pane for role '%s': %s", p.Role, err.Error()))
			}
			paneID = newPaneID
		}

		if paneID == "" {
			die(fmt.Sprintf("failed to get pane ID for role '%s'", p.Role))
		}

		panes = append(panes, createdPane{paneID: paneID, cfg: p})

		if p.Foreground {
			foregroundPaneID = paneID
		}
	}

	time.Sleep(1 * time.Second)

	for _, p := range panes {
		registerCmd := fmt.Sprintf("cd %q && PANECOM_MUX=%s panecom register %s", cwd, mux.Kind(), p.cfg.Role)
		if err := sendToPane(p.paneID, registerCmd); err != nil {
			die(fmt.Sprintf("failed to register role '%s': %s", p.cfg.Role, err.Error()))
		}
		time.Sleep(500 * time.Millisecond)

		if p.cfg.Cmd != "" {
			if err := sendToPane(p.paneID, p.cfg.Cmd); err != nil {
				die(fmt.Sprintf("failed to start cmd for '%s': %s", p.cfg.Role, err.Error()))
			}
			time.Sleep(500 * time.Millisecond)
		}
	}

	if focus {
		if foregroundPaneID != "" {
			_ = mux.FocusPane(foregroundPaneID)
		}
	} else {
		if myPaneID != "" {
			_ = mux.FocusPane(myPaneID)
		}
	}

	for _, p := range panes {
		marker := " "
		if p.cfg.Foreground {
			marker = "*"
		}
		fmt.Fprintf(os.Stderr, " %s %s → %s", marker, p.cfg.Role, p.paneID)
		if p.cfg.Cmd != "" {
			fmt.Fprintf(os.Stderr, " (%s)", p.cfg.Cmd)
		}
		fmt.Fprintln(os.Stderr)
	}
}

func usage() {
	fmt.Fprintf(os.Stderr, `Usage:
  panecom [--mux zellij|tmux] <command> [args...]

Commands:
  register                       Choose a pane and role interactively
  register <role>                Register current pane as <role>
  whoami                         Show current pane's role
  list [--all | <project>]       List roles here, or in a linked project
  link [-f] <project> <path>     Link a project so its roles can be addressed
                                 as <project>/<role>; no args lists links
  unlink <project>               Remove a link
  resolve <role>                 Show pane ID for <role>
  open [-d right|down] <role>    Open new pane and register as <role>
  dump [--full] [-l N] <role>    Dump screen of <role>'s pane
  send [--no-verify] [--verify-timeout N] <role> <msg>
                                 Send message to <role>'s pane
                                 exit 3: unconfirmed (screen unchanged)
                                 exit 4: not submitted (text left in input line)
  share [--full] [-l N] <role>    Share this pane's screen with <role>
  exec [--timeout N] <role> <cmd> Run command on <role>, return output
  profile [-f] <name>            Launch profile from .panecom/config.yaml
  update                         Update panecom to the latest version
  --version                      Show version

Multiplexer selection (in priority order):
  --mux flag > PANECOM_MUX env > auto-detect (ZELLIJ_SESSION_NAME > TMUX)
`)
	os.Exit(1)
}

func main() {
	args := os.Args[1:]
	if len(args) == 0 {
		usage()
	}

	var muxFlag string
	if args[0] == "--mux" {
		if len(args) < 3 {
			usage()
		}
		muxFlag = args[1]
		args = args[2:]
	}

	switch args[0] {
	case "--version":
		fmt.Printf("panecom %s\n", Version)
		return
	case "update":
		if err := update.Run(Version); err != nil {
			die(err.Error())
		}
		return
	case "exec":
		rest := args[1:]
		if len(rest) >= 1 && rest[0] == "--" {
			cmdExecRun(os.Getenv("PANECOM_TOKEN"))
			return
		}
	}

	var err error
	mux, err = DetectMultiplexer(muxFlag, nil)
	if err != nil {
		die(err.Error())
	}

	switch args[0] {
	case "register":
		switch len(args) {
		case 1:
			cmdRegisterInteractive()
		case 2:
			cmdRegister(args[1])
		default:
			usage()
		}
	case "whoami":
		if len(args) != 1 {
			usage()
		}
		cmdWhoami()
	case "link":
		force := false
		positional := []string{}
		for _, a := range args[1:] {
			if a == "-f" || a == "--force" {
				force = true
			} else {
				positional = append(positional, a)
			}
		}
		switch len(positional) {
		case 0:
			cmdLinkList()
		case 2:
			cmdLink(positional[0], positional[1], force)
		default:
			usage()
		}
	case "unlink":
		if len(args) != 2 {
			usage()
		}
		cmdUnlink(args[1])
	case "list":
		all := false
		alias := ""
		for _, a := range args[1:] {
			if a == "--all" {
				all = true
			} else if alias == "" {
				alias = a
			} else {
				usage()
			}
		}
		if alias != "" && all {
			usage()
		}
		cmdList(alias, all)
	case "open":
		direction := ""
		binPath := ""
		role := ""
		rest := args[1:]
		for i := 0; i < len(rest); i++ {
			if (rest[i] == "-d" || rest[i] == "--direction") && i+1 < len(rest) {
				direction = rest[i+1]
				i++
			} else if rest[i] == "--bin" && i+1 < len(rest) {
				binPath = rest[i+1]
				i++
			} else {
				role = rest[i]
			}
		}
		if role == "" {
			usage()
		}
		cmdOpen(role, direction, binPath)
	case "resolve":
		if len(args) != 2 {
			usage()
		}
		cmdResolve(args[1])
	case "dump":
		full := false
		lines := 0
		role := ""
		rest := args[1:]
		for i := 0; i < len(rest); i++ {
			if rest[i] == "--full" {
				full = true
			} else if (rest[i] == "-l" || rest[i] == "--lines") && i+1 < len(rest) {
				n, err := strconv.Atoi(rest[i+1])
				if err != nil || n <= 0 {
					die("--lines requires a positive integer")
				}
				lines = n
				i++
			} else {
				role = rest[i]
			}
		}
		if role == "" {
			usage()
		}
		cmdDump(role, full, lines)
	case "send":
		verify := true
		verifyTimeout := 3.0
		positional := []string{}
		rest := args[1:]
		for i := 0; i < len(rest); i++ {
			if rest[i] == "--no-verify" {
				verify = false
			} else if rest[i] == "--verify-timeout" && i+1 < len(rest) {
				t, err := strconv.ParseFloat(rest[i+1], 64)
				if err != nil || t <= 0 {
					die("--verify-timeout requires a positive number")
				}
				verifyTimeout = t
				i++
			} else {
				positional = append(positional, rest[i:]...)
				break
			}
		}
		if len(positional) < 2 {
			usage()
		}
		cmdSend(positional[0], strings.Join(positional[1:], " "), verify, verifyTimeout)
	case "share":
		full := false
		lines := 0
		role := ""
		rest := args[1:]
		for i := 0; i < len(rest); i++ {
			if rest[i] == "--full" {
				full = true
			} else if (rest[i] == "-l" || rest[i] == "--lines") && i+1 < len(rest) {
				n, err := strconv.Atoi(rest[i+1])
				if err != nil || n <= 0 {
					die("--lines requires a positive integer")
				}
				lines = n
				i++
			} else {
				role = rest[i]
			}
		}
		if role == "" {
			usage()
		}
		cmdShare(role, full, lines)
	case "exec":
		rest := args[1:]
		timeoutSec := 30.0
		positional := []string{}
		for i := 0; i < len(rest); i++ {
			if rest[i] == "--timeout" && i+1 < len(rest) {
				t, err := strconv.ParseFloat(rest[i+1], 64)
				if err != nil {
					die("invalid --timeout value: " + rest[i+1])
				}
				timeoutSec = t
				i++
			} else {
				positional = append(positional, rest[i])
			}
		}
		if len(positional) < 2 {
			usage()
		}
		cmdExecRemote(positional[0], strings.Join(positional[1:], " "), timeoutSec)
	case "profile":
		binPath := ""
		name := ""
		focus := false
		rest := args[1:]
		for i := 0; i < len(rest); i++ {
			if rest[i] == "--bin" && i+1 < len(rest) {
				binPath = rest[i+1]
				i++
			} else if rest[i] == "-f" || rest[i] == "--focus" {
				focus = true
			} else {
				name = rest[i]
			}
		}
		if name == "" {
			usage()
		}
		cmdProfile(name, binPath, focus)
	default:
		usage()
	}
}
