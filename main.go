package main

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

var rolePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

func die(msg string) {
	fmt.Fprintf(os.Stderr, "panecom: %s\n", msg)
	os.Exit(1)
}

func hashString(s string) string {
	h := sha256.Sum256([]byte(s))
	return fmt.Sprintf("%x", h[:])
}

func stateRoot() string {
	if dir := os.Getenv("PANECOM_STATE_DIR"); dir != "" {
		return dir
	}
	cwd := canonicalCwd()
	dir := cwd
	for {
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
	return filepath.Join(cwd, ".panecom")
}

func sessionRoot(sessionName string) string {
	return filepath.Join(stateRoot(), "sessions", hashString(sessionName))
}

func zellijSessionName() string {
	s := os.Getenv("ZELLIJ_SESSION_NAME")
	if s == "" {
		die("not running inside Zellij (ZELLIJ_SESSION_NAME not set)")
	}
	return s
}

func currentPaneID() string {
	id := os.Getenv("ZELLIJ_PANE_ID")
	if id == "" {
		return ""
	}
	return "terminal_" + id
}

func requireCurrentPaneID() string {
	id := currentPaneID()
	if id == "" {
		die("ZELLIJ_PANE_ID not set")
	}
	return id
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

type zellijPane struct {
	ID       int    `json:"id"`
	IsPlugin bool   `json:"is_plugin"`
	TabID    int    `json:"tab_id"`
	TabName  string `json:"tab_name"`
}

func listTerminalPanes() ([]zellijPane, error) {
	out, err := exec.Command("zellij", "action", "list-panes", "--json", "-t").Output()
	if err != nil {
		return nil, err
	}
	var all []zellijPane
	if err := json.Unmarshal(out, &all); err != nil {
		return nil, err
	}
	var terminals []zellijPane
	for _, p := range all {
		if !p.IsPlugin {
			terminals = append(terminals, p)
		}
	}
	return terminals, nil
}

func paneExists(paneID string) (bool, error) {
	numStr := strings.TrimPrefix(paneID, "terminal_")
	num, err := strconv.Atoi(numStr)
	if err != nil {
		return false, fmt.Errorf("invalid pane ID: %s", paneID)
	}
	panes, err := listTerminalPanes()
	if err != nil {
		return false, fmt.Errorf("failed to list panes: %w", err)
	}
	for _, p := range panes {
		if p.ID == num {
			return true, nil
		}
	}
	return false, nil
}

func namespaceDir(sRoot, cwd string) string {
	return filepath.Join(sRoot, "namespaces", hashString(cwd))
}

func namespaceForCurrentPane(sRoot, paneID string) (string, error) {
	path := filepath.Join(sRoot, "panes", paneID)
	cwdHash, err := readFile(path)
	if err != nil {
		return "", fmt.Errorf("pane '%s' is not registered", paneID)
	}
	return filepath.Join(sRoot, "namespaces", cwdHash), nil
}

func resolveRole(nsDir, role string) (string, error) {
	path := filepath.Join(nsDir, "roles", role)
	paneID, err := readFile(path)
	if err != nil {
		return "", fmt.Errorf("role '%s' is not registered in this namespace", role)
	}
	return paneID, nil
}

func cmdRegister(role string) {
	validateRole(role)
	session := zellijSessionName()
	paneID := requireCurrentPaneID()
	cwd := canonicalCwd()
	sRoot := sessionRoot(session)
	cwdHash := hashString(cwd)
	nsDir := namespaceDir(sRoot, cwd)

	if err := atomicWrite(filepath.Join(sRoot, ".session"), session); err != nil {
		die("failed to write session state: " + err.Error())
	}
	if err := atomicWrite(filepath.Join(nsDir, ".cwd"), cwd); err != nil {
		die("failed to write namespace state: " + err.Error())
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
		die("failed to write role: " + err.Error())
	}
	if err := atomicWrite(filepath.Join(sRoot, "panes", paneID), cwdHash); err != nil {
		die("failed to write pane mapping: " + err.Error())
	}

	_ = exec.Command("zellij", "action", "rename-pane", "--pane-id", paneID, fmt.Sprintf("panecom:%s", role)).Run()

	fmt.Println(role)
}

func cmdOpen(role string, direction string, binPath string) {
	validateRole(role)
	myPaneID := requireCurrentPaneID()

	cwd := canonicalCwd()

	newPaneArgs := []string{"action", "new-pane", "--near-current-pane", "--cwd", cwd}
	if direction != "" {
		newPaneArgs = append(newPaneArgs, "--direction", direction)
	}
	out, err := exec.Command("zellij", newPaneArgs...).Output()
	if err != nil {
		die("failed to create new pane: " + err.Error())
	}
	newPaneID := strings.TrimSpace(string(out))
	if newPaneID == "" {
		die("failed to get new pane ID")
	}

	numID := strings.TrimPrefix(newPaneID, "terminal_")

	registerCmd := fmt.Sprintf("ZELLIJ_PANE_ID=%s panecom register %s", numID, role)
	if err := sendToPane(newPaneID, registerCmd); err != nil {
		die(fmt.Sprintf("failed to register in new pane: %s", err.Error()))
	}

	time.Sleep(300 * time.Millisecond)
	_ = exec.Command("zellij", "action", "focus-pane-id", myPaneID).Run()

	fmt.Println(newPaneID)
}

func cmdWhoami() {
	session := zellijSessionName()
	paneID := requireCurrentPaneID()
	sRoot := sessionRoot(session)

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

func resolveRoleForCommand(role string) string {
	validateRole(role)
	session := zellijSessionName()
	paneID := currentPaneID()
	sRoot := sessionRoot(session)

	var nsDir string
	if paneID != "" {
		var err error
		nsDir, err = namespaceForCurrentPane(sRoot, paneID)
		if err != nil {
			cwd := canonicalCwd()
			nsDir = namespaceDir(sRoot, cwd)
		}
	} else {
		cwd := canonicalCwd()
		nsDir = namespaceDir(sRoot, cwd)
	}

	targetPaneID, err := resolveRole(nsDir, role)
	if err != nil {
		die(fmt.Sprintf("role '%s' is not registered in this namespace", role))
	}

	exists, err := paneExists(targetPaneID)
	if err != nil {
		die(fmt.Sprintf("failed to check pane '%s': %s", targetPaneID, err.Error()))
	}
	if !exists {
		_ = os.Remove(filepath.Join(nsDir, "roles", role))
		_ = os.Remove(filepath.Join(sRoot, "panes", targetPaneID))
		_ = os.Remove(filepath.Join(sRoot, "panes", targetPaneID+".bin"))
		die(fmt.Sprintf("role '%s' points to stale pane '%s'", role, targetPaneID))
	}

	return targetPaneID
}

func cmdResolve(role string) {
	paneID := resolveRoleForCommand(role)
	fmt.Println(paneID)
}

func cmdDump(role string, full bool, lines int) {
	paneID := resolveRoleForCommand(role)
	screen, err := dumpPane(paneID, full)
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

func cmdSend(role, message string) {
	paneID := resolveRoleForCommand(role)
	if err := sendToPane(paneID, message); err != nil {
		die(fmt.Sprintf("failed to send message to '%s': %s", role, err.Error()))
	}
}

func dumpPane(paneID string, full bool) (string, error) {
	args := []string{"action", "dump-screen", "--pane-id", paneID}
	if full {
		args = append(args, "--full")
	}
	out, err := exec.Command("zellij", args...).Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}

func sendToPane(paneID, message string) error {
	cmd := exec.Command("zellij", "action", "write-chars", "--pane-id", paneID, message)
	if err := cmd.Run(); err != nil {
		return err
	}
	time.Sleep(100 * time.Millisecond)
	cmd = exec.Command("zellij", "action", "write", "--pane-id", paneID, "13")
	return cmd.Run()
}

func cmdShare(toRole string, full bool, lines int) {
	fromPaneID := requireCurrentPaneID()
	toPaneID := resolveRoleForCommand(toRole)

	session := zellijSessionName()
	sRoot := sessionRoot(session)
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

	screen, err := dumpPane(fromPaneID, full)
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

	// Mkdir is atomic — only one process succeeds
	if err := os.Mkdir(lockDir, 0755); err == nil {
		// We own the lock — write token
		if wErr := os.WriteFile(filepath.Join(lockDir, "token"), []byte(token), 0644); wErr != nil {
			_ = os.RemoveAll(lockDir)
			die("failed to write lock token: " + wErr.Error())
		}
		return
	}

	// Lock exists — check for stale (runner completed)
	oldToken, readErr := readFile(filepath.Join(lockDir, "token"))
	if readErr != nil {
		die("another exec is already running")
	}
	oldExitcode := filepath.Join(stateRoot(), "exec", oldToken, "exitcode")
	if _, statErr := os.Stat(oldExitcode); statErr != nil {
		die("another exec is already running")
	}

	// Stale: atomically claim by renaming the lock dir
	recoverDir := lockDir + ".recover." + token
	if err := os.Rename(lockDir, recoverDir); err != nil {
		// Another process already claimed it
		die("another exec is already running")
	}
	_ = os.RemoveAll(recoverDir)

	// Create fresh lock
	if err := os.Mkdir(lockDir, 0755); err != nil {
		die("another exec is already running")
	}
	if wErr := os.WriteFile(filepath.Join(lockDir, "token"), []byte(token), 0644); wErr != nil {
		_ = os.RemoveAll(lockDir)
		die("failed to write lock token: " + wErr.Error())
	}
}

func execDieWithCleanup(token, execDir, msg string) {
	// Write exitcode so stale detection can recover the lock
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

	// Update "current" symlink for progress monitoring (cat .panecom/exec/current/stdout)
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

	// Clean up old completed exec dirs
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
	// Leading space suppresses history when HIST_IGNORE_SPACE is set in shell
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
			// Lock stays — runner will write exitcode, next acquirer does stale recovery
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

	// Lock is NOT released here. Next acquirer's stale detection handles cleanup.
	// This avoids TOCTOU between read-token and remove-lock.

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
		// No exec dir — write exitcode to enable stale recovery
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
	// Lock is NOT released here. Stale detection by next acquirer handles cleanup.
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

func findProjectConfig() string {
	cwd := canonicalCwd()
	dir := cwd
	for {
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

	_ = binPath // reserved for future use

	cwd := canonicalCwd()
	myPaneID := currentPaneID()

	seen := make(map[string]bool)
	for _, p := range profile.Panes {
		validateRole(p.Role)
		if seen[p.Role] {
			die(fmt.Sprintf("duplicate role '%s' in profile '%s'", p.Role, name))
		}
		seen[p.Role] = true
	}

	panesBefore, _ := listTerminalPanes()
	existingIDs := make(map[int]bool)
	for _, p := range panesBefore {
		existingIDs[p.ID] = true
	}

	tabOut, err := exec.Command("zellij", "action", "new-tab", "--name", name, "--cwd", cwd).Output()
	if err != nil {
		die("failed to create tab: " + err.Error())
	}
	tabIDStr := strings.TrimSpace(string(tabOut))
	tabID, err := strconv.Atoi(tabIDStr)
	if err != nil {
		die(fmt.Sprintf("failed to parse tab ID '%s': %s", tabIDStr, err.Error()))
	}
	time.Sleep(500 * time.Millisecond)

	firstPaneID := ""
	panesAfter, err := listTerminalPanes()
	if err != nil {
		die("failed to list panes after tab creation: " + err.Error())
	}
	for _, p := range panesAfter {
		if p.TabID == tabID && !existingIDs[p.ID] {
			firstPaneID = fmt.Sprintf("terminal_%d", p.ID)
			break
		}
	}
	if firstPaneID == "" {
		die("failed to detect initial pane in new tab")
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
			newOut, err := exec.Command("zellij", "action", "new-pane", "--direction", dir, "--cwd", cwd, "--tab-id", strconv.Itoa(tabID)).Output()
			if err != nil {
				die(fmt.Sprintf("failed to create pane for role '%s': %s", p.Role, err.Error()))
			}
			paneID = strings.TrimSpace(string(newOut))
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
		numID := strings.TrimPrefix(p.paneID, "terminal_")
		registerCmd := fmt.Sprintf("cd %q && ZELLIJ_PANE_ID=%s panecom register %s", cwd, numID, p.cfg.Role)
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
			_ = exec.Command("zellij", "action", "focus-pane-id", foregroundPaneID).Run()
		}
	} else {
		if myPaneID != "" {
			_ = exec.Command("zellij", "action", "focus-pane-id", myPaneID).Run()
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
  panecom register <role>                Register current pane as <role>
  panecom whoami                         Show current pane's role
  panecom resolve <role>                 Show pane ID for <role>
  panecom open [-d right|down] <role>    Open new pane and register as <role>
  panecom dump [--full] [-l N] <role>    Dump screen of <role>'s pane
  panecom send <role> <msg>              Send message to <role>'s pane
  panecom share [--full] [-l N] <role>    Share this pane's screen with <role>
  panecom exec [--timeout N] <role> <cmd> Run command on <role>, return output
  panecom profile [-f] <name>            Launch profile from .panecom/config.yaml
`)
	os.Exit(1)
}

func main() {
	args := os.Args[1:]
	if len(args) == 0 {
		usage()
	}

	switch args[0] {
	case "register":
		if len(args) != 2 {
			usage()
		}
		cmdRegister(args[1])
	case "whoami":
		if len(args) != 1 {
			usage()
		}
		cmdWhoami()
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
		if len(args) < 3 {
			usage()
		}
		cmdSend(args[1], strings.Join(args[2:], " "))
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
		if len(rest) >= 1 && rest[0] == "--" {
			cmdExecRun(os.Getenv("PANECOM_TOKEN"))
			return
		}
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
