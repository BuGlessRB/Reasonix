//go:build windows

package update

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/windows"
)

func TestRunInstallerStartErrorIsActionableAndPathSafe(t *testing.T) {
	for _, tc := range []struct {
		name string
		raw  error
		want string
	}{
		{"declined", windows.ERROR_CANCELLED, "administrator prompt"},
		{"quarantined", windows.ERROR_FILE_NOT_FOUND, "quarantined"},
		{"denied", windows.ERROR_ACCESS_DENIED, "denied"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw := &os.PathError{Op: "shellexecute", Path: `C:\Users\Private\ReasonixStudio-installer.exe`, Err: tc.raw}
			err := runInstallerStartError(raw)
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error is not actionable: %v", err)
			}
			if strings.Contains(err.Error(), `C:\Users`) {
				t.Fatalf("error leaks local path: %v", err)
			}
		})
	}
}

// Elevation is the failure this path exists to avoid: CreateProcess reports it
// and ShellExecute does not, so a message naming it means the fix regressed.
func TestRunInstallerStartErrorNamesUnexpectedElevation(t *testing.T) {
	err := runInstallerStartError(windows.ERROR_ELEVATION_REQUIRED)
	if !errors.Is(err, windows.ERROR_ELEVATION_REQUIRED) {
		t.Fatalf("elevation failure lost its cause: %v", err)
	}
}

// A line that never said its installer needs admin must not reach a launch
// call chosen for it: that guess is what shipped the elevation failure.
func TestRunInstallerRequiresAnElevationDeclaration(t *testing.T) {
	err := (Line{}).RunInstaller(filepath.Join(t.TempDir(), "absent.exe"))
	if err == nil {
		t.Fatal("an undeclared line was allowed to start an installer")
	}
	if !strings.Contains(err.Error(), "declares no elevated installer") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunInstallerRejectsMissingInstaller(t *testing.T) {
	elevated := Line{Windows: WindowsLine{Elevated: true}}
	if err := elevated.RunInstaller(""); err == nil {
		t.Fatal("empty installer path was accepted")
	}
	missing := filepath.Join(t.TempDir(), "absent.exe")
	err := elevated.RunInstaller(missing)
	if err == nil {
		t.Fatal("missing installer was accepted")
	}
	if !strings.Contains(err.Error(), "unreadable") {
		t.Fatalf("unexpected error: %v", err)
	}
}
