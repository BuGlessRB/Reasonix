//go:build darwin

package main

/*
// Wails' darwin frontend references UTType. `wails build` links this framework
// implicitly; a plain `go build` does not.
#cgo LDFLAGS: -framework UniformTypeIdentifiers
*/
import "C"
