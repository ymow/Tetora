package main

import "testing"

func TestParseVersion(t *testing.T) {
	tests := []struct {
		input string
		want  []int
	}{
		{"2.0.3", []int{2, 0, 3}},
		{"2.0.3.1", []int{2, 0, 3, 1}},
		{"2.0.2.12", []int{2, 0, 2, 12}},
		{"dev", nil},
		{"", nil},
		{"v2.0.3", []int{2, 0, 3}},
		{"abc", nil},
	}
	for _, tt := range tests {
		got := parseVersion(tt.input)
		if tt.want == nil {
			if got != nil {
				t.Errorf("parseVersion(%q) = %v, want nil", tt.input, got)
			}
			continue
		}
		if len(got) != len(tt.want) {
			t.Errorf("parseVersion(%q) = %v, want %v", tt.input, got, tt.want)
			continue
		}
		for i := range got {
			if got[i] != tt.want[i] {
				t.Errorf("parseVersion(%q)[%d] = %d, want %d", tt.input, i, got[i], tt.want[i])
			}
		}
	}
}

func TestIsDevVersion(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"2.0.3", false},
		{"2.0.3.1", true},
		{"2.0.2.12", true},
		{"dev", false},
	}
	for _, tt := range tests {
		if got := isDevVersion(tt.input); got != tt.want {
			t.Errorf("isDevVersion(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestVersionNewerThan(t *testing.T) {
	tests := []struct {
		a, b string
		want bool
	}{
		// Release vs release
		{"2.0.3", "2.0.2", true},
		{"2.0.3", "2.0.3", false},
		{"2.0.2", "2.0.3", false},
		{"2.1.0", "2.0.9", true},
		{"3.0.0", "2.9.9", true},

		// Release vs dev
		{"2.0.3", "2.0.2.12", true},  // newer release > older dev
		{"2.0.3", "2.0.3.1", false},  // same base release vs dev: release is NOT "newer" (0 < 1 at segment 4)
		{"2.0.4", "2.0.3.1", true},   // newer release > dev

		// Dev vs dev
		{"2.0.3.2", "2.0.3.1", true},
		{"2.0.3.1", "2.0.3.2", false},
	}
	for _, tt := range tests {
		if got := versionNewerThan(tt.a, tt.b); got != tt.want {
			t.Errorf("versionNewerThan(%q, %q) = %v, want %v", tt.a, tt.b, got, tt.want)
		}
	}
}

func TestDevBaseVersion(t *testing.T) {
	tests := []struct {
		input, want string
	}{
		{"2.0.3.1", "2.0.3"},
		{"2.0.2.12", "2.0.2"},
		{"2.0.3", "2.0.3"},
	}
	for _, tt := range tests {
		if got := devBaseVersion(tt.input); got != tt.want {
			t.Errorf("devBaseVersion(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

// TestUpgradeScenarios verifies the upgrade decision logic for key scenarios.
func TestUpgradeScenarios(t *testing.T) {
	type scenario struct {
		name    string
		current string // tetoraVersion
		latest  string // GitHub release
		should  string // "upgrade" or "skip"
	}
	scenarios := []scenario{
		{"dev to newer release", "2.0.2.12", "2.0.3", "upgrade"},
		{"dev to same base release", "2.0.3.1", "2.0.3", "upgrade"},
		{"dev to older release", "2.0.4.1", "2.0.3", "skip"},
		{"release to same release", "2.0.3", "2.0.3", "skip"},
		{"release to newer release", "2.0.2", "2.0.3", "upgrade"},
		{"release to older release", "2.0.4", "2.0.3", "skip"},
	}

	for _, s := range scenarios {
		t.Run(s.name, func(t *testing.T) {
			shouldUpgrade := false
			if s.latest == s.current {
				shouldUpgrade = false
			} else if isDevVersion(s.current) {
				base := devBaseVersion(s.current)
				if base == s.latest || versionNewerThan(s.latest, base) {
					shouldUpgrade = true
				}
			} else if versionNewerThan(s.latest, s.current) {
				shouldUpgrade = true
			}

			expected := s.should == "upgrade"
			if shouldUpgrade != expected {
				t.Errorf("current=%s latest=%s: got upgrade=%v, want %v", s.current, s.latest, shouldUpgrade, expected)
			}
		})
	}
}
