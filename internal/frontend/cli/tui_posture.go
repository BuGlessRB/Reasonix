package cli

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"reasonix/internal/contract/config"
	"reasonix/internal/session/control"
)

// errYoloNotConfirmed is the person answering no to the one-time YOLO notice.
var errYoloNotConfirmed = errors.New("YOLO was not confirmed; start again without --yolo, or answer y to enable it")

// postureHost is what settling a terminal session's posture reads and writes.
type postureHost interface {
	WritesConfined() bool
	WorkspaceRoot() string
	WorkspaceTrust() config.WorkspaceTrust
	SetWorkspaceTrust(config.WorkspaceTrust) error
	DefaultApprovalMode() string
	SetToolApprovalMode(string)
}

// settleTUIPosture puts an interactive session in the posture it opens in.
// A named YOLO needs the one-time confirmation; an unnamed posture is the
// default, after asking once whether to trust a folder whose writes the
// sandbox would confine.
func settleTUIPosture(ctrl postureHost, mode cliPermissionMode, named bool, home string, in *bufio.Scanner, out io.Writer) error {
	if named {
		if mode.approval == control.ToolApprovalYolo && !config.YoloAcknowledged(home) {
			if err := confirmYolo(home, in, out); err != nil {
				return err
			}
		}
		ctrl.SetToolApprovalMode(mode.approval)
		return nil
	}
	if ctrl.WritesConfined() && ctrl.WorkspaceTrust() == config.WorkspaceTrustUndecided && trustPromptable(ctrl.WorkspaceRoot()) {
		askWorkspaceTrust(ctrl, in, out)
	}
	ctrl.SetToolApprovalMode(ctrl.DefaultApprovalMode())
	return nil
}

func confirmYolo(home string, in *bufio.Scanner, out io.Writer) error {
	fmt.Fprintln(out, "YOLO runs file edits and shell commands without asking first.")
	fmt.Fprintln(out, "The sandbox, network policy and deny rules still apply. This is asked once.")
	if !strings.EqualFold(ask(in, out, "Enable YOLO?", "y/N"), "y") {
		return errYoloNotConfirmed
	}
	if err := config.AcknowledgeYolo(home); err != nil {
		fmt.Fprintln(out, "warning: could not record the confirmation:", err)
	}
	return nil
}

// askWorkspaceTrust records the answer either way: a folder declined once is
// not asked about again, and `reasonix trust` changes it later.
func askWorkspaceTrust(ctrl postureHost, in *bufio.Scanner, out io.Writer) {
	fmt.Fprintf(out, "Reasonix can edit files in %s without asking each time:\n", ctrl.WorkspaceRoot())
	fmt.Fprintln(out, "the OS sandbox keeps shell writes inside this folder, and deny rules still apply.")
	trust := config.WorkspaceTrustDeclined
	if strings.EqualFold(ask(in, out, "Trust this folder? (`reasonix trust --revoke` undoes it)", "y/N"), "y") {
		trust = config.WorkspaceTrusted
	}
	if err := ctrl.SetWorkspaceTrust(trust); err != nil {
		fmt.Fprintln(out, "warning: could not record the answer:", err)
	}
}

// trustPromptable leaves out the folders nobody should grant wholesale: a
// home directory or a filesystem root holds far more than one project.
func trustPromptable(root string) bool {
	root = filepath.Clean(strings.TrimSpace(root))
	if root == "." || root == "" || filepath.Dir(root) == root {
		return false
	}
	if home, err := os.UserHomeDir(); err == nil && sameDir(home, root) {
		return false
	}
	return true
}

func sameDir(a, b string) bool {
	if r, err := filepath.EvalSymlinks(a); err == nil {
		a = r
	}
	if r, err := filepath.EvalSymlinks(b); err == nil {
		b = r
	}
	a, b = filepath.Clean(a), filepath.Clean(b)
	if runtime.GOOS == "windows" || runtime.GOOS == "darwin" {
		return strings.EqualFold(a, b)
	}
	return a == b
}
