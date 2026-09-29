package update

import "testing"

func TestParseVersion(t *testing.T) {
	tests := []struct {
		input               string
		major, minor, patch int
		wantErr             bool
	}{
		{"1.2.3", 1, 2, 3, false},
		{"v1.2.3", 1, 2, 3, false},
		{"0.2.0", 0, 2, 0, false},
		{"1.2", 0, 0, 0, true},
		{"abc", 0, 0, 0, true},
	}
	for _, tt := range tests {
		maj, min, pat, err := parseVersion(tt.input)
		if tt.wantErr {
			if err == nil {
				t.Errorf("parseVersion(%q) expected error", tt.input)
			}
			continue
		}
		if err != nil {
			t.Errorf("parseVersion(%q) unexpected error: %v", tt.input, err)
			continue
		}
		if maj != tt.major || min != tt.minor || pat != tt.patch {
			t.Errorf("parseVersion(%q) = %d.%d.%d, want %d.%d.%d", tt.input, maj, min, pat, tt.major, tt.minor, tt.patch)
		}
	}
}

func TestIsNewer(t *testing.T) {
	tests := []struct {
		latest, current string
		want            bool
	}{
		{"1.0.0", "0.9.0", true},
		{"0.3.0", "0.2.0", true},
		{"0.2.1", "0.2.0", true},
		{"0.2.0", "0.2.0", false},
		{"0.1.0", "0.2.0", false},
	}
	for _, tt := range tests {
		got, err := isNewer(tt.latest, tt.current)
		if err != nil {
			t.Errorf("isNewer(%q, %q) error: %v", tt.latest, tt.current, err)
			continue
		}
		if got != tt.want {
			t.Errorf("isNewer(%q, %q) = %v, want %v", tt.latest, tt.current, got, tt.want)
		}
	}
}
