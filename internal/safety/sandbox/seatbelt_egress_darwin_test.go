package sandbox

import (
	"net"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"
)

// The probe runs inside sandbox-exec as this test binary, so the boundary is
// measured with no interpreter the host might lack.
const egressProbeEnv = "REASONIX_SANDBOX_EGRESS_PROBE"

func TestEgressProbeHelper(t *testing.T) {
	spec := os.Getenv(egressProbeEnv)
	if spec == "" {
		t.Skip("helper process only")
	}
	kind, target, _ := strings.Cut(spec, " ")
	var err error
	switch kind {
	case "tcp", "unix":
		var c net.Conn
		if c, err = net.DialTimeout(kind, target, 3*time.Second); err == nil {
			_ = c.Close()
		}
	case "self":
		var ln net.Listener
		if ln, err = net.Listen("tcp", target); err == nil {
			var c net.Conn
			if c, err = net.DialTimeout("tcp", ln.Addr().String(), 3*time.Second); err == nil {
				_ = c.Close()
			}
			_ = ln.Close()
		}
	}
	if err != nil {
		os.Exit(3)
	}
	os.Exit(0)
}

func listenLoopback(t *testing.T) (net.Listener, int) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			_ = c.Close()
		}
	}()
	return ln, ln.Addr().(*net.TCPAddr).Port
}

// With egress confined, only the proxy port and loopback stay reachable; a
// loopback port closed for a host proxy and every external address do not.
func TestEgressProfileConfinesToTheProxy(t *testing.T) {
	if !Available() {
		t.Skip("sandbox-exec not available")
	}
	_, proxyPort := listenLoopback(t)
	_, closedPort := listenLoopback(t)
	_, devPort := listenLoopback(t)
	sockDir, err := os.MkdirTemp("/tmp", "sbeg")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(sockDir) })
	sock := sockDir + "/s"
	uln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = uln.Close() })
	go func() {
		for {
			c, err := uln.Accept()
			if err != nil {
				return
			}
			_ = c.Close()
		}
	}()

	spec := Spec{Mode: "enforce", Network: true, Egress: fixedRoute(proxyPort), ClosedLoopbackPorts: []int{closedPort}}
	probe := func(sandboxed bool, what string) bool {
		args := []string{os.Args[0], "-test.run=^TestEgressProbeHelper$"}
		if sandboxed {
			var wrapped bool
			if args, wrapped = CommandArgs(spec, args); !wrapped {
				t.Fatal("egress spec was not wrapped")
			}
		}
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Env = append(os.Environ(), egressProbeEnv+"="+what)
		return cmd.Run() == nil
	}
	loop := func(port int) string { return "tcp 127.0.0.1:" + strconv.Itoa(port) }
	for what, want := range map[string]bool{
		loop(proxyPort):    true,
		loop(devPort):      true,
		loop(closedPort):   false,
		"self 127.0.0.1:0": true,
		"self [::1]:0":     true,
		"unix " + sock:     true,
	} {
		if got := probe(true, what); got != want {
			t.Errorf("%s reachable = %v, want %v", what, got, want)
		}
	}
	const external = "tcp 1.1.1.1:443"
	if !probe(false, external) {
		t.Log("no external network on this host; skipping the external probe")
		return
	}
	if probe(true, external) {
		t.Error("a confined command reached an external address directly")
	}
}

func TestEgressRulesOnlyWhenNetworkIsOn(t *testing.T) {
	if p := seatbeltProfile(Spec{Network: false, Egress: fixedRoute(4000)}); strings.Contains(p, "localhost:4000") {
		t.Fatalf("network off still opened the proxy port:\n%s", p)
	}
	if p := seatbeltProfile(Spec{Network: true}); strings.Contains(p, "(deny network*)") {
		t.Fatalf("open network without a proxy denied all network:\n%s", p)
	}
}

type fixedRoute int

func (r fixedRoute) Port() int              { return int(r) }
func (fixedRoute) Env(string) []string      { return nil }
func (fixedRoute) Refusals(string) []string { return nil }
