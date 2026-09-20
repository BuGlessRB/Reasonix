//go:build !windows

package sandbox

import "os/exec"

func PrepareRunnerDiagnostics(_ *exec.Cmd) (func(error) error, error) {
	return func(err error) error { return err }, nil
}
