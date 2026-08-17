package main

import (
	"runtime"

	"reasonix/desktop/internal/update"
	"reasonix/internal/installlayout"
)

// launcherCandidates are the entry points that survive this build's executable
// being replaced, most specific first. Guard trails the thin launcher as the
// one-release migration fallback for flat 1.18-1.19.1 installs.
func launcherCandidates() []string {
	if runtime.GOOS == "windows" {
		return []string{"reasonix-launcher.exe", "Reasonix.exe", "reasonix-guard.exe"}
	}
	return []string{"reasonix-launcher", "reasonix-guard"}
}

// desktopLine is this host's identity to the shared update path: which files a
// release archive carries, what they install as, and how dpkg and Polkit name
// this install. reasonix-guard has no installed name — v1.20+ stopped
// persisting it, but archives still carry it for updaters that read them.
func desktopLine() update.Line {
	return update.Line{
		Members: []update.ReleaseMember{
			{Archive: "reasonix-desktop", Installed: installlayout.DesktopBinaryName(), Mode: 0o700},
			{Archive: "reasonix", Installed: installlayout.CLIBinaryName(), Mode: 0o700},
			{Archive: "reasonix-guard"},
		},
		Launchers: launcherCandidates(),
		Deb: update.DebLine{
			Package:      linuxDebPackageName,
			HelperPath:   linuxUpdateHelperPath,
			PolkitAction: "io.reasonix.desktop.update",
		},
	}
}
