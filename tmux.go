package main

import (
	"fmt"
	"os"
	"strings"
	"time"
)

type TmuxMux struct {
	runner CommandRunner
}

func (t *TmuxMux) Kind() string { return "tmux" }

func (t *TmuxMux) SessionName() (string, error) {
	if os.Getenv("TMUX") == "" {
		return "", fmt.Errorf("not running inside tmux (TMUX not set)")
	}
	out, err := t.runner.Output("tmux", "display-message", "-p", "#{session_name}")
	if err != nil {
		return "", fmt.Errorf("failed to get tmux session name: %w", err)
	}
	name := strings.TrimSpace(string(out))
	if name == "" {
		return "", fmt.Errorf("tmux session name is empty")
	}
	return name, nil
}

func (t *TmuxMux) CurrentPaneID() (string, error) {
	id := os.Getenv("TMUX_PANE")
	if id == "" {
		return "", fmt.Errorf("TMUX_PANE not set")
	}
	return id, nil
}

func (t *TmuxMux) ListPanes(sessionName string) ([]Pane, error) {
	target := sessionName
	if target == "" {
		var err error
		target, err = t.SessionName()
		if err != nil {
			return nil, err
		}
	}
	out, err := t.runner.Output("tmux", "list-panes", "-s", "-t", target, "-F", "#{pane_id}\t#{window_id}\t#{window_name}")
	if err != nil {
		return nil, fmt.Errorf("failed to list tmux panes: %w", err)
	}
	var panes []Pane
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "\t", 3)
		if len(parts) < 3 {
			continue
		}
		panes = append(panes, Pane{
			ID:      parts[0],
			TabID:   parts[1],
			TabName: parts[2],
		})
	}
	return panes, nil
}

func (t *TmuxMux) PaneExists(sessionName string, paneID string) (bool, error) {
	panes, err := t.ListPanes(sessionName)
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

func (t *TmuxMux) NewPane(targetPaneID string, cwd string, direction string) (string, error) {
	args := []string{"split-window", "-t", targetPaneID, "-c", cwd, "-P", "-F", "#{pane_id}"}
	if direction == "down" {
		args = append(args, "-v")
	} else {
		args = append(args, "-h")
	}
	out, err := t.runner.Output("tmux", args...)
	if err != nil {
		return "", fmt.Errorf("failed to create new pane: %w", err)
	}
	paneID := strings.TrimSpace(string(out))
	if paneID == "" {
		return "", fmt.Errorf("failed to get new pane ID")
	}
	return paneID, nil
}

func (t *TmuxMux) NewTab(name string, cwd string) (string, string, error) {
	out, err := t.runner.Output("tmux", "new-window", "-n", name, "-c", cwd, "-P", "-F", "#{window_id}\t#{pane_id}")
	if err != nil {
		return "", "", fmt.Errorf("failed to create new window: %w", err)
	}
	parts := strings.SplitN(strings.TrimSpace(string(out)), "\t", 2)
	if len(parts) < 2 {
		return "", "", fmt.Errorf("unexpected new-window output: %s", string(out))
	}
	return parts[0], parts[1], nil
}

func (t *TmuxMux) NewPaneInTab(tabID string, cwd string, direction string) (string, error) {
	args := []string{"split-window", "-t", tabID, "-c", cwd, "-P", "-F", "#{pane_id}"}
	if direction == "down" {
		args = append(args, "-v")
	} else {
		args = append(args, "-h")
	}
	out, err := t.runner.Output("tmux", args...)
	if err != nil {
		return "", fmt.Errorf("failed to create pane in window: %w", err)
	}
	paneID := strings.TrimSpace(string(out))
	if paneID == "" {
		return "", fmt.Errorf("failed to get pane ID")
	}
	return paneID, nil
}

func (t *TmuxMux) DumpPane(paneID string, full bool) (string, error) {
	args := []string{"capture-pane", "-t", paneID, "-p"}
	if full {
		args = append(args, "-S", "-")
	}
	out, err := t.runner.Output("tmux", args...)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

func (t *TmuxMux) SendKeys(paneID string, text string) error {
	return t.runner.Run("tmux", "send-keys", "-t", paneID, "-l", "--", text)
}

func (t *TmuxMux) SendEnter(paneID string) error {
	time.Sleep(100 * time.Millisecond)
	return t.runner.Run("tmux", "send-keys", "-t", paneID, "Enter")
}

func (t *TmuxMux) RenamePane(paneID string, name string) error {
	return t.runner.Run("tmux", "select-pane", "-t", paneID, "-T", name)
}

func (t *TmuxMux) FocusPane(paneID string) error {
	return t.runner.Run("tmux", "switch-client", "-t", paneID)
}
