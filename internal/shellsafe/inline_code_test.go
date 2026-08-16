package shellsafe

import "testing"

// The two copies of this rule had drifted: permission knew `bash -c` and the
// smaller interpreters, evidence did not, so the same command was inline code
// to the approval layer and an ordinary program to the ledger.
func TestArgvCarriesInlineCodeCoversEveryInterpreter(t *testing.T) {
	carries := [][]string{
		{"python3", "-c", "print(1)"},
		{"python", "-c", "print(1)"},
		{"node", "-e", "console.log(1)"},
		{"node", "--eval=1+1"},
		{"bun", "-p", "1"},
		{"deno", "eval", "console.log(1)"},
		{"perl", "-e", "print 1"},
		{"ruby", "-e", "puts 1"},
		{"lua", "-e", "print(1)"},
		{"rscript", "-e", "1"},
		{"osascript", "-e", "beep"},
		{"php", "-r", "echo 1;"},
		{"bash", "-c", "ls"},
		{"sh", "-lc", "ls"},
		{"zsh", "--command", "ls"},
		{"/usr/bin/python3", "-c", "print(1)"},
		{"pwsh", "-Command", "ls"},
		{"cmd", "/c", "dir"},
	}
	for _, argv := range carries {
		if !ArgvCarriesInlineCode(argv) {
			t.Errorf("%v: inline code went unrecognised", argv)
		}
	}
	plain := [][]string{
		{"python3", "script.py"},
		{"node", "index.js"},
		{"bash", "install.sh"},
		{"bash", "--", "-c"}, // after --, no more options
		{"go", "test", "./..."},
		{"grep", "-e", "pattern", "file.txt"}, // -e is not code here
		{},
	}
	for _, argv := range plain {
		if ArgvCarriesInlineCode(argv) {
			t.Errorf("%v: an ordinary call read as inline code", argv)
		}
	}
}

func TestExecutableBaseIgnoresPathAndExtension(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"python3", "python3"},
		{"/usr/bin/Python3", "python3"},
		{`C:\Windows\System32\cmd.exe`, "cmd"},
		{"", ""},
	} {
		if got := ExecutableBase(tc.in); got != tc.want {
			t.Errorf("ExecutableBase(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
