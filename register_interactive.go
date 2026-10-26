package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"
	"unicode"

	"golang.org/x/term"
)

// registerSelectedPane registers an existing pane using that pane's working
// directory. The command runs in the caller's pane; nothing is typed into the
// selected pane, which may contain a shell, an editor, or an AI agent.
func registerSelectedPane(role string, pane Pane, session string) (string, error) {
	if pane.Cwd == "" {
		return "", fmt.Errorf("working directory is unavailable for the selected pane")
	}
	resolved, err := expandPathFrom(canonicalCwd(), pane.Cwd)
	if err != nil {
		return "", fmt.Errorf("selected pane working directory: %w", err)
	}
	anchor := namespaceAnchorFrom(resolved, canonicalHome())
	alive, err := mux.PaneExists(session, pane.ID)
	if err != nil {
		return "", fmt.Errorf("checking selected pane: %w", err)
	}
	if !alive {
		return "", fmt.Errorf("selected pane has closed")
	}
	if err := registerPane(role, pane.ID, anchor, session); err != nil {
		return "", err
	}
	return anchor, nil
}

func cmdRegisterInteractive() {
	session, _ := requireSession()
	panes, err := mux.ListPanes(session)
	if err != nil {
		die("failed to list panes: " + err.Error())
	}
	if len(panes) == 0 {
		die("no terminal panes found in this session")
	}
	tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		die("interactive register needs a terminal (or use: panecom register <role>)")
	}
	defer tty.Close()

	pane, ok, err := pickPaneTTY(tty, panes, currentPaneIDOrEmpty())
	if err != nil {
		die("pane selection failed: " + err.Error())
	}
	if !ok {
		return
	}
	reader := bufio.NewReader(tty)
	if pane.Cwd == "" {
		path, err := promptLine(tty, reader, "Selected pane's project path: ")
		if err != nil {
			die("cannot read project path: " + err.Error())
		}
		pane.Cwd = path
	}
	role, err := promptLine(tty, reader, "Role name: ")
	if err != nil {
		die("cannot read role name: " + err.Error())
	}
	if !rolePattern.MatchString(role) {
		die(fmt.Sprintf("invalid role name '%s' (must match [A-Za-z0-9][A-Za-z0-9._-]*)", role))
	}
	anchor, err := registerSelectedPane(role, pane, session)
	if err != nil {
		die(err.Error())
	}
	fmt.Printf("%s -> %s (%s)\n", role, pane.Title, anchor)
}

func promptLine(out io.Writer, reader *bufio.Reader, label string) (string, error) {
	if _, err := fmt.Fprint(out, label); err != nil {
		return "", err
	}
	line, err := reader.ReadString('\n')
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(line), nil
}

func pickPaneTTY(tty *os.File, panes []Pane, currentID string) (Pane, bool, error) {
	oldState, err := term.MakeRaw(int(tty.Fd()))
	if err != nil {
		return Pane{}, false, err
	}
	_, _ = fmt.Fprint(tty, "\x1b[?1049h\x1b[?25l")
	defer func() {
		_, _ = fmt.Fprint(tty, "\x1b[?25h\x1b[?1049l")
		_ = term.Restore(int(tty.Fd()), oldState)
	}()
	width, height, err := term.GetSize(int(tty.Fd()))
	if err != nil || width < 30 || height < 8 {
		width, height = 80, 24
	}
	return pickPane(panes, currentID, tty, tty, width, height, func(p Pane) (string, error) {
		return mux.DumpPane(p.ID, false)
	})
}

// pickPane contains the interaction logic independently of terminal raw mode.
// Up/down (or j/k) changes the highlighted pane, Enter confirms, and q exits.
func pickPane(panes []Pane, currentID string, in io.Reader, out io.Writer, width, height int, preview func(Pane) (string, error)) (Pane, bool, error) {
	selected := 0
	for i, p := range panes {
		if p.ID == currentID {
			selected = i
			break
		}
	}
	if len(panes) == 0 {
		return Pane{}, false, fmt.Errorf("no panes to select")
	}
	readByte := func() (byte, error) {
		var b [1]byte
		_, err := io.ReadFull(in, b[:])
		return b[0], err
	}
	for {
		var screen string
		if preview != nil {
			if s, err := preview(panes[selected]); err == nil {
				screen = s
			} else {
				screen = "(preview unavailable: " + err.Error() + ")"
			}
		}
		if _, err := io.WriteString(out, renderPanePicker(panes, selected, screen, width, height)); err != nil {
			return Pane{}, false, err
		}
		key, err := readByte()
		if err != nil {
			return Pane{}, false, err
		}
		switch key {
		case '\r', '\n':
			return panes[selected], true, nil
		case 'q', 3:
			return Pane{}, false, nil
		case 'j', '\t':
			selected = (selected + 1) % len(panes)
		case 'k':
			selected = (selected + len(panes) - 1) % len(panes)
		case 27:
			prefix, err := readByte()
			if err != nil {
				return Pane{}, false, err
			}
			if prefix != '[' && prefix != 'O' {
				continue
			}
			direction, err := readByte()
			if err != nil {
				return Pane{}, false, err
			}
			switch direction {
			case 'A', 'D':
				selected = (selected + len(panes) - 1) % len(panes)
			case 'B', 'C':
				selected = (selected + 1) % len(panes)
			}
		}
	}
}

func renderPanePicker(panes []Pane, selected int, preview string, width, height int) string {
	var b strings.Builder
	b.WriteString("\x1b[H\x1b[2J")
	b.WriteString("Select a pane  ↑/↓ or j/k: move  Enter: select  q: cancel\r\n\r\n")
	// Reserve room for the selected pane's preview on a small terminal.
	visible := height / 2
	if visible < 3 {
		visible = 3
	}
	start := selected - visible/2
	if start < 0 {
		start = 0
	}
	if start+visible > len(panes) {
		start = len(panes) - visible
		if start < 0 {
			start = 0
		}
	}
	for i := start; i < len(panes) && i < start+visible; i++ {
		p := panes[i]
		marker := "  "
		if i == selected {
			marker = "> "
		}
		name := p.Title
		if name == "" {
			name = p.Command
		}
		if name == "" {
			name = "pane"
		}
		line := fmt.Sprintf("%s[%s] %s  %s", marker, p.TabName, name, p.Cwd)
		b.WriteString(fitLine(line, width))
		b.WriteString("\r\n")
	}
	b.WriteString("\r\nPreview:\r\n")
	lines := strings.Split(strings.TrimRight(preview, "\n"), "\n")
	limit := height - visible - 6
	if limit < 1 {
		limit = 1
	}
	if len(lines) > limit {
		lines = lines[len(lines)-limit:]
	}
	for _, line := range lines {
		b.WriteString(fitLine(line, width))
		b.WriteString("\r\n")
	}
	return b.String()
}

func fitLine(s string, width int) string {
	var b strings.Builder
	for _, r := range s {
		if b.Len() >= width-1 {
			break
		}
		if r == '\t' {
			r = ' '
		}
		if !unicode.IsPrint(r) {
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}
