//go:build windows

package sandbox

import (
	"crypto/rand"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// PrepareRunnerDiagnostics gives only the helper an inherited report handle.
// The temporary file has sharing disabled and is deleted on last close, so the
// restricted child cannot reopen it even if the temp directory is writable.
// A file rather than a pipe also makes reads bounded after cancellation: a
// stuck helper cannot hold the caller waiting for EOF. No path enters argv/env.
func PrepareRunnerDiagnostics(cmd *exec.Cmd) (func(error) error, error) {
	if len(cmd.Args) < 4 || cmd.Args[1] != WindowsHelperCommand || cmd.Args[3] != "--" {
		return func(err error) error { return err }, nil
	}
	path, err := windows.UTF16PtrFromString(filepath.Join(os.TempDir(), "reasonix-runner-"+rand.Text()))
	if err != nil {
		return nil, err
	}
	sa := &windows.SecurityAttributes{Length: uint32(unsafe.Sizeof(windows.SecurityAttributes{})), InheritHandle: 1}
	handle, err := windows.CreateFile(path, windows.GENERIC_READ|windows.GENERIC_WRITE, 0, sa, windows.CREATE_NEW, windows.FILE_ATTRIBUTE_TEMPORARY|windows.FILE_FLAG_DELETE_ON_CLOSE, 0)
	if err != nil {
		return nil, err
	}
	f := os.NewFile(uintptr(handle), "sandbox runner report")
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.AdditionalInheritedHandles = append(cmd.SysProcAttr.AdditionalInheritedHandles, syscall.Handle(handle))
	env := cmd.Env
	if env == nil {
		env = os.Environ()
	}
	cmd.Env = make([]string, 0, len(env)+1)
	for _, entry := range env {
		key, _, _ := strings.Cut(entry, "=")
		if !strings.EqualFold(key, runnerReportEnvironment) {
			cmd.Env = append(cmd.Env, entry)
		}
	}
	cmd.Env = append(cmd.Env, runnerReportEnvironment+"="+strconv.FormatUint(uint64(handle), 10))
	return func(cause error) error {
		defer f.Close()
		return readRunnerReport(io.NewSectionReader(f, 0, runnerReportLimit+1), cause)
	}, nil
}

func openRunnerReport() (*os.File, error) {
	value := os.Getenv(runnerReportEnvironment)
	_ = os.Unsetenv(runnerReportEnvironment)
	if value == "" {
		return nil, nil
	} // Standalone helper invocation.
	n, err := strconv.ParseUint(value, 10, 64)
	if err != nil || n == 0 {
		return nil, fmt.Errorf("invalid runner report handle")
	}
	handle := windows.Handle(n)
	// Clear inheritance before any child can start. winsandbox also passes an
	// explicit allowlist containing only duplicates of the three stdio handles.
	if err := windows.SetHandleInformation(handle, windows.HANDLE_FLAG_INHERIT, 0); err != nil {
		return nil, err
	}
	return os.NewFile(uintptr(handle), "sandbox runner report"), nil
}
