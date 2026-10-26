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
	Title   string
	Cwd     string
	Command string
	X       int
	Y       int
	Width   int
	Height  int
}

// paneCache memoises the pane list for the lifetime of one command. Checking
// liveness role by role otherwise spawns a multiplexer query per role, which a
// namespace with dozens of registrations pays for on every list. A panecom
// process is short lived and only changes the pane set through NewPane/NewTab,
// so those paths invalidate the cache and read the multiplexer directly.
// The key keeps a cache filled for one session from answering for another.
type paneCache struct {
	key    string
	panes  []Pane
	loaded bool
}

func (c *paneCache) get(key string, load func() ([]Pane, error)) ([]Pane, error) {
	if c.loaded && c.key == key {
		return c.panes, nil
	}
	panes, err := load()
	if err != nil {
		return nil, err
	}
	c.key, c.panes, c.loaded = key, panes, true
	return panes, nil
}

func (c *paneCache) invalidate() { c.key, c.panes, c.loaded = "", nil, false }

type Multiplexer interface {
	Kind() string
	SessionName() (string, error)
	CurrentPaneID() (string, error)
	ListPanes(sessionName string) ([]Pane, error)
	// ListSessions names the sessions that are currently running. Sessions a
	// multiplexer keeps around in an exited state are not running: their panes
	// are gone and their IDs do not survive a resurrect.
	ListSessions() ([]string, error)
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
