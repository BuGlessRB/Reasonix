package sandbox

import (
	"os/exec"
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
			shellHost{"windows", fakePath("pwsh", "powershell"), yes, gitBash, winPS, yes, no},
			[]string{`C:\fake\Git\bin\bash.exe`, `C:\fake\PowerShell\7\pwsh.exe`, `C:\fake\System32\powershell.exe`},
		},
		{
			// The same bash reached twice — once on PATH, once as the Git
			// candidate — is one interpreter, and two rows offering it would ask
			// the user to choose between a thing and itself.
			"a bash found twice is listed once",
			shellHost{"windows", fakePath("bash"), func(p string) bool { return p == `C:\fake\bash.exe` }, []string{`C:\fake\bash.exe`}, nil, yes, no},
			[]string{`C:\fake\bash.exe`},
		},
		{
			// The WSL launcher runs commands inside the Linux VM, where the
			// workspace is a /mnt path; offering it would hand the agent a shell
			// that cannot see the files it was pointed at.
			"the wsl launcher is not on offer",
			shellHost{"windows", fakePath("bash", "powershell"), no, nil, winPS, yes, func(p string) bool { return p == `C:\fake\bash.exe` }},
			[]string{`C:\fake\powershell.exe`},
		},
		{
			"a unix host offers the one bash it has",
			shellHost{"darwin", fakePath("bash"), no, nil, nil, yes, no},
			[]string{`C:\fake\bash.exe`},
		},
		{
			"a host with nothing offers nothing",
			shellHost{"linux", fakePath(), no, nil, nil, yes, no},
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
		{"windows", fakePath("pwsh"), yes, []string{`C:\fake\Git\bin\bash.exe`}, []string{`C:\fake\PowerShell\7\pwsh.exe`}, yes, no},
		{"windows", fakePath("pwsh", "powershell"), no, nil, nil, yes, no},
		{"darwin", fakePath("bash"), no, nil, nil, yes, no},
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
