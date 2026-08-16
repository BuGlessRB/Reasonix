package agent

import (
	"strings"
	"testing"
)

func TestSteerText(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
		wantOK  bool
	}{
		{
			name:    "happy path: prefix + newline + text",
			content: MidTurnSteerPrefix + "\nplease use smaller diffs",
			want:    "please use smaller diffs",
			wantOK:  true,
		},
		{
			name:    "prefix only, no user text",
			content: MidTurnSteerPrefix,
			want:    "",
			wantOK:  true,
		},
		{
			name:    "prefix with trailing whitespace only",
			content: MidTurnSteerPrefix + "\n  ",
			want:    "  ",
			wantOK:  true,
		},
		{
			name:    "round-trip through midTurnSteerMessage",
			content: midTurnSteerMessage("stop using such large diffs", false),
			want:    "stop using such large diffs",
			wantOK:  true,
		},
		{
			name:    "user text with leading/trailing spaces preserved (matches live event)",
			content: MidTurnSteerPrefix + "\n   keep going but use read_file first   ",
			want:    "   keep going but use read_file first   ",
			wantOK:  true,
		},
		{
			name:    "regular user message, not steer",
			content: "please use smaller diffs",
			want:    "",
			wantOK:  false,
		},
		{
			name:    "empty string",
			content: "",
			want:    "",
			wantOK:  false,
		},
		{
			name:    "whitespace only",
			content: "   ",
			want:    "",
			wantOK:  false,
		},
		{
			name:    "prefix-like but truncated (no closing bracket)",
			content: "[Mid-turn steer queued by the user. Do not treat this as a new task\nplease go on",
			want:    "",
			wantOK:  false,
		},
		{
			name:    "prefix appears mid-message, not at start",
			content: "hey model " + MidTurnSteerPrefix + "\nuse smaller diffs",
			want:    "",
			wantOK:  false,
		},
		{
			name:    "multiline steer text preserved",
			content: MidTurnSteerPrefix + "\nline one\nline two",
			want:    "line one\nline two",
			wantOK:  true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := SteerText(tt.content)
			if ok != tt.wantOK {
				t.Errorf("SteerText() ok = %v, want %v", ok, tt.wantOK)
			}
			if got != tt.want {
				t.Errorf("SteerText() text = %q, want %q", got, tt.want)
			}
			// Sanity: when ok is true the result must never contain the prefix.
			if ok && strings.Contains(got, MidTurnSteerPrefix) {
				t.Errorf("SteerText() returned text still contains the prefix: %q", got)
			}
		})
	}
}

func TestMidTurnSteerMessageRoundTrip(t *testing.T) {
	inputs := []string{
		"stop",
		"use read_file instead of cat",
		"",
		"  keep going  ",
	}
	for _, host := range []bool{false, true} {
		for _, in := range inputs {
			msg := midTurnSteerMessage(in, host)
			got, ok := SteerText(msg)
			if !ok {
				t.Errorf("SteerText(midTurnSteerMessage(%q, %v)): not recognized as steer", in, host)
				continue
			}
			if got != in {
				t.Errorf("SteerText(midTurnSteerMessage(%q, %v)) = %q, want %q", in, host, got, in)
			}
		}
	}
}

// Recovery guidance rides the steer path but must never present itself as the
// user speaking: a model told the user interrupted answers a person who did not.
func TestHostNoticeDoesNotClaimTheUserSpoke(t *testing.T) {
	msg := midTurnSteerMessage("a tool failed", true)
	if strings.Contains(msg, MidTurnSteerPrefix) {
		t.Fatalf("host notice = %q, want no user-steer prefix", msg)
	}
	if !strings.HasPrefix(msg, HostNoticePrefix) {
		t.Fatalf("host notice = %q, want the host prefix", msg)
	}
	if !strings.Contains(HostNoticePrefix, "the user did not send this") {
		t.Fatalf("host prefix = %q, want it to disclaim the user", HostNoticePrefix)
	}
}
