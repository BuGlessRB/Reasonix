// single_instance.go — one Studio window per data home.
package main

import (
	"os"

	"github.com/wailsapp/wails/v2/pkg/options"

	"reasonix/internal/instanceid"
)

// singleInstanceLock routes a second launch back to the window already up.
// Without it the second process opens its own, and two windows over one data
// home are two writers for every session file the sidebar lists.
func singleInstanceLock(shell *App) *options.SingleInstanceLock {
	// A dev build has to be able to run beside an installed one.
	if os.Getenv("REASONIX_DEV") != "" {
		return nil
	}
	return &options.SingleInstanceLock{
		UniqueId:               instanceid.Current(),
		OnSecondInstanceLaunch: func(options.SecondInstanceData) { shell.showWindow() },
	}
}
