package bootstrap

import (
	"context"
	"errors"
	"strings"
	"testing"

	"reasonix/internal/remote"
	"reasonix/internal/remote/sftpfs"
)

func TestParseUname(t *testing.T) {
	cases := []struct {
		in           string
		goos, goarch string
		wantErr      bool
	}{
		{"Linux x86_64", "linux", "amd64", false},
		{"Linux aarch64", "linux", "arm64", false},
		{"Darwin arm64", "darwin", "arm64", false},
		{"Darwin x86_64", "darwin", "amd64", false},
		{"Linux armv7l", "linux", "arm", false},
		{"  Linux   x86_64  \n", "linux", "amd64", false},
		{"MINGW64_NT-10.0 x86_64", "", "", true}, // Windows shell
		{"Linux mips", "", "", true},
		{"garbage", "", "", true},
	}
	for _, c := range cases {
		goos, goarch, err := ParseUname(c.in)
		if c.wantErr {
			if err == nil {
				t.Errorf("ParseUname(%q): expected error", c.in)
			}
			continue
		}
		if err != nil || goos != c.goos || goarch != c.goarch {
			t.Errorf("ParseUname(%q) = (%q,%q,%v), want (%q,%q)", c.in, goos, goarch, err, c.goos, c.goarch)
		}
	}
}

func TestParseVersion(t *testing.T) {
	cases := map[string]string{
		"reasonix v1.9.0":        "1.9.0",
		"1.9.0":                  "1.9.0",
		"reasonix version 2.0.1": "2.0.1",
		"v1.10.0-rc.1":           "1.10.0-rc.1",
	}
	for in, want := range cases {
		got, err := ParseVersion(in)
		if err != nil || got != want {
			t.Errorf("ParseVersion(%q) = (%q,%v), want %q", in, got, err, want)
		}
	}
	if _, err := ParseVersion("no version here"); err == nil {
		t.Error("expected error for versionless output")
	}
}

func TestCompareVersions(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"1.9.0", "1.9.0", 0},
		{"1.9.0", "1.10.0", -1},
		{"1.10.0", "1.9.0", 1},
		{"2.0.0", "1.99.99", 1},
		{"1.9", "1.9.0", 0},
		{"1.9.1-rc.1", "1.9.1", 0}, // pre-release ignored for ordering
	}
	for _, c := range cases {
		if got := CompareVersions(c.a, c.b); got != c.want {
			t.Errorf("CompareVersions(%q,%q) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}

// TestLaunchCommandQuotesHostilePaths is the security golden: a workspace or
// log path containing shell metacharacters must be fully single-quoted so it
// cannot break out of the launch command.
func TestLaunchCommandQuotesHostilePaths(t *testing.T) {
	paths := StatePaths{
		Dir:       "/home/dev/.reasonix/remote",
		TokenFile: "/home/dev/.reasonix/remote/serve-x.token",
		PortFile:  "/home/dev/.reasonix/remote/serve-x.port",
		PidFile:   "/home/dev/.reasonix/remote/serve-x.pid",
		LogFile:   "/home/dev/.reasonix/remote/serve-x.log",
	}
	hostile := "/tmp/'; rm -rf ~; echo '"
	cmd := LaunchCommand(LaunchSpec{Bin: "/usr/bin/reasonix", Workspace: hostile}, paths)

	// The hostile workspace must appear only inside a quoted operand, escaped.
	if strings.Contains(cmd, "; rm -rf ~; echo") && !strings.Contains(cmd, `'\''; rm -rf ~; echo '\''`) {
		t.Fatalf("hostile workspace not properly escaped:\n%s", cmd)
	}
	// No unescaped `rm -rf` sequence that would execute.
	if strings.Contains(cmd, "cd /tmp/'; rm -rf") {
		t.Fatalf("workspace broke out of quoting:\n%s", cmd)
	}
	// Sanity: the essential flags are present.
	for _, want := range []string{"--addr 127.0.0.1:0", "--auth token", "--token-file", "--port-file", "$SX nohup", "echo $!"} {
		if !strings.Contains(cmd, want) {
			t.Errorf("launch command missing %q:\n%s", want, cmd)
		}
	}
}

func TestShellQuote(t *testing.T) {
	cases := map[string]string{
		"simple":      "'simple'",
		"has space":   "'has space'",
		"a'b":         `'a'\''b'`,
		"'; rm -rf ~": `''\''; rm -rf ~'`,
	}
	for in, want := range cases {
		if got := shellQuote(in); got != want {
			t.Errorf("shellQuote(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestStopAndServeAliveCommands(t *testing.T) {
	paths := StatePaths{TokenFile: "/state/ws.token", PortFile: "/state/ws.port"}
	stop := StopCommand(4321, paths)
	for _, want := range []string{"kill -TERM 4321", "kill -0 4321", "kill -KILL 4321"} {
		if !strings.Contains(stop, want) {
			t.Errorf("StopCommand missing %q: %s", want, stop)
		}
	}
	alive := ServeAliveCommand(99, paths)
	// Must check liveness AND that the process is a reasonix serve (guards PID
	// reuse), not just kill -0.
	for _, want := range []string{"kill -0 99", "ps -p 99", "*reasonix*serve*", paths.TokenFile, paths.PortFile} {
		if !strings.Contains(alive, want) {
			t.Errorf("ServeAliveCommand missing %q: %s", want, alive)
		}
	}
	if strings.Count(stop, "ours") < 3 {
		t.Fatalf("StopCommand must revalidate ownership during TERM/KILL wait: %s", stop)
	}
}

func TestLaunchCommandDetachAndLogHardening(t *testing.T) {
	cmd := LaunchCommand(LaunchSpec{Bin: "/usr/bin/reasonix", Workspace: "/ws"}, StatePaths{
		Dir: "/d", TokenFile: "/d/t", PortFile: "/d/p", PidFile: "/d/i", LogFile: "/d/l",
	})
	// setsid must be optional (macOS lacks it) and the log created 0600 so the
	// serve token line (already suppressed under --port-file) can't leak.
	for _, want := range []string{"command -v setsid", "$SX nohup", "chmod 600", "umask 077", "--port-file"} {
		if !strings.Contains(cmd, want) {
			t.Errorf("LaunchCommand missing %q:\n%s", want, cmd)
		}
	}
	if !strings.Contains(cmd, "rm -f '/d/p' '/d/i'") {
		t.Fatalf("LaunchCommand does not clear stale port/pid files before launch:\n%s", cmd)
	}
	if strings.Contains(cmd, "setsid nohup") {
		t.Errorf("setsid must be conditional, not hard-wired:\n%s", cmd)
	}
}

func TestLocateCommandProbesPortFileFlag(t *testing.T) {
	cmd := LocateCommand("/home/x/.reasonix/remote/bin/reasonix")
	for _, want := range []string{"serve --help", "port-file", "flag yes", "flag no", "--version"} {
		if !strings.Contains(cmd, want) {
			t.Errorf("LocateCommand missing %q:\n%s", want, cmd)
		}
	}
}

// One place is not every place. A machine can hold an old reasonix on PATH and
// a current one this bootstrap uploaded beside it, and a probe that stopped at
// the first path it found reported only the one that cannot be used.
func TestLocateCommandReportsEveryCandidateItFinds(t *testing.T) {
	cmd := LocateCommand("/home/x/.reasonix/remote/bin/reasonix")
	for _, want := range []string{"command -v reasonix", "/home/x/.reasonix/remote/bin/reasonix", "npm prefix -g"} {
		if !strings.Contains(cmd, want) {
			t.Errorf("LocateCommand does not look in %q:\n%s", want, cmd)
		}
	}
}

// The probe's records, read back. Order is the order it looked, because that
// is what decides which usable one wins.
func TestParseCandidatesReadsOneRecordPerBinary(t *testing.T) {
	got := parseCandidates("bin /usr/bin/reasonix\nver reasonix 1.31.4\nflag yes\n" +
		"bin /home/x/.reasonix/remote/bin/reasonix\nver reasonix v2.7.0\nflag yes\n" +
		"bin /opt/old/reasonix\nver reasonix dev\nflag no\n")
	want := []candidate{
		{path: "/usr/bin/reasonix", version: "1.31.4", portFile: true},
		{path: "/home/x/.reasonix/remote/bin/reasonix", version: "2.7.0", portFile: true},
		{path: "/opt/old/reasonix", portFile: false},
	}
	if len(got) != len(want) {
		t.Fatalf("parsed %d candidates, want %d: %+v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("candidate %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}

// A version below the floor is not a usable binary, and a version nothing
// could read is: a source build calls itself "dev", and refusing those would
// take the bootstrap away from everyone developing against it.
func TestUsableTakesTheFloorAndForgivesAnUnreadableVersion(t *testing.T) {
	for _, tc := range []struct {
		name string
		c    candidate
		want bool
	}{
		{"old line", candidate{path: "/usr/bin/reasonix", version: "1.31.4", portFile: true}, false},
		{"this line", candidate{path: "/usr/bin/reasonix", version: "2.7.0", portFile: true}, true},
		{"the floor itself", candidate{path: "/usr/bin/reasonix", version: "2.0.0", portFile: true}, true},
		{"source build", candidate{path: "/usr/bin/reasonix", portFile: true}, true},
		{"no port-file flag", candidate{path: "/usr/bin/reasonix", version: "2.7.0"}, false},
	} {
		if got := tc.c.usable(MinPaneVersion); got != tc.want {
			t.Errorf("%s: usable = %v, want %v", tc.name, got, tc.want)
		}
	}
}

// probeConn answers the locate probe and nothing else: choosing between the
// binaries a machine holds needs no file layer.
type probeConn struct{ out string }

func (c probeConn) Exec(context.Context, string) (remote.ExecResult, error) {
	return remote.ExecResult{Stdout: []byte(c.out)}, nil
}

func (probeConn) SFTP() (*sftpfs.FS, error) { return nil, errors.New("no file layer here") }

// A machine can hold both, and it usually does after this bootstrap has
// uploaded one: the old install stays on PATH and answers `command -v` first.
// Taking the first path found is how the upload was spent and then ignored.
func TestLocateTakesTheUsableBinaryNotTheFirstOne(t *testing.T) {
	uploaded := "/home/x/.reasonix/remote/bin/reasonix"
	conn := probeConn{out: "bin /usr/bin/reasonix\nver reasonix 1.31.4\nflag yes\n" +
		"bin " + uploaded + "\nver reasonix v2.7.0\nflag yes\n"}
	bin, version := locate(context.Background(), conn, posixShell{}, uploaded, MinPaneVersion)
	if bin != uploaded || version != "2.7.0" {
		t.Fatalf("locate = %q %q, want the uploaded 2.7.0 over the 1.31.4 on PATH", bin, version)
	}
	// Without a floor the caller wants whatever is there, and PATH comes first.
	if bin, _ := locate(context.Background(), conn, posixShell{}, uploaded, ""); bin != "/usr/bin/reasonix" {
		t.Fatalf("locate without a floor = %q, want the one on PATH", bin)
	}
}

// "Has none" and "has one from the older line" are different next moves, so
// they are different answers. The version of the newest one turned down is
// what a reader needs to act.
func TestOutdatedNamesTheNewestOneTurnedDown(t *testing.T) {
	found := []candidate{
		{path: "/usr/bin/reasonix", version: "1.29.0", portFile: true},
		{path: "/opt/reasonix", version: "1.31.4", portFile: true},
		{path: "/src/reasonix", portFile: true},
	}
	if got := outdated(found, MinPaneVersion); got != "1.31.4" {
		t.Fatalf("outdated = %q, want 1.31.4", got)
	}
	if got := outdated([]candidate{{path: "/usr/bin/reasonix", version: "2.7.0", portFile: true}}, MinPaneVersion); got != "" {
		t.Fatalf("outdated = %q, want nothing: this machine has no old kernel", got)
	}
}

// The broker token authenticates a remote kernel to the model credentials at
// home, so it rides a file for the same reason the serve token does: argv is
// readable by every account on the machine, through `ps`.
func TestLaunchCarriesTheBrokerByFileNotArgv(t *testing.T) {
	paths := StatePaths{
		Dir: "/d", TokenFile: "/d/t", BrokerTokenFile: "/d/b",
		PortFile: "/d/p", PidFile: "/d/i", LogFile: "/d/l",
	}
	spec := LaunchSpec{Bin: "/usr/bin/reasonix", Workspace: "/ws", BrokerAddr: "127.0.0.1:41235"}
	cmd := LaunchCommand(spec, paths)

	for _, want := range []string{"--provider-broker '127.0.0.1:41235'", "--provider-broker-token-file '/d/b'"} {
		if !strings.Contains(cmd, want) {
			t.Fatalf("launch command is missing %q:\n%s", want, cmd)
		}
	}
	if strings.Contains(cmd, "secret-broker-token") {
		t.Fatal("the broker token reached argv")
	}
}

// A launch with no broker is the ordinary one, and must stay byte-identical:
// the flags a kernel does not understand are the flags an older one refuses.
func TestLaunchWithoutABrokerNamesNoBrokerFlags(t *testing.T) {
	paths := StatePaths{
		Dir: "/d", TokenFile: "/d/t", BrokerTokenFile: "/d/b",
		PortFile: "/d/p", PidFile: "/d/i", LogFile: "/d/l",
	}
	cmd := LaunchCommand(LaunchSpec{Bin: "/usr/bin/reasonix", Workspace: "/ws"}, paths)
	if strings.Contains(cmd, "provider-broker") {
		t.Fatalf("an unconfigured broker still reached the command line:\n%s", cmd)
	}
}

// The address is an operand like every other, and a hostile one must not break
// out of its quoting.
func TestLaunchQuotesTheBrokerAddress(t *testing.T) {
	paths := StatePaths{
		Dir: "/d", TokenFile: "/d/t", BrokerTokenFile: "/d/b",
		PortFile: "/d/p", PidFile: "/d/i", LogFile: "/d/l",
	}
	hostile := "127.0.0.1:1'; rm -rf ~; echo '"
	cmd := LaunchCommand(LaunchSpec{Bin: "/r", Workspace: "/ws", BrokerAddr: hostile}, paths)
	if strings.Contains(cmd, "; rm -rf ~; echo") && !strings.Contains(cmd, `'\''; rm -rf ~; echo '\''`) {
		t.Fatalf("hostile broker address not properly escaped:\n%s", cmd)
	}
}
