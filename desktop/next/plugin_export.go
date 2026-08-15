// plugin_export.go — handing a packed plugin package to the user.
package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"reasonix/internal/config"
	"reasonix/internal/pluginpkg"
)

// PluginExport is what the window answers with: where the archive landed, and
// the variables its configuration no longer carries. An empty path is the
// user cancelling, which is not a failure.
type PluginExport struct {
	Path     string   `json:"path"`
	Required []string `json:"required"`
}

// SavePluginExport packs an installed package and writes it wherever the native
// dialog points. A WKWebView starts no downloads of its own, so the browser
// build's <a download> is not a path this window has — without this binding the
// export button would do nothing at all on macOS.
func (a *App) SavePluginExport(name string) (PluginExport, error) {
	if a.ctx == nil {
		return PluginExport{}, nil
	}
	home := config.ReasonixHomeDir()
	st, err := pluginpkg.LoadState(home)
	if err != nil {
		return PluginExport{}, err
	}
	root := ""
	for _, p := range st.Plugins {
		if p.Name == name {
			root = pluginpkg.ResolveRoot(home, p.Root)
			break
		}
	}
	if root == "" {
		return PluginExport{}, fmt.Errorf("plugin %q is not installed", name)
	}
	archive, required, err := pluginpkg.Export(name, root)
	if err != nil {
		return PluginExport{}, err
	}
	// Packed before the dialog opens: a package that cannot be packed should
	// say so instead of asking where to put a file it will never write.
	path, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
		Title:           "导出插件包",
		DefaultFilename: name + ".zip",
	})
	if err != nil {
		return PluginExport{}, err
	}
	if strings.TrimSpace(path) == "" {
		return PluginExport{Required: required}, nil
	}
	if err := os.WriteFile(path, archive, 0o644); err != nil {
		return PluginExport{}, err
	}
	return PluginExport{Path: path, Required: required}, nil
}
