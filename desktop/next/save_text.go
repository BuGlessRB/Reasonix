//go:build desktop

package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// SaveText writes text the window produced to a file the user picks. A webview
// starts no downloads of its own, so anything assembled in the frontend — the
// trajectory is the first — needs the shell to put it on disk. Empty path means
// the dialog was dismissed, which is an answer, not a failure.
func (a *App) SaveText(name, content string) (string, error) {
	if a.ctx == nil {
		return "", nil
	}
	if strings.TrimSpace(content) == "" {
		return "", fmt.Errorf("nothing to save")
	}
	path, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
		Title:           "保存",
		DefaultFilename: name,
	})
	if err != nil || strings.TrimSpace(path) == "" {
		return "", err
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return "", err
	}
	return path, nil
}
