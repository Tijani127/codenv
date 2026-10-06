package manifest

import "testing"

func TestVersionMatches(t *testing.T) {
	cases := []struct {
		have, want string
		ok         bool
	}{
		{"3.11.9", "3.11", true},
		{"3.11.9", "3.11.9", true},
		{"3.12.1", "3.11", false},
		{"26.03", "26", true},
		{"26.03", "26.0", false},
		{"", "3.11", false},
		{"?", "3.11", false},
	}
	for _, tc := range cases {
		if got := versionMatches(tc.have, tc.want); got != tc.ok {
			t.Errorf("versionMatches(%q,%q) = %v, want %v", tc.have, tc.want, got, tc.ok)
		}
	}
}
