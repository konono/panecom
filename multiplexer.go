package main

import (
	"fmt"
	"os"
	"os/exec"
)

type Pane struct {
	ID      string
	TabID   string
	TabName string
}

type Multiplexer interface {
	Kind() string
	SessionName() (string, error)
	CurrentPaneID() (string, error)
	ListPanes(sessionName string) ([]Pane, error)
	PaneExists(sessionName string, paneID string) (bool, error)
	NewPane(targetPaneID string, cwd string, direction string) (paneID string, err error)
	NewTab(name string, cwd string) (tabID string, initialPaneID string, err error)
	NewPaneInTab(tabID string, cwd string, direction string) (paneID string, err error)
	DumpPane(paneID string, full bool) (string, error)
	SendKeys(paneID string, text string) error
	SendEnter(paneID string) error
	RenamePane(paneID string, name string) error
	FocusPane(paneID string) error
}

type CommandRunner interface {
	Output(name string, args ...string) ([]byte, error)
	Run(name string, args ...string) error
}

type execRunner struct{}

func (e *execRunner) Output(name string, args ...string) ([]byte, error) {
	return exec.Command(name, args...).Output()
}

func (e *execRunner) Run(name string, args ...string) error {
	return exec.Command(name, args...).Run()
}

var defaultRunner CommandRunner = &execRunner{}

func DetectMultiplexer(muxFlag string, runner CommandRunner) (Multiplexer, error) {
	if runner == nil {
		runner = defaultRunner
	}

	kind := muxFlag
	if kind == "" {
		kind = os.Getenv("PANECOM_MUX")
	}

	if kind != "" {
		switch kind {
		case "zellij":
			return &ZellijMux{runner: runner}, nil
		case "tmux":
			return &TmuxMux{runner: runner}, nil
		default:
			return nil, fmt.Errorf("unsupported multiplexer: %s (supported: zellij, tmux)", kind)
		}
	}

	if os.Getenv("ZELLIJ_SESSION_NAME") != "" {
		return &ZellijMux{runner: runner}, nil
	}
	if os.Getenv("TMUX") != "" {
		return &TmuxMux{runner: runner}, nil
	}

	return nil, fmt.Errorf("not running inside a supported multiplexer (set --mux or PANECOM_MUX)")
}
