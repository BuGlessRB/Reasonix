package main

import (
	"runtime"

	"reasonix/desktop/internal/update"
)

// studioLine is Studio's identity to the shared update path. Every system path
// here is Studio's own: two lines installed side by side must share no file, or
// dpkg would see one package overwriting the other's helper.
func studioLine() update.Line {
	return update.Line{
		Members:   []update.ReleaseMember{{Archive: binName(), Installed: binName(), Mode: 0o700}},
		Launchers: []string{binName()},
		Deb: update.DebLine{
			Package:      "reasonix-studio",
			HelperPath:   "/usr/lib/reasonix-studio/reasonix-studio-update-helper",
			PolkitAction: "io.reasonix.studio.update",
		},
	}
}

func binName() string {
	if runtime.GOOS == "windows" {
		return "reasonix-studio.exe"
	}
	return "reasonix-studio"
}
