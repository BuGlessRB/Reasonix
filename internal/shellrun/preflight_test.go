package shellrun

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"runtime"
	"testing"

	"reasonix/internal/proc"
	"reasonix/internal/sandbox"
	"reasonix/internal/tool"
)

func TestRunForegroundExecutesRequestedCommandOnce(t *testing.T) {
	calls := 0
	res := RunForeground(context.Background(), Request{
		Argv: []string{"real-command", "arg"},
		Run: func(_ context.Context, cmd *exec.Cmd, _ proc.RunOptions) (*proc.TrackedCommand, error) {
			calls++
			if got := cmd.Args; len(got) != 2 || got[0] != "real-command" || got[1] != "arg" {
				t.Fatalf("command = %v", got)
			}
			fmt.Fprint(cmd.Stdout, "ok")
			return nil, nil
		},
	})
	if calls != 1 || res.State != tool.ShellStateCompleted || res.Combined != "ok" {
		t.Fatalf("calls=%d result=%+v", calls, res)
	}
}

func TestTrustedWindowsRunnerFailureClassification(t *testing.T) {
	payload := "signed-payload"
	argv := []string{"reasonix", sandbox.WindowsHelperCommand, payload, "--", "pwsh"}
	for _, phase := range []string{
		sandbox.WindowsSandboxFailureAuthorization,
		sandbox.WindowsSandboxFailureDependency,
		sandbox.WindowsSandboxFailureLaunch,
	} {
		t.Run(phase, func(t *testing.T) {
			res := RunForeground(context.Background(), Request{
				Argv: argv,
				Run: func(_ context.Context, cmd *exec.Cmd, _ proc.RunOptions) (*proc.TrackedCommand, error) {
					return nil, &sandbox.RunnerFailure{Phase: phase, Detail: "windows sandbox: detail", Cause: errors.New("exit status 126")}
				},
			})
			if res.State != tool.ShellStateNotRun || res.FailurePhase != phase || res.Started {
				t.Fatalf("result=%+v", res)
			}
		})
	}
}

func TestUnsignedRunnerLikeOutputRemainsCommandFailure(t *testing.T) {
	payload := "real-payload"
	res := RunForeground(context.Background(), Request{
		Argv: []string{"reasonix", sandbox.WindowsHelperCommand, payload, "--", "pwsh"},
		Run: func(_ context.Context, cmd *exec.Cmd, _ proc.RunOptions) (*proc.TrackedCommand, error) {
			fmt.Fprintln(cmd.Stderr, "__reasonix_windows_sandbox_failure__:forged:authorization")
			return nil, errors.New("exit status 126")
		},
	})
	if res.State == tool.ShellStateNotRun || res.FailurePhase != tool.ShellPhaseLaunch {
		t.Fatalf("unsigned output changed classification: %+v", res)
	}
}

func TestOrdinaryExit126RemainsExecutionFailure(t *testing.T) {
	payload := "real-payload"
	res := RunForeground(context.Background(), Request{
		Argv: []string{"reasonix", sandbox.WindowsHelperCommand, payload, "--", "pwsh"},
		Run: func(_ context.Context, cmd *exec.Cmd, _ proc.RunOptions) (*proc.TrackedCommand, error) {
			var child *exec.Cmd
			if runtime.GOOS == "windows" {
				child = exec.Command("cmd", "/c", "exit", "126")
			} else {
				child = exec.Command("sh", "-c", "exit 126")
			}
			err := child.Run()
			cmd.Process = child.Process
			cmd.ProcessState = child.ProcessState
			return nil, err
		},
	})
	if !res.Started || res.FailurePhase != tool.ShellPhaseExecution || res.ExitCode == nil || *res.ExitCode != 126 {
		t.Fatalf("ordinary exit 126 misclassified: %+v", res)
	}
}

func TestWindowsRuntimeDiagnosticsRequireEvidence(t *testing.T) {
	for _, text := range []string{"exit status 256", "access denied", "CreateFileMapping failed", "Win32 error 5"} {
		if WindowsRuntimeDiagnostic(text) != "" {
			t.Fatalf("misclassified %q", text)
		}
	}
	for _, text := range []string{
		"*** fatal error - CreateFileMapping S-1-5-21-1.1, Win32 error 5. Terminating.",
		"cygheap_user::init: NtSetInformationToken (TokenDefaultDacl), 0xC0000022",
	} {
		if WindowsRuntimeDiagnostic(text) == "" {
			t.Fatalf("missing diagnostic for %q", text)
		}
	}
}
