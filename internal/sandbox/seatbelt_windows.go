//go:build windows

package sandbox

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"reasonix/internal/winsandbox"
)

type windowsSandboxPayload struct {
	Version  int  `json:"version"`
	Spec     Spec `json:"spec"`
	Writable bool `json:"writable"`
}

const windowsSandboxPayloadVersion = 1

// Command returns the shell invocation unwrapped: the Windows backend is
// retired from enforcement (Available is always false), so the helper argv is
// never produced. The wrapping code remains only until the backend is removed.
func Command(spec Spec, sh Shell, command string) ([]string, bool) {
	if ValidateShellPolicy(spec, sh) != nil {
		return nil, false
	}
	// Shells must remain able to read ordinary user files, so they use the
	// restricted-token lane in every constrained preset. Read-only receives no
	// workspace write ACL; workspace access receives only its canonical roots.
	return windowsSandboxCommand(spec, sh.argv(command), true)
}

// CommandArgs is like Command but accepts the command as raw argv instead of a
// shell command string.
func CommandArgs(spec Spec, args []string) ([]string, bool) {
	return windowsSandboxCommand(spec, args, spec.DirectWrites)
}

func windowsSandboxCommand(spec Spec, args []string, writable bool) ([]string, bool) {
	// Mirror the darwin/linux wrappers: when the backend (or this binary's
	// helper dispatch) is unavailable, return the argv unwrapped so the bash
	// tool takes its explicit fail-closed / escape-approval path instead of
	// spawning a helper that cannot work.
	if !spec.Enforce() || !Available() {
		return args, false
	}
	exe, err := os.Executable()
	if err != nil || exe == "" {
		return args, false
	}
	payload, err := encodeWindowsSandboxPayload(windowsSandboxPayload{Spec: spec, Writable: writable})
	if err != nil {
		return args, false
	}
	out := []string{exe, WindowsHelperCommand, payload, "--"}
	out = append(out, args...)
	return out, true
}

// Available is always false on Windows: the restricted-token/AppContainer
// backend is retired from enforcement (see OSSandboxSupported). Reporting it
// as unavailable keeps every wrapper, MCP confinement, and capability surface
// on the unwrapped path without a second platform switch, and it guarantees
// the helper never mutates ACLs or creates restricted tokens on user machines.
func Available() bool { return false }

func repairLegacyCredentialDeny(path string) error {
	return winsandbox.RepairLegacyCredentialDeny(path)
}

func encodeWindowsSandboxPayload(payload windowsSandboxPayload) (string, error) {
	payload.Version = windowsSandboxPayloadVersion
	b, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func decodeWindowsSandboxPayload(s string) (windowsSandboxPayload, error) {
	var payload windowsSandboxPayload
	b, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return payload, err
	}
	decoder := json.NewDecoder(bytes.NewReader(b))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&payload); err != nil {
		return payload, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return payload, fmt.Errorf("multiple JSON values")
		}
		return payload, err
	}
	if payload.Version != windowsSandboxPayloadVersion {
		return payload, fmt.Errorf("unsupported payload version %d", payload.Version)
	}
	return payload, nil
}

// RunWindowsSandboxHelper is the hidden process entry point used by Command and
// CommandArgs on native Windows.
func RunWindowsSandboxHelper(args []string, stdin *os.File, stdout *os.File, stderr *os.File) int {
	report, reportErr := openRunnerReport()
	if reportErr != nil {
		fmt.Fprintln(stderr, "windows sandbox report channel:", reportErr)
		return 126
	}
	if report != nil {
		defer report.Close()
	}
	reportFailure := func(phase string, err error) int {
		if report != nil {
			_ = writeRunnerReport(report, phase, err)
		}
		fmt.Fprintln(stderr, "windows sandbox:", err)
		return 126
	}
	if len(args) < 3 || args[1] != "--" {
		return reportFailure(WindowsSandboxFailureLaunch, fmt.Errorf("usage: reasonix %s <payload> -- <command> [args...]", WindowsHelperCommand))
	}
	payload, err := decodeWindowsSandboxPayload(args[0])
	if err != nil {
		return reportFailure(WindowsSandboxFailureDependency, fmt.Errorf("invalid windows sandbox payload: %w", err))
	}
	child := args[2:]
	if len(child) == 0 || strings.TrimSpace(child[0]) == "" {
		return reportFailure(WindowsSandboxFailureLaunch, fmt.Errorf("windows sandbox command is required"))
	}
	result, err := winsandbox.Run(convertWindowsSandboxSpec(payload.Spec, payload.Writable), child, winsandbox.RunOptions{
		Stdin:  stdin,
		Stdout: stdout,
		Stderr: stderr,
	})
	if err != nil {
		if phase, preCommand := windowsSandboxFailurePhase(err); preCommand {
			return reportFailure(phase, err)
		} else {
			// The process was resumed before this failure. Keep it in the normal
			// execution lane so mutation risk remains may_be_partial.
			fmt.Fprintln(stderr, "windows sandbox:", err)
		}
		return 126
	}
	return result.ExitCode
}

func windowsSandboxFailurePhase(err error) (string, bool) {
	if phase, ok := winsandbox.FailurePhaseOf(err); ok {
		return string(phase), true
	}
	if errors.Is(err, winsandbox.ErrUnsupported) {
		return WindowsSandboxFailureDependency, true
	}
	return "", false
}

func convertWindowsSandboxSpec(spec Spec, writable bool) winsandbox.Spec {
	roots := spec.WriteRoots
	if spec.ReadOnly {
		roots = spec.AppContainerWriteRoots
	}
	readableRoots := append([]string(nil), spec.ReadRoots...)
	return winsandbox.Spec{
		ReadableRoots:       readableRoots,
		WritableRoots:       append([]string(nil), roots...),
		ForbidReadRoots:     append([]string(nil), spec.ForbidReadRoots...),
		ProtectedWriteRoots: append([]string(nil), spec.ProtectedWriteRoots...),
		Network:             spec.Network,
		Writable:            writable,
		ReadOnly:            spec.ReadOnly,
		TempDir:             spec.SessionTemp,
		TempPrefix:          "reasonix-sandbox-",
		LockWait:            spec.WindowsLockWait,
	}
}
