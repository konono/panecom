package main

import "strings"

// paneHighlighter changes only the multiplexer decoration. It never focuses a
// pane, so keyboard input stays in the interactive picker.
type paneHighlighter interface {
	Select(Pane) error
	Restore()
}

type noopHighlighter struct{}

func (noopHighlighter) Select(Pane) error { return nil }
func (noopHighlighter) Restore()          {}

func newPaneHighlighter(m Multiplexer) paneHighlighter {
	switch v := m.(type) {
	case *ZellijMux:
		return &zellijPaneHighlighter{runner: v.runner}
	case *TmuxMux:
		// tmux has one marked pane globally; preserve it across the picker.
		out, err := v.runner.Output("tmux", "list-panes", "-a", "-F", "#{pane_id}\t#{pane_marked}")
		if err != nil {
			return noopHighlighter{}
		}
		var original string
		for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
			parts := strings.SplitN(line, "\t", 2)
			if len(parts) == 2 && parts[1] == "1" {
				original = parts[0]
				break
			}
		}
		return &tmuxPaneHighlighter{runner: v.runner, original: original}
	default:
		return noopHighlighter{}
	}
}

type zellijPaneHighlighter struct {
	runner  CommandRunner
	current string
}

func (h *zellijPaneHighlighter) Select(p Pane) error {
	if h.current == p.ID {
		return nil
	}
	if h.current != "" {
		_ = h.runner.Run("zellij", "action", "set-pane-color", "--pane-id", h.current, "--reset")
	}
	h.current = ""
	if err := h.runner.Run("zellij", "action", "set-pane-color", "--pane-id", p.ID, "--fg", "#ffffff", "--bg", "#354a7a"); err != nil {
		return err
	}
	h.current = p.ID
	return nil
}

func (h *zellijPaneHighlighter) Restore() {
	if h.current != "" {
		_ = h.runner.Run("zellij", "action", "set-pane-color", "--pane-id", h.current, "--reset")
		h.current = ""
	}
}

type tmuxPaneHighlighter struct {
	runner   CommandRunner
	original string
	current  string
}

func (h *tmuxPaneHighlighter) Select(p Pane) error {
	if h.current == p.ID {
		return nil
	}
	if err := h.runner.Run("tmux", "select-pane", "-m", "-t", p.ID); err != nil {
		return err
	}
	h.current = p.ID
	return nil
}

func (h *tmuxPaneHighlighter) Restore() {
	if h.current == "" {
		return
	}
	if h.original != "" {
		_ = h.runner.Run("tmux", "select-pane", "-m", "-t", h.original)
	} else {
		_ = h.runner.Run("tmux", "select-pane", "-M", "-t", h.current)
	}
	h.current = ""
}
