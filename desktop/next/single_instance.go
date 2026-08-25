// single_instance.go — one Studio window per data home.
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"

	"github.com/wailsapp/wails/v2/pkg/options"

	"reasonix/internal/agent"
	"reasonix/internal/config"
)

// The bundle's own identifier, so a Studio launch never answers the 1.x
// desktop's lock: two applications, two windows.
const singleInstanceIDPrefix = "io.reasonix.studio"

// singleInstanceLock routes a second launch back to the window already up.
// Without it the second process opens its own, and two windows over one data
// home are two writers for every session file the sidebar lists.
func singleInstanceLock(shell *App) *options.SingleInstanceLock {
	// A dev build has to be able to run beside an installed one.
	if os.Getenv("REASONIX_DEV") != "" {
		return nil
	}
	return &options.SingleInstanceLock{
		UniqueId:               singleInstanceID(),
		OnSecondInstanceLaunch: func(options.SecondInstanceData) { shell.showWindow() },
	}
}

// singleInstanceID names the data home rather than the executable: an installed
// build, a portable one and a canary all write the same sessions, so they have
// to find each other. An explicit REASONIX_HOME is still its own instance,
// which is what makes a second home the way to run two.
func singleInstanceID() string {
	root := strings.TrimSpace(config.ReasonixHomeDir())
	if root == "" {
		return singleInstanceIDPrefix
	}
	// The lease path canonicalizer, so a home below a symlink or a junction —
	// or one that does not exist yet — still hashes to the physical directory.
	if marker := agent.CanonicalSessionPath(filepath.Join(root, ".reasonix-home.identity")); marker != "" {
		root = filepath.Dir(marker)
	}
	sum := sha256.Sum256([]byte(filepath.Clean(root)))
	return singleInstanceIDPrefix + "." + hex.EncodeToString(sum[:8])
}
