package serve

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"reasonix/internal/base/filelock"
	"reasonix/internal/base/fileutil"
	"reasonix/internal/contract/config"
)

type workspaceList struct {
	Paths  []string `json:"paths"`
	Launch string   `json:"launch,omitempty"`
}

var errWorkspaceNotRemembered = errors.New("workspace is not remembered")
var errWorkspaceListFull = errors.New("the project list already contains 32 projects; remove one before adding another")

func workspacesPath() string {
	if dir := config.MemoryUserDir(); dir != "" {
		return filepath.Join(dir, "serve-workspaces.json")
	}
	return ""
}

// Workspaces is the sidebar's remembered project order.
func Workspaces() []string {
	list, _ := readWorkspaceList(workspacesPath())
	return list.Paths
}

// LaunchWorkspaces puts the launch project first without changing sidebar order.
func LaunchWorkspaces() []string {
	list, _ := readWorkspaceList(workspacesPath())
	if list.Launch == "" {
		return list.Paths
	}
	paths := []string{list.Launch}
	for _, dir := range list.Paths {
		if dir != list.Launch {
			paths = append(paths, dir)
		}
	}
	return paths
}

func readWorkspaceList(path string) (workspaceList, error) {
	var list workspaceList
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) || path == "" {
		return list, nil
	}
	if err != nil {
		return list, err
	}
	data = bytes.TrimSpace(data)
	if len(data) > 0 && data[0] == '[' {
		err = json.Unmarshal(data, &list.Paths)
	} else {
		err = json.Unmarshal(data, &list)
	}
	if err != nil {
		return workspaceList{}, err
	}
	paths := make([]string, 0, len(list.Paths))
	for _, dir := range list.Paths {
		dir = strings.TrimSpace(dir)
		if dir != "" && !slices.Contains(paths, dir) {
			paths = append(paths, dir)
		}
	}
	list.Paths = paths
	if !slices.Contains(paths, list.Launch) {
		list.Launch = ""
		if len(paths) > 0 {
			list.Launch = paths[0]
		}
	}
	return list, nil
}

func updateWorkspaceList(ctx context.Context, repair bool, mutate func(*workspaceList) error) error {
	path := workspacesPath()
	if path == "" {
		return errors.New("no workspace list directory")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	unlock, err := filelock.Acquire(ctx, path+".lock")
	if err != nil {
		return err
	}
	defer unlock()
	list, err := readWorkspaceList(path)
	if err != nil {
		var syntax *json.SyntaxError
		var shape *json.UnmarshalTypeError
		if !repair || (!errors.As(err, &syntax) && !errors.As(err, &shape)) {
			return err
		}
		if err := backupWorkspaceList(path); err != nil {
			return err
		}
		list = workspaceList{}
	}
	if err := mutate(&list); err != nil {
		return err
	}
	data, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return err
	}
	return fileutil.AtomicWriteFile(path, data, 0o644)
}

func backupWorkspaceList(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".bak-*")
	if err != nil {
		return err
	}
	_, writeErr := f.Write(data)
	return errors.Join(writeErr, f.Close())
}

func rememberWorkspace(dir string) {
	if err := addRememberedWorkspace(context.Background(), dir); err != nil {
		slog.Warn("serve: remember workspace", "err", err)
	}
}

func addRememberedWorkspace(ctx context.Context, dir string) error {
	if dir == "" {
		return nil
	}
	return updateWorkspaceList(ctx, true, func(list *workspaceList) error {
		if slices.Contains(list.Paths, dir) {
			return nil
		}
		if len(list.Paths) >= workspaceRecentMax {
			return refusal(http.StatusConflict, "workspace.limit_reached", errWorkspaceListFull, nil)
		}
		list.Paths = append([]string{dir}, list.Paths...)
		list.Launch = dir
		return nil
	})
}

func forgetWorkspace(dir string) {
	if dir == "" {
		return
	}
	err := updateWorkspaceList(context.Background(), true, func(list *workspaceList) error {
		list.Paths = slices.DeleteFunc(list.Paths, func(path string) bool { return path == dir })
		if list.Launch == dir {
			list.Launch = ""
			if len(list.Paths) > 0 {
				list.Launch = list.Paths[0]
			}
		}
		return nil
	})
	if err != nil {
		slog.Warn("serve: forget workspace", "err", err)
	}
}

func moveWorkspace(ctx context.Context, dir string, direction int) error {
	return updateWorkspaceList(ctx, false, func(list *workspaceList) error {
		at := slices.Index(list.Paths, dir)
		if at < 0 {
			return errWorkspaceNotRemembered
		}
		to := at + direction
		if to >= 0 && to < len(list.Paths) {
			list.Paths[at], list.Paths[to] = list.Paths[to], list.Paths[at]
		}
		return nil
	})
}
