package moderation

import "testing"

func TestRegexModerator_BlocksSlurs(t *testing.T) {
	mod := NewRegexModerator()
	tests := []struct {
		input   string
		blocked bool
	}{
		{"hello world", false},
		{"great post", false},
		{"you should kys now", true},
		{"KYS", true},
		{"kill yourself please", true},
		{"i will kill you", true},
		{"buy followers cheap", true},
		{"click here to earn money", true},
		// Word boundary — should not match substrings
		{"analyst report", false},
		{"skyscraper views", false},
	}
	for _, tt := range tests {
		ok, reason := mod.Check(tt.input)
		if tt.blocked && ok {
			t.Errorf("expected %q to be blocked, but it passed", tt.input)
		}
		if !tt.blocked && !ok {
			t.Errorf("expected %q to pass, but it was blocked: %s", tt.input, reason)
		}
	}
}

func TestRegexModerator_ExtraWords(t *testing.T) {
	mod := NewRegexModerator("badword")
	ok, _ := mod.Check("this is a badword test")
	if ok {
		t.Error("expected 'badword' to be blocked")
	}
	ok, _ = mod.Check("this is fine")
	if !ok {
		t.Error("expected clean content to pass")
	}
}
