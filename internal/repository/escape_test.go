package repository

import "testing"

func TestEscapeLike(t *testing.T) {
	tests := []struct {
		input, want string
	}{
		{"alice", "alice"},
		{"al%ice", `al\%ice`},
		{"a_b", `a\_b`},
		{`a\b`, `a\\b`},
		{"%_%", `\%\_\%`},
		{"", ""},
	}
	for _, tt := range tests {
		got := escapeLike(tt.input)
		if got != tt.want {
			t.Errorf("escapeLike(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
