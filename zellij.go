package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type ZellijMux struct {
	runner CommandRunner
}

func (z *ZellijMux) Kind() string { return "zellij" }

func (z *ZellijMux) SessionName() (string, error) {
	s := os.Getenv("ZELLIJ_SESSION_NAME")
	if s == "" {
		return "", fmt.Errorf("not running inside Zellij (ZELLIJ_SESSION_NAME not set)")
	}
	return s, nil
}

func (z *ZellijMux) CurrentPaneID() (string, error) {
	id := os.Getenv("ZELLIJ_PANE_ID")
	if id == "" {
		return "", fmt.Errorf("ZELLIJ_PANE_ID not set")
	}
	return "terminal_" + id, nil
}

type zellijRawPane struct {
	ID       int    `json:"id"`
	IsPlugin bool   `json:"is_plugin"`
	TabID    int    `json:"tab_id"`
	TabName  string `json:"tab_name"`
}

func (z *ZellijMux) ListPanes(_ string) ([]Pane, error) {
	out, err := z.runner.Output("zellij", "action", "list-panes", "--json", "-t")
	if err != nil {
		return nil, err
	}
	var raw []zellijRawPane
	if err := json.Unmarshal(out, &raw); err != nil {
		return nil, err
	}
	var panes []Pane
	for _, p := range raw {
		if !p.IsPlugin {
			panes = append(panes, Pane{
				ID:      fmt.Sprintf("terminal_%d", p.ID),
				TabID:   strconv.Itoa(p.TabID),
				TabName: p.TabName,
			})
		}
	}
	return panes, nil
}

func (z *ZellijMux) PaneExists(sessionName string, paneID string) (bool, error) {
	panes, err := z.ListPanes(sessionName)
	if err != nil {
		return false, fmt.Errorf("failed to list panes: %w", err)
	}
	for _, p := range panes {
		if p.ID == paneID {
			return true, nil
		}
	}
	return false, nil
}

func (z *ZellijMux) NewPane(targetPaneID string, cwd string, direction string) (string, error) {
	args := []string{"action", "new-pane", "--near-current-pane", "--cwd", cwd}
	if direction != "" {
		args = append(args, "--direction", direction)
	}
	out, err := z.runner.Output("zellij", args...)
	if err != nil {
		return "", fmt.Errorf("failed to create new pane: %w", err)
	}
	paneID := strings.TrimSpace(string(out))
	if paneID == "" {
		return "", fmt.Errorf("failed to get new pane ID")
	}
	return paneID, nil
}

func (z *ZellijMux) NewTab(name string, cwd string) (string, string, error) {
	panesBefore, err := z.ListPanes("")
	if err != nil {
		return "", "", fmt.Errorf("failed to list panes before tab creation: %w", err)
	}
	existingIDs := make(map[string]bool)
	for _, p := range panesBefore {
		existingIDs[p.ID] = true
	}

	out, err := z.runner.Output("zellij", "action", "new-tab", "--name", name, "--cwd", cwd)
	if err != nil {
		return "", "", fmt.Errorf("failed to create tab: %w", err)
	}
	tabID := strings.TrimSpace(string(out))
	if _, err := strconv.Atoi(tabID); err != nil {
		return "", "", fmt.Errorf("failed to parse tab ID '%s': %w", tabID, err)
	}

	time.Sleep(500 * time.Millisecond)

	panesAfter, err := z.ListPanes("")
	if err != nil {
		return "", "", fmt.Errorf("failed to list panes after tab creation: %w", err)
	}
	for _, p := range panesAfter {
		if p.TabID == tabID && !existingIDs[p.ID] {
			return tabID, p.ID, nil
		}
	}
	return "", "", fmt.Errorf("failed to detect initial pane in new tab")
}

func (z *ZellijMux) NewPaneInTab(tabID string, cwd string, direction string) (string, error) {
	dir := direction
	if dir == "" {
		dir = "right"
	}
	out, err := z.runner.Output("zellij", "action", "new-pane", "--direction", dir, "--cwd", cwd, "--tab-id", tabID)
	if err != nil {
		return "", fmt.Errorf("failed to create pane in tab: %w", err)
	}
	paneID := strings.TrimSpace(string(out))
	if paneID == "" {
		return "", fmt.Errorf("failed to get pane ID")
	}
	return paneID, nil
}

func (z *ZellijMux) DumpPane(paneID string, full bool) (string, error) {
	args := []string{"action", "dump-screen", "--pane-id", paneID}
	if full {
		args = append(args, "--full")
	}
	out, err := z.runner.Output("zellij", args...)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

func (z *ZellijMux) SendKeys(paneID string, text string) error {
	return z.runner.Run("zellij", "action", "write-chars", "--pane-id", paneID, text)
}

func (z *ZellijMux) SendEnter(paneID string) error {
	return z.runner.Run("zellij", "action", "write", "--pane-id", paneID, "13")
}

func (z *ZellijMux) RenamePane(paneID string, name string) error {
	return z.runner.Run("zellij", "action", "rename-pane", "--pane-id", paneID, name)
}

func (z *ZellijMux) FocusPane(paneID string) error {
	return z.runner.Run("zellij", "action", "focus-pane-id", paneID)
}
