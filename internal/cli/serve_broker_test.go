package cli

import (
	"flag"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func brokerFlagsFor(t *testing.T, args ...string) brokerFlags {
	t.Helper()
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	b := registerBrokerFlags(fs)
	if err := fs.Parse(args); err != nil {
		t.Fatalf("parse %v: %v", args, err)
	}
	return b
}

func writeTokenFile(t *testing.T, token string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "broker.token")
	if err := os.WriteFile(path, []byte(token+"\n"), 0o600); err != nil {
		t.Fatalf("write token: %v", err)
	}
	return path
}

// The address arrives from the forward that published it. A non-loopback one
// means something other than the tunnel is carrying the conversation, and the
// whole conversation — the workspace in it — is what these requests send.
func TestBrokerRefusesANonLoopbackAddress(t *testing.T) {
	refused := []string{
		"http://10.0.0.4:8080",
		"http://broker.example.com:8080",
		"https://127.0.0.1:8080",
		"http://0.0.0.0:8080",
	}
	tokenFile := writeTokenFile(t, "t")
	for _, addr := range refused {
		t.Run(addr, func(t *testing.T) {
			b := brokerFlagsFor(t, "--provider-broker", addr, "--provider-broker-token-file", tokenFile)
			if _, err := b.resolver(); err == nil {
				t.Fatalf("%s was accepted as a broker address", addr)
			}
		})
	}
}

func TestBrokerAcceptsLoopback(t *testing.T) {
	tokenFile := writeTokenFile(t, "secret")
	for _, addr := range []string{"http://127.0.0.1:41235", "127.0.0.1:41235", "http://localhost:41235", "http://[::1]:41235"} {
		t.Run(addr, func(t *testing.T) {
			b := brokerFlagsFor(t, "--provider-broker", addr, "--provider-broker-token-file", tokenFile)
			resolver, err := b.resolver()
			if err != nil {
				t.Fatalf("%s was refused: %v", addr, err)
			}
			if resolver == nil {
				t.Fatal("a configured broker produced no resolver")
			}
		})
	}
}

// No broker flag is the ordinary local serve, which must keep reading its own
// config rather than being handed a resolver that resolves nothing.
func TestNoBrokerLeavesTheLocalPath(t *testing.T) {
	b := brokerFlagsFor(t)
	resolver, err := b.resolver()
	if err != nil {
		t.Fatalf("resolver: %v", err)
	}
	if resolver != nil {
		t.Fatalf("an unflagged serve got a broker resolver: %#v", resolver)
	}
}

// An address with no token would be a broker nobody authenticates to.
func TestBrokerRequiresAToken(t *testing.T) {
	b := brokerFlagsFor(t, "--provider-broker", "http://127.0.0.1:41235")
	if _, err := b.resolver(); err == nil {
		t.Fatal("a broker without a token file was accepted")
	}
}

// The token file is read with serve's own rules: a world-readable one is every
// account on the machine holding a key to the user's model spend.
func TestBrokerTokenFileMustBePrivate(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permission bits do not decide access here")
	}
	path := writeTokenFile(t, "secret")
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	b := brokerFlagsFor(t, "--provider-broker", "http://127.0.0.1:41235", "--provider-broker-token-file", path)
	_, err := b.resolver()
	if err == nil {
		t.Fatal("a world-readable token file was accepted")
	}
	if !strings.Contains(err.Error(), "chmod 600") {
		t.Fatalf("the refusal did not name the fix: %v", err)
	}
}
