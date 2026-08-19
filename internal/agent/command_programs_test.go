package agent

import (
	"strings"
	"testing"
)

// A shell command is a thing, not a clause. The advisory that named the
// command used %q, and the command that produced it in the field was a forty
// line here-document pasted into the middle of a sentence. Every shape below
// is read from the parsed command; an unreadable one is left unnamed rather
// than guessed at from its spelling.
func TestCommandProgramsNamesWhatRuns(t *testing.T) {
	for _, tc := range []struct{ command, want string }{
		{"go test ./...", "go"},
		{"make lint", "make"},
		{"/usr/local/bin/pytest -q", "pytest"},
		{"cd /tmp && npm run build", "cd · npm"},
		{"pytest -q | tail -5", "pytest · tail"},
		{"python3 - <<'EOF'\nprint(1)\nEOF", "python3"},
		{"cd /Users/yhh/projects && python3 - <<'EOF'\nimport re\nEOF", "python3"},
		{"", ""},
	} {
		if got := commandPrograms(tc.command); got != tc.want {
			t.Errorf("commandPrograms(%q) = %q, want %q", tc.command, got, tc.want)
		}
	}
}

// The whole point is that the command never reaches the message. A heredoc
// body carries the user's own file contents, and it was being rendered into a
// card in a chat window.
func TestCommandProgramsNeverCarriesTheCommand(t *testing.T) {
	command := "cd /Users/yhh/projects && python3 - <<'EOF'\nimport re\nassert 'secret' in open('x').read()\nEOF"
	got := commandPrograms(command)
	if len(got) > 40 {
		t.Fatalf("named the command in %d characters: %q", len(got), got)
	}
	for _, leaked := range []string{"secret", "import re", "EOF", "/Users/yhh"} {
		if strings.Contains(got, leaked) {
			t.Errorf("name %q carries %q from the command body", got, leaked)
		}
	}
}
