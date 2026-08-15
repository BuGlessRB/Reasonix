// filedrop.go — dropped files, as paths rather than bytes.
package main

import "github.com/wailsapp/wails/v2/pkg/options"

// dragAndDrop asks the native side to report dropped files' absolute paths: a
// webview hands JavaScript a File and never a path, which is the one thing an
// installer needs. Nothing subscribes here — the page takes them through the
// Wails runtime, the only subscription that filters by drop target. The two
// tests next door say what each half of that is load-bearing for.
func dragAndDrop() *options.DragAndDrop {
	return &options.DragAndDrop{EnableFileDrop: true}
}
