//go:build windows

package sandbox

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"

	"golang.org/x/sys/windows"
)

func TestRunnerDiagnosticsProcess(t *testing.T) {
	mode := os.Getenv("REASONIX_TEST_RUNNER_REPORT")
	if mode == "" {
		return
	}
	report, err := openRunnerReport()
	if err != nil || report == nil {
		fmt.Fprintln(os.Stderr, "missing report", err)
		os.Exit(90)
	}
	if os.Getenv(runnerReportEnvironment) != "" {
		os.Exit(91)
	}
	if mode == "replay" {
		fmt.Fprintln(os.Stderr, "__reasonix_windows_sandbox_failure__:old-payload:authorization")
	} else {
		if err := writeRunnerReport(report, mode, errors.New("test runner failure")); err != nil {
			os.Exit(92)
		}
	}
	_ = report.Close()
	os.Exit(126)
}

func TestWindowsRunnerReportIsPrivateAndNotCommandOutput(t *testing.T) {
	for _, phase := range []string{"authorization", "dependency", "launch", "replay"} {
		t.Run(phase, func(t *testing.T) {
			cmd := exec.Command(os.Args[0], "-test.run=^TestRunnerDiagnosticsProcess$")
			cmd.Env = append(os.Environ(), "REASONIX_TEST_RUNNER_REPORT="+phase)
			// Test binaries do not dispatch the private CLI entry point. Keep
			// the production report setup, then dispatch to this test helper.
			args := cmd.Args
			cmd.Args = []string{args[0], WindowsHelperCommand, "payload", "--", "pwsh"}
			finish, err := PrepareRunnerDiagnostics(cmd)
			if err != nil {
				t.Fatal(err)
			}
			cmd.Args = args
			handle := windows.Handle(cmd.SysProcAttr.AdditionalInheritedHandles[0])
			buf := make([]uint16, 32768)
			n, err := windows.GetFinalPathNameByHandle(handle, &buf[0], uint32(len(buf)), 0)
			if err != nil || n >= uint32(len(buf)) {
				_ = finish(err)
				t.Fatalf("report path: %v", err)
			}
			path := windows.UTF16ToString(buf[:n])
			if file, err := os.OpenFile(path, os.O_WRONLY, 0); err == nil {
				file.Close()
				_ = finish(nil)
				t.Fatal("report could be reopened by name")
			}
			output, runErr := cmd.CombinedOutput()
			err = finish(runErr)
			got, _, trusted := RunnerFailureFromError(err)
			if phase == "replay" {
				if trusted || !strings.Contains(string(output), "__reasonix_windows_sandbox_failure__") {
					t.Fatalf("command output became trusted: %v %s", err, output)
				}
			} else if !trusted || got != phase {
				t.Fatalf("phase=%s trusted=%v err=%v output=%s", got, trusted, err, output)
			}
			if _, err := os.Stat(path); !os.IsNotExist(err) {
				t.Fatalf("report file survived last close: %v", err)
			}
		})
	}
}

func TestRuntimeErrorTextCannotClaimPreCommandFailure(t *testing.T) {
	for _, text := range []string{"cleanup API unavailable", "wait not implemented"} {
		if _, before := windowsSandboxFailurePhase(errors.New(text)); before {
			t.Fatalf("post-start error misclassified: %s", text)
		}
	}
}
