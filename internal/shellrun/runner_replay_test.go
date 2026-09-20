package shellrun

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"reasonix/internal/proc"
	"reasonix/internal/sandbox"
	"reasonix/internal/tool"
)

func TestReplayedRunnerMarkerCannotEraseExecution(t *testing.T) {
	payload := "same-payload-for-same-sandbox-settings"
	marker := fmt.Sprintf("__reasonix_windows_sandbox_failure__:%x:authorization", sha256.Sum256([]byte(payload)))
	mutated := filepath.Join(t.TempDir(), "mutated")
	result := RunForeground(t.Context(), Request{
		Argv: []string{"reasonix", sandbox.WindowsHelperCommand, payload, "--", "pwsh"},
		Run: func(_ context.Context, cmd *exec.Cmd, _ proc.RunOptions) (*proc.TrackedCommand, error) {
			var child *exec.Cmd
			if runtime.GOOS == "windows" {
				child = exec.Command("cmd", "/c", `echo changed>"`+mutated+`" & echo `+marker+` & exit /b 126`)
			} else {
				child = exec.Command("sh", "-c", `touch "$1"; printf '%s\n' "$2"; exit 126`, "test", mutated, marker)
			}
			child.Stdout, child.Stderr = cmd.Stdout, cmd.Stderr
			err := child.Run()
			cmd.Process, cmd.ProcessState = child.Process, child.ProcessState
			return nil, err
		},
	})
	if _, err := os.Stat(mutated); err != nil {
		t.Fatal(err)
	}
	if !result.Started || result.FailurePhase != tool.ShellPhaseExecution || result.State != tool.ShellStateFailed {
		t.Fatalf("mutation misreported: %+v", result)
	}
}
