package main

import (
	"strings"
	"testing"
)

func TestZellijPickerHighlightsAndRestores(t *testing.T) {
	r := newFakeRunner()
	h := newPaneHighlighter(&ZellijMux{runner: r})
	for _, id := range []string{"terminal_1", "terminal_2"} {
		if err := h.Select(Pane{ID: id}); err != nil {
			t.Fatal(err)
		}
	}
	h.Restore()
	want := []string{
		"action set-pane-color --pane-id terminal_1 --fg #ffffff --bg #354a7a",
		"action set-pane-color --pane-id terminal_1 --reset",
		"action set-pane-color --pane-id terminal_2 --fg #ffffff --bg #354a7a",
		"action set-pane-color --pane-id terminal_2 --reset",
	}
	if len(r.calls) != len(want) {
		t.Fatalf("calls: %+v", r.calls)
	}
	for i, call := range r.calls {
		if call.Name != "zellij" || strings.Join(call.Args, " ") != want[i] {
			t.Fatalf("call %d = %+v, want %s", i, call, want[i])
		}
	}
}

func TestTmuxPickerRestoresPriorMark(t *testing.T) {
	r := newFakeRunner()
	r.Stub([]byte("%1\t0\n%2\t1\n%3\t0\n"), "tmux", "list-panes", "-a", "-F", "#{pane_id}\t#{pane_marked}")
	h := newPaneHighlighter(&TmuxMux{runner: r})
	if err := h.Select(Pane{ID: "%1"}); err != nil {
		t.Fatal(err)
	}
	if err := h.Select(Pane{ID: "%3"}); err != nil {
		t.Fatal(err)
	}
	h.Restore()
	want := []string{
		"list-panes -a -F #{pane_id}\t#{pane_marked}",
		"select-pane -m -t %1",
		"select-pane -m -t %3",
		"select-pane -m -t %2",
	}
	if len(r.calls) != len(want) {
		t.Fatalf("calls: %+v", r.calls)
	}
	for i, call := range r.calls {
		if call.Name != "tmux" || strings.Join(call.Args, " ") != want[i] {
			t.Fatalf("call %d = %+v, want %s", i, call, want[i])
		}
	}
}

func TestTmuxPickerClearsTemporaryMark(t *testing.T) {
	r := newFakeRunner()
	r.Stub([]byte("%1\t0\n"), "tmux", "list-panes", "-a", "-F", "#{pane_id}\t#{pane_marked}")
	h := newPaneHighlighter(&TmuxMux{runner: r})
	if err := h.Select(Pane{ID: "%1"}); err != nil {
		t.Fatal(err)
	}
	h.Restore()
	last := r.calls[len(r.calls)-1]
	if last.Name != "tmux" || strings.Join(last.Args, " ") != "select-pane -M -t %1" {
		t.Fatalf("temporary mark not cleared: %+v", last)
	}
}
