//go:build !desktop

package main

import "context"

// fitWindow needs the Wails runtime that only the desktop tag links in. The
// no-op keeps the package buildable and vettable without it: a Wails app no
// tool can analyse is one where a type error waits until release day.
func fitWindow(context.Context) {}
