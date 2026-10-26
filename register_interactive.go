package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"
	"unicode"

	"golang.org/x/sys/unix"
	"golang.org/x/term"
)

// Esc is also the prefix of arrow keys, so wait briefly before treating it
// as cancellation. Select works on macOS PTYs, where Poll can report NVAL
// despite readable bytes.
const escapeSequenceTimeoutMS = 500

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
	currentID := requirePaneID()
	panes, err = panesInCurrentTab(panes, currentID)
	if err != nil {
		die(err.Error())
	}
	tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		die("interactive register needs a terminal (or use: panecom register <role>)")
	}
	defer func() { _ = tty.Close() }()

	pane, ok, err := pickPaneTTY(tty, panes, currentID)
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
	if anchor != namespaceAnchor() {
		fmt.Fprintf(os.Stderr, "panecom: registered in another project; to use it here, run: panecom link <project> %q, then address <project>/%s\n", anchor, role)
	}
}

func panesInCurrentTab(panes []Pane, currentID string) ([]Pane, error) {
	for _, current := range panes {
		if current.ID == currentID {
			var local []Pane
			for _, pane := range panes {
				if pane.TabID == current.TabID {
					local = append(local, pane)
				}
			}
			return local, nil
		}
	}
	return nil, fmt.Errorf("current pane %s is not in the session's pane list", currentID)
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
	highlight := newPaneHighlighter(mux)
	defer highlight.Restore()
	return pickPane(panes, currentID, tty, tty, width, height, func(p Pane) (string, error) {
		return mux.DumpPane(p.ID, false)
	}, highlight.Select)
}

// pickPane contains the interaction logic independently of terminal raw mode.
// Arrows move across the pane layout, Enter confirms, and Esc/q exits.
func pickPane(panes []Pane, currentID string, in io.Reader, out io.Writer, width, height int, preview func(Pane) (string, error), highlight func(Pane) error) (Pane, bool, error) {
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
	lastHighlighted := ""
	for {
		if highlight != nil && panes[selected].ID != lastHighlighted {
			_ = highlight(panes[selected]) // Picker remains usable if visual marking fails.
			lastHighlighted = panes[selected].ID
		}
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
		case 'j':
			selected = spatialNeighbor(panes, selected, 'D')
		case 'k':
			selected = spatialNeighbor(panes, selected, 'U')
		case 'h':
			selected = spatialNeighbor(panes, selected, 'L')
		case 'l':
			selected = spatialNeighbor(panes, selected, 'R')
		case 27:
			// A lone Esc cancels; arrow keys arrive as Esc [ A/B/C/D.
			ready, err := inputReady(in, escapeSequenceTimeoutMS)
			if err != nil {
				return Pane{}, false, err
			}
			if !ready {
				return Pane{}, false, nil
			}
			prefix, err := readByte()
			if err != nil {
				if err == io.EOF {
					return Pane{}, false, nil
				}
				return Pane{}, false, err
			}
			if prefix != '[' && prefix != 'O' {
				continue
			}
			// CSI may contain modifiers such as Esc [ 1 ; 2 B.
			for i := 0; i < 16; i++ {
				ready, err := inputReady(in, escapeSequenceTimeoutMS)
				if err != nil {
					return Pane{}, false, err
				}
				if !ready {
					break
				}
				code, err := readByte()
				if err != nil {
					if err == io.EOF {
						return Pane{}, false, nil
					}
					return Pane{}, false, err
				}
				switch code {
				case 'A':
					selected = spatialNeighbor(panes, selected, 'U')
				case 'B':
					selected = spatialNeighbor(panes, selected, 'D')
				case 'C':
					selected = spatialNeighbor(panes, selected, 'R')
				case 'D':
					selected = spatialNeighbor(panes, selected, 'L')
				default:
					if code >= '0' && code <= '9' || code == ';' {
						continue
					}
				}
				break
			}
		}
	}
}

// inputReady waits only for a terminal file. Other readers (used by tests)
// report EOF normally, so the picker can also be tested without a real TTY.
func inputReady(in io.Reader, timeoutMS int) (bool, error) {
	f, ok := in.(*os.File)
	if !ok {
		return true, nil
	}
	fd := int(f.Fd())
	deadline := time.Now().Add(time.Duration(timeoutMS) * time.Millisecond)
	for {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return false, nil
		}
		var readSet unix.FdSet
		readSet.Set(fd)
		timeout := unix.NsecToTimeval(remaining.Nanoseconds())
		n, err := unix.Select(fd+1, &readSet, nil, nil, &timeout)
		if err == unix.EINTR {
			continue
		}
		if err != nil {
			return false, err
		}
		return n > 0 && readSet.IsSet(fd), nil
	}
}

func renderPanePicker(panes []Pane, selected int, preview string, width, height int) string {
	var b strings.Builder
	b.WriteString("\x1b[H\x1b[2J")
	b.WriteString("Select a pane  arrows/hjkl: move  Enter: select  Esc/q: cancel\r\n")
	b.WriteString("Tab/window: " + panes[selected].TabName + "  (" + panes[selected].TabID + ")\r\n\r\n")
	var shown []int
	for i, p := range panes {
		if p.TabID == panes[selected].TabID {
			shown = append(shown, i)
		}
	}
	sort.SliceStable(shown, func(i, j int) bool {
		a, b := panes[shown[i]], panes[shown[j]]
		if a.Y != b.Y {
			return a.Y < b.Y
		}
		return a.X < b.X
	})
	selectedInTab := 0
	for i, index := range shown {
		if index == selected {
			selectedInTab = i
			break
		}
	}
	// Reserve room for the selected pane's preview on a small terminal.
	visible := height / 2
	if visible < 3 {
		visible = 3
	}
	start := selectedInTab - visible/2
	if start < 0 {
		start = 0
	}
	if start+visible > len(shown) {
		start = len(shown) - visible
		if start < 0 {
			start = 0
		}
	}
	for i := start; i < len(shown) && i < start+visible; i++ {
		p := panes[shown[i]]
		marker := "  "
		if shown[i] == selected {
			marker = "> "
		}
		name := p.Title
		if name == "" {
			name = p.Command
		}
		if name == "" {
			name = "pane"
		}
		line := fmt.Sprintf("%s[%s] %s  %s  %s", marker, p.TabName, p.ID, name, p.Cwd)
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
