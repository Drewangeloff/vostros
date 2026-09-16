package repository

import (
	"reflect"
	"testing"
)

func TestMentionUsernames(t *testing.T) {
	cases := []struct {
		text string
		want []string
	}{
		{"@alice can @bob help? @alice", []string{"alice", "bob"}},
		{"Contact person@alice.com or https://example.com/@bob", []string{}},
		{"(@alice), @Case_Sensitive!", []string{"alice", "Case_Sensitive"}},
		{"@ab @abcdefghijklmnopqrstu @abcdefghijklmnopqrst", []string{"abcdefghijklmnopqrst"}},
		{"Email us, then\n@alice: what did you find?", []string{"alice"}},
	}
	for _, tc := range cases {
		if got := MentionUsernames(tc.text); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%q: got %v want %v", tc.text, got, tc.want)
		}
	}
}
