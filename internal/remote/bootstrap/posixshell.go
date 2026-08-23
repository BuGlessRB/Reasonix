package bootstrap

import (
	"context"

	"reasonix/internal/remote/sftpfs"
)

// posixShell speaks to Linux and macOS remotes. The commands themselves stay
// where they were: they are the contract this package has always had, and the
// exported forms are what its tests hold it to.
type posixShell struct{}

func (posixShell) Executable() string { return "reasonix" }

func (posixShell) Home(ctx context.Context, _ Conn, fs *sftpfs.FS) (string, error) {
	return fs.RealPath(ctx, "~")
}

func (posixShell) Paths(home, workspace string) StatePaths {
	return pathsFor(home, workspace)
}

func (posixShell) Launch(bin, workspace string, p StatePaths) string {
	return LaunchCommand(bin, workspace, p)
}

func (posixShell) Alive(pid int, p StatePaths) string {
	return ServeAliveCommand(pid, p)
}

func (posixShell) Stop(pid int, p StatePaths) string {
	return StopCommand(pid, p)
}

func (posixShell) Logs(logFile string, n int) string {
	return LogsCommand(logFile, n)
}

func (posixShell) Locate(uploadedBin string) string {
	return LocateCommand(uploadedBin)
}
