package main

import (
	"os"
	"testing"
)

func TestDetectMultiplexerFlag(t *testing.T) {
	t.Setenv("ZELLIJ_SESSION_NAME", "z-session")
	t.Setenv("TMUX", "/tmp/tmux-501/default,12345,0")

	m, err := DetectMultiplexer("tmux", defaultRunner)
	if err != nil {
		t.Fatal(err)
	}
	if m.Kind() != "tmux" {
		t.Errorf("expected tmux, got %s", m.Kind())
	}

	m, err = DetectMultiplexer("zellij", defaultRunner)
	if err != nil {
		t.Fatal(err)
	}
	if m.Kind() != "zellij" {
		t.Errorf("expected zellij, got %s", m.Kind())
	}
}

func TestDetectMultiplexerEnv(t *testing.T) {
	t.Setenv("ZELLIJ_SESSION_NAME", "")
	t.Setenv("TMUX", "/tmp/tmux-501/default,12345,0")
	t.Setenv("PANECOM_MUX", "tmux")

	m, err := DetectMultiplexer("", defaultRunner)
	if err != nil {
		t.Fatal(err)
	}
	if m.Kind() != "tmux" {
		t.Errorf("expected tmux, got %s", m.Kind())
	}
}

func TestDetectMultiplexerPANECOM_MUXOverridesAutoDetect(t *testing.T) {
	t.Setenv("ZELLIJ_SESSION_NAME", "z-session")
	t.Setenv("TMUX", "/tmp/tmux-501/default,12345,0")
	t.Setenv("PANECOM_MUX", "tmux")

	m, err := DetectMultiplexer("", defaultRunner)
	if err != nil {
		t.Fatal(err)
	}
	if m.Kind() != "tmux" {
		t.Errorf("expected tmux via PANECOM_MUX, got %s", m.Kind())
	}
}

func TestDetectMultiplexerAutoZellij(t *testing.T) {
	t.Setenv("ZELLIJ_SESSION_NAME", "z-session")
	t.Setenv("TMUX", "")
	t.Setenv("PANECOM_MUX", "")

	m, err := DetectMultiplexer("", defaultRunner)
	if err != nil {
		t.Fatal(err)
	}
	if m.Kind() != "zellij" {
		t.Errorf("expected zellij, got %s", m.Kind())
	}
}

func TestDetectMultiplexerAutoTmux(t *testing.T) {
	t.Setenv("ZELLIJ_SESSION_NAME", "")
	t.Setenv("TMUX", "/tmp/tmux-501/default,12345,0")
	t.Setenv("PANECOM_MUX", "")

	m, err := DetectMultiplexer("", defaultRunner)
	if err != nil {
		t.Fatal(err)
	}
	if m.Kind() != "tmux" {
		t.Errorf("expected tmux, got %s", m.Kind())
	}
}

func TestDetectMultiplexerZellijPriorityOverTmux(t *testing.T) {
	t.Setenv("ZELLIJ_SESSION_NAME", "z-session")
	t.Setenv("TMUX", "/tmp/tmux-501/default,12345,0")
	t.Setenv("PANECOM_MUX", "")

	m, err := DetectMultiplexer("", defaultRunner)
	if err != nil {
		t.Fatal(err)
	}
	if m.Kind() != "zellij" {
		t.Errorf("expected zellij (priority over tmux in auto-detect), got %s", m.Kind())
	}
}

func TestDetectMultiplexerNone(t *testing.T) {
	t.Setenv("ZELLIJ_SESSION_NAME", "")
	t.Setenv("TMUX", "")
	t.Setenv("PANECOM_MUX", "")

	_, err := DetectMultiplexer("", defaultRunner)
	if err == nil {
		t.Error("expected error when no multiplexer detected")
	}
}

func TestDetectMultiplexerInvalidName(t *testing.T) {
	_, err := DetectMultiplexer("screen", defaultRunner)
	if err == nil {
		t.Error("expected error for unsupported multiplexer")
	}
}

func TestDetectMultiplexerFlagOverridesAll(t *testing.T) {
	t.Setenv("ZELLIJ_SESSION_NAME", "z-session")
	t.Setenv("TMUX", "/tmp/tmux-501/default,12345,0")
	t.Setenv("PANECOM_MUX", "zellij")

	m, err := DetectMultiplexer("tmux", defaultRunner)
	if err != nil {
		t.Fatal(err)
	}
	if m.Kind() != "tmux" {
		t.Errorf("--mux flag should override PANECOM_MUX, got %s", m.Kind())
	}
}

func TestSessionRootZellijCompat(t *testing.T) {
	os.Setenv("PANECOM_STATE_DIR", "/tmp/test-state")
	defer os.Unsetenv("PANECOM_STATE_DIR")

	zellijRoot := sessionRoot("zellij", "my-session")
	expectedHash := hashString("my-session")
	if zellijRoot != "/tmp/test-state/sessions/"+expectedHash {
		t.Errorf("zellij session root should use raw session name hash, got %s", zellijRoot)
	}
}

func TestSessionRootTmuxSeparation(t *testing.T) {
	os.Setenv("PANECOM_STATE_DIR", "/tmp/test-state")
	defer os.Unsetenv("PANECOM_STATE_DIR")

	zellijRoot := sessionRoot("zellij", "my-session")
	tmuxRoot := sessionRoot("tmux", "my-session")

	if zellijRoot == tmuxRoot {
		t.Error("same session name with different mux kinds should produce different state roots")
	}

	tmuxExpectedHash := hashString("tmux:my-session")
	if tmuxRoot != "/tmp/test-state/sessions/"+tmuxExpectedHash {
		t.Errorf("tmux session root should use 'tmux:<session>' hash, got %s", tmuxRoot)
	}
}
