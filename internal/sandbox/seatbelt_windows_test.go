//go:build windows

package sandbox

import (
	"encoding/base64"
	"testing"
)

// The Windows backend is retired from enforcement: even with the helper
// dispatch registered and the native APIs present, no launch path may wrap a
// command, and the effective spec must never demand confinement.
func TestWindowsNeverWrapsCommands(t *testing.T) {
	RegisterHelperDispatch()
	if Available() {
		t.Fatal("Available() must be false on Windows")
	}
	if OSSandboxSupported() {
		t.Fatal("OSSandboxSupported() must be false on Windows")
	}
	spec := Spec{Mode: "enforce", WriteRoots: []string{`C:\work`}, Network: true}
	sh := Shell{Kind: ShellPowerShell, Path: "powershell"}
	if argv, wrapped := Command(spec, sh, "Write-Output ok"); wrapped || len(argv) == 0 || argv[0] != sh.Path {
		t.Fatalf("Command wrapped=%v argv=%v", wrapped, argv)
	}
	if argv, wrapped := CommandArgs(spec, []string{`C:\tools\rg.exe`, "needle"}); wrapped || argv[0] != `C:\tools\rg.exe` {
		t.Fatalf("CommandArgs wrapped=%v argv=%v", wrapped, argv)
	}
	for _, launch := range []Prepared{
		PrepareShell(spec, sh, "exit 0", `C:\private-temp`),
		PrepareShellArgs(spec, []string{sh.Path, "-NoLogo", "-NoProfile"}, `C:\private-temp`),
	} {
		if launch.Wrapped {
			t.Fatalf("prepared launch wrapped: %+v", launch)
		}
	}
}

func TestConvertWindowsSandboxSpec(t *testing.T) {
	sessionTemp := `C:\private\session-temp`
	protected := `C:\Users\agent\AppData\Roaming\Reasonix`
	spec := Spec{
		Mode:                "enforce",
		WriteRoots:          []string{`C:\work`},
		ForbidReadRoots:     []string{`C:\work\secret`},
		ProtectedWriteRoots: []string{protected},
		SessionTemp:         sessionTemp,
		Network:             true,
	}
	got := convertWindowsSandboxSpec(spec, true)
	if !got.Writable || !got.Network || got.TempPrefix != "reasonix-sandbox-" {
		t.Fatalf("converted flags = %+v", got)
	}
	if len(got.WritableRoots) != 1 || got.WritableRoots[0] != spec.WriteRoots[0] {
		t.Fatalf("converted writable roots = %v", got.WritableRoots)
	}
	if len(got.ForbidReadRoots) != 1 || got.ForbidReadRoots[0] != spec.ForbidReadRoots[0] {
		t.Fatalf("converted forbid roots = %v", got.ForbidReadRoots)
	}
	if got.TempDir != sessionTemp {
		t.Fatalf("converted temp dir = %q, want %q", got.TempDir, sessionTemp)
	}
	if len(got.ProtectedWriteRoots) != 1 || got.ProtectedWriteRoots[0] != protected {
		t.Fatalf("converted protected roots = %v", got.ProtectedWriteRoots)
	}
	if len(got.WritableRoots) != 1 {
		t.Fatalf("session temp must remain a separate capability, got roots %v", got.WritableRoots)
	}

	got.WritableRoots[0] = `C:\mutated`
	got.ForbidReadRoots[0] = `C:\mutated`
	if spec.WriteRoots[0] == got.WritableRoots[0] || spec.ForbidReadRoots[0] == got.ForbidReadRoots[0] {
		t.Fatal("converted spec should not alias Reasonix slices")
	}
}

func TestWindowsSandboxPayloadRejectsUnknownOrStaleProtocol(t *testing.T) {
	for name, raw := range map[string]string{
		"missing version": `{"spec":{},"writable":true}`,
		"future version":  `{"version":2,"spec":{},"writable":true}`,
		"unknown field":   `{"version":1,"spec":{},"writable":true,"surprise":1}`,
		"trailing value":  `{"version":1,"spec":{},"writable":true} {}`,
	} {
		t.Run(name, func(t *testing.T) {
			encoded := base64.RawURLEncoding.EncodeToString([]byte(raw))
			if _, err := decodeWindowsSandboxPayload(encoded); err == nil {
				t.Fatalf("decode accepted invalid payload %s", raw)
			}
		})
	}
}
