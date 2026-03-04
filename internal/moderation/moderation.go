package moderation

import (
	"regexp"
	"strings"
)

type Moderator interface {
	Check(content string) (ok bool, reason string)
}

// RegexModerator is the Phase 0-1 content moderator.
// It checks against a blocklist of patterns. Fast and free.
type RegexModerator struct {
	patterns []*regexp.Regexp
}

func NewRegexModerator(extraWords ...string) *RegexModerator {
	// Default blocklist — covers slurs, threats, and spam patterns.
	// Extend via extraWords parameter or load from DB in future.
	defaults := []string{
		"nigger", "nigga", "faggot", "kike", "spic", "wetback", "chink",
		"kill yourself", "kys",
		"i will kill", "i will murder",
		"buy followers", "click here to earn",
	}
	words := append(defaults, extraWords...)
	var patterns []*regexp.Regexp
	for _, w := range words {
		p, err := regexp.Compile(`(?i)\b` + regexp.QuoteMeta(w) + `\b`)
		if err == nil {
			patterns = append(patterns, p)
		}
	}
	return &RegexModerator{patterns: patterns}
}

func (m *RegexModerator) Check(content string) (bool, string) {
	lower := strings.ToLower(content)
	for _, p := range m.patterns {
		if p.MatchString(lower) {
			return false, "content violates community guidelines"
		}
	}
	return true, ""
}
