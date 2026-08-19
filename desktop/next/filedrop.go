// filedrop.go — dropped files, as paths rather than bytes.
package main

import "github.com/wailsapp/wails/v2/pkg/options"

// dragAndDrop asks the native side to report dropped files' absolute paths: a
// webview hands JavaScript a File and never a path, and the path is what lets a
// turn work on the file itself rather than on a copy. Nothing subscribes here —
// only the page can say which element a drop landed on.
func dragAndDrop() *options.DragAndDrop {
	return &options.DragAndDrop{EnableFileDrop: true}
}
