package sandbox

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func fakePath(names ...string) func(string) (string, error) {
	set := map[string]bool{}
	for _, n := range names {
		set[n] = true
	}
	return func(name string) (string, error) {
		if set[name] {
			return `C:\fake\` + name + ".exe", nil
		}
		return "", exec.ErrNotFound
	}
}

func paths(shells []Shell) []string {
	out := make([]string, 0, len(shells))
	for _, sh := range shells {
		out = append(out, sh.Path)
	}
	return out
}

func TestAvailableListsInstalledInterpreters(t *testing.T) {
	yes := func(string) bool { return true }
	no := func(string) bool { return false }
	gitBash := []string{`C:\fake\Git\bin\bash.exe`}
	winPS := []string{`C:\fake\PowerShell\7\pwsh.exe`, `C:\fake\System32\powershell.exe`}

	cases := []struct {
		name string
		host shellHost
		want []string
	}{
		{
			"windows with git bash lists bash first",
			shellHost{"windows", fakePath("pwsh", "powershell"), yes, gitBash, winPS, yes, no, yes},
			[]string{`C:\fake\Git\bin\bash.exe`, `C:\fake\PowerShell\7\pwsh.exe`, `C:\fake\System32\powershell.exe`},
		},
		{
			// The same bash reached twice — once on PATH, once as the Git
			// candidate — is one interpreter, and two rows offering it would ask
			// the user to choose between a thing and itself.
			"a bash found twice is listed once",
			shellHost{"windows", fakePath("bash"), func(p string) bool { return p == `C:\fake\bash.exe` }, []string{`C:\fake\bash.exe`}, nil, yes, no, yes},
			[]string{`C:\fake\bash.exe`},
		},
		{
			// The WSL launcher runs commands inside the Linux VM, where the
			// workspace is a /mnt path; offering it would hand the agent a shell
			// that cannot see the files it was pointed at.
			"the wsl launcher is not on offer",
			shellHost{"windows", fakePath("bash", "powershell"), no, nil, winPS, yes, func(p string) bool { return p == `C:\fake\bash.exe` }, yes},
			[]string{`C:\fake\powershell.exe`},
		},
		{
			"a unix host offers the one bash it has",
			shellHost{"darwin", fakePath("bash"), no, nil, nil, yes, no, yes},
			[]string{`C:\fake\bash.exe`},
		},
		{
			"a host with nothing offers nothing",
			shellHost{"linux", fakePath(), no, nil, nil, yes, no, yes},
			nil,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := paths(c.host.available())
			if len(got) != len(c.want) {
				t.Fatalf("available = %v, want %v", got, c.want)
			}
			for i := range got {
				if got[i] != c.want[i] {
					t.Fatalf("available = %v, want %v", got, c.want)
				}
			}
		})
	}
}

// Auto-detection's pick has to be the head of the offered list: a settings pane
// that says "自动 → X" while listing Y first is describing a different machine.
func TestAvailableHeadIsWhatAutoPicks(t *testing.T) {
	yes := func(string) bool { return true }
	no := func(string) bool { return false }
	hosts := []shellHost{
		{"windows", fakePath("pwsh"), yes, []string{`C:\fake\Git\bin\bash.exe`}, []string{`C:\fake\PowerShell\7\pwsh.exe`}, yes, no, yes},
		{"windows", fakePath("pwsh", "powershell"), no, nil, nil, yes, no, yes},
		{"darwin", fakePath("bash"), no, nil, nil, yes, no, yes},
	}
	for _, h := range hosts {
		list := h.available()
		if len(list) == 0 {
			t.Fatalf("host %+v offered nothing", h.goos)
		}
		if got := h.auto(); got != list[0] {
			t.Fatalf("auto = %+v, want the first offered %+v", got, list[0])
		}
	}
}

// Git's installer puts only <root>\cmd on PATH, and the first bash.exe on PATH
// is normally the WSL launcher, so a Git installed outside Program Files is
// reachable only through where git.exe lives or a later PATH entry.
func TestBashCandidatesFollowGitAndPath(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("drive-letter paths only split on Windows")
	}
	env := map[string]string{"PATH": `C:\Windows\System32;D:\Tools\Git\usr\bin`}
	getenv := func(k string) string { return env[k] }
	look := func(name string) (string, error) {
		if name == "git" {
			return `E:\dev\Git\cmd\git.exe`, nil
		}
		return "", exec.ErrNotFound
	}
	got := map[string]bool{}
	for _, p := range bashCandidates(getenv, look) {
		got[filepath.ToSlash(p)] = true
	}
	for _, want := range []string{"E:/dev/Git/bin/bash.exe", "E:/dev/Git/usr/bin/bash.exe", "D:/Tools/Git/usr/bin/bash.exe"} {
		if !got[want] {
			t.Errorf("candidates lack %s: %v", want, got)
		}
	}
}

func TestWSLLauncherFoundAsCandidateIsNotOffered(t *testing.T) {
	yes := func(string) bool { return true }
	wsl := `C:\Windows\System32\bash.exe`
	git := `D:\Git\bin\bash.exe`
	h := shellHost{"windows", fakePath(), yes, []string{wsl, git}, nil, yes, func(p string) bool { return p == wsl }, yes}
	if got := paths(h.available()); len(got) != 1 || got[0] != git {
		t.Fatalf("available = %v, want only %s", got, git)
	}
	if sh, ok := h.bash(); !ok || sh.Path != git {
		t.Fatalf("bash() = %v, %v", sh, ok)
	}
}

func wslFixture(t *testing.T) (win, stub string) {
	t.Helper()
	win = filepath.Join(t.TempDir(), "win")
	sys := filepath.Join(win, "System32")
	if err := os.MkdirAll(sys, 0o755); err != nil {
		t.Fatal(err)
	}
	stub = filepath.Join(sys, "bash.exe")
	if err := os.WriteFile(stub, []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}
	return win, stub
}

func TestWSLLauncherPathJudgesByIdentity(t *testing.T) {
	win, stub := wslFixture(t)
	git := filepath.Join(t.TempDir(), "bash.exe")
	if err := os.WriteFile(git, []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}
	if !isWSLLauncherPath(stub, win) {
		t.Error("the launcher is not judged WSL")
	}
	if isWSLLauncherPath(git, win) {
		t.Error("an unrelated bash judged WSL")
	}
	if isWSLLauncherPath(stub, "") {
		t.Error("no Windows directory should establish nothing")
	}
}

func TestWSLLauncherPathFollowsLinks(t *testing.T) {
	win, stub := wslFixture(t)
	other := t.TempDir()
	hard := filepath.Join(other, "hard.exe")
	if err := os.Link(stub, hard); err != nil {
		t.Skip("hard links unavailable:", err)
	}
	if !isWSLLauncherPath(hard, win) {
		t.Error("hard link to the launcher not judged WSL")
	}
	sym := filepath.Join(other, "sym.exe")
	if err := os.Symlink(stub, sym); err != nil {
		t.Skip("symlinks unavailable:", err)
	}
	if !isWSLLauncherPath(sym, win) {
		t.Error("symlink to the launcher not judged WSL")
	}
}

func TestStoreAliasBashIsNeverOfferedOrChosen(t *testing.T) {
	yes := func(string) bool { return true }
	sep := string(filepath.Separator)
	alias := sep + filepath.Join("u", "AppData", "Local", "Microsoft", "WindowsApps", "bash.exe")
	isWSL := func(p string) bool { return p == alias }
	h := shellHost{"windows", fakePath(), yes, []string{alias}, nil, yes, isWSL, yes}
	if got := paths(h.available()); len(got) != 0 {
		t.Fatalf("available = %v, want none", got)
	}
	if sh, ok := h.bash(); ok {
		t.Fatalf("bash() chose %v", sh)
	}
	if got := h.auto(); got.Path != "bash" {
		t.Fatalf("auto = %v", got)
	}
}

func TestBashCandidatesDropRelativePathAndDuplicates(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("drive-letter paths only parse on Windows")
	}
	env := map[string]string{
		"ProgramFiles": `C:\Program Files`,
		"ProgramW6432": `C:\PROGRAM FILES`,
		"PATH":         `sub;.;C:\Program Files\Git\usr\bin;D:\x`,
	}
	look := func(string) (string, error) { return `C:\Program Files\Git\cmd\git.exe`, nil }
	seen := map[string]int{}
	for _, p := range bashCandidates(func(k string) string { return env[k] }, look) {
		if !filepath.IsAbs(p) {
			t.Errorf("relative candidate %q", p)
		}
		seen[strings.ToLower(p)]++
	}
	for p, n := range seen {
		if n > 1 {
			t.Errorf("%s listed %d times", p, n)
		}
	}
	if seen[`d:\x\bash.exe`] != 1 {
		t.Errorf("absolute PATH entry lost: %v", seen)
	}
}
