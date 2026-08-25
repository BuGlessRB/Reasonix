//go:build !linux && !darwin

package main

import (
	"errors"
	"os"
)

// Windows resolves the handle its crash writer uses once, at start, so pointing
// os.Stderr somewhere else afterwards moves the window's own logs and not the
// trace. Saying so is better than a redirect that looks like it worked.
const canRedirectStderr = false

var errNoStderrRedirect = errors.New("stderr redirect is not available on this platform")

func redirectStderr(*os.File) error { return errNoStderrRedirect }

func dupStderr() (*os.File, error) { return nil, errNoStderrRedirect }
