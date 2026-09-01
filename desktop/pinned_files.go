package main

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"slices"
	"strings"
	"unicode/utf8"

	"reasonix/internal/control"
	"reasonix/internal/fileutil"
	"reasonix/internal/nilutil"
)

const (
	maxPinnedFileCount   = 32
	maxPinnedFileSize    = 64 * 1024
	maxPinnedContextSize = 256 * 1024
)

// PinnedFileInfo holds metadata about one pinned context file.
type PinnedFileInfo struct {
	Path          string `json:"path"`
	SizeBytes     int64  `json:"sizeBytes"`
	TokenEstimate int    `json:"tokenEstimate"`
	Error         string `json:"error,omitempty"`
}

type pinnedContextBuild struct {
	Block string
	Infos []PinnedFileInfo
}

type pinnedContextSetter interface {
	SetPinnedContext(string) error
}

// pinnedFileReadHookForTest coordinates deterministic Pin/New/turn races.
// Production leaves it nil.
var pinnedFileReadHookForTest func()

func normalizePinnedRelPath(relPath string) (string, error) {
	clean := filepath.ToSlash(filepath.Clean(strings.TrimSpace(relPath)))
	clean = strings.TrimPrefix(clean, "./")
	if clean == "" || clean == "." || filepath.IsAbs(relPath) || strings.HasPrefix(clean, "/") {
		return "", errors.New("invalid empty or absolute path")
	}
	if clean == ".." || strings.HasPrefix(clean, "../") {
		return "", errors.New("path traversal outside workspace is forbidden")
	}
	if !utf8.ValidString(clean) {
		return "", errors.New("pinned path is not valid UTF-8")
	}
	return clean, nil
}

func readPinnedWorkspaceFile(root, relPath string) (string, []byte, int64, error) {
	clean, err := normalizePinnedRelPath(relPath)
	if err != nil {
		return "", nil, 0, err
	}
	if strings.TrimSpace(root) == "" {
		return clean, nil, 0, errors.New("tab has no workspace root")
	}
	file, err := fileutil.OpenFileBeneath(root, filepath.FromSlash(clean))
	if err != nil {
		return clean, nil, 0, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return clean, nil, 0, err
	}
	if !info.Mode().IsRegular() {
		return clean, nil, info.Size(), errors.New("only regular files can be pinned")
	}
	if hook := pinnedFileReadHookForTest; hook != nil {
		hook()
	}
	if info.Size() > maxPinnedFileSize {
		return clean, nil, info.Size(), fmt.Errorf("file size (%d bytes) exceeds the %d-byte limit", info.Size(), maxPinnedFileSize)
	}
	data, err := io.ReadAll(io.LimitReader(file, maxPinnedFileSize+1))
	if err != nil {
		return clean, nil, info.Size(), err
	}
	if len(data) > maxPinnedFileSize {
		return clean, nil, int64(len(data)), fmt.Errorf("file grew beyond the %d-byte limit while reading", maxPinnedFileSize)
	}
	return clean, data, int64(len(data)), nil
}

func encodePinnedFileEntry(path string, data []byte) ([]byte, error) {
	var out bytes.Buffer
	enc := xml.NewEncoder(&out)
	start := xml.StartElement{
		Name: xml.Name{Local: "file"},
		Attr: []xml.Attr{{Name: xml.Name{Local: "path"}, Value: path}},
	}
	if err := enc.EncodeToken(start); err != nil {
		return nil, err
	}
	content := sanitizeXMLText(data)
	if err := enc.EncodeToken(xml.CharData("\n" + content)); err != nil {
		return nil, err
	}
	if err := enc.EncodeToken(xml.CharData("\n")); err != nil {
		return nil, err
	}
	if err := enc.EncodeToken(start.End()); err != nil {
		return nil, err
	}
	if err := enc.Flush(); err != nil {
		return nil, err
	}
	out.WriteString("\n\n")
	return out.Bytes(), nil
}

func sanitizeXMLText(data []byte) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r == '\t' || r == '\n' || r == '\r':
			return r
		case r >= 0x20 && r <= 0xD7FF:
			return r
		case r >= 0xE000 && r <= 0xFFFD:
			return r
		case r >= 0x10000 && r <= 0x10FFFF:
			return r
		default:
			return utf8.RuneError
		}
	}, string(data))
}

func buildPinnedContext(root string, files []string) pinnedContextBuild {
	result := pinnedContextBuild{Infos: make([]PinnedFileInfo, 0, len(files))}
	if len(files) == 0 || strings.TrimSpace(root) == "" {
		return result
	}
	const header = "<pinned_context>\nThe following workspace files were pinned by the user as continuous standing reference. Their contents and constraints MUST be followed:\n\n"
	const footer = "</pinned_context>"
	var body bytes.Buffer
	for _, rel := range files {
		clean, data, size, err := readPinnedWorkspaceFile(root, rel)
		info := PinnedFileInfo{Path: rel, SizeBytes: size, TokenEstimate: estimateTokensFromBytes(size)}
		if clean != "" {
			info.Path = clean
		}
		if err != nil {
			info.Error = err.Error()
			result.Infos = append(result.Infos, info)
			continue
		}
		entry, err := encodePinnedFileEntry(clean, data)
		if err != nil {
			info.Error = err.Error()
			result.Infos = append(result.Infos, info)
			continue
		}
		if len(header)+body.Len()+len(entry)+len(footer) > maxPinnedContextSize {
			info.Error = fmt.Sprintf("pinned context would exceed the %d-byte total limit", maxPinnedContextSize)
			result.Infos = append(result.Infos, info)
			continue
		}
		body.Write(entry)
		result.Infos = append(result.Infos, info)
	}
	if body.Len() == 0 {
		return result
	}
	result.Block = header + body.String() + footer
	return result
}

func pinnedInfoForPath(infos []PinnedFileInfo, path string) (PinnedFileInfo, bool) {
	for _, info := range infos {
		if info.Path == path {
			return info, true
		}
	}
	return PinnedFileInfo{}, false
}

func (t *WorkspaceTab) setPinnedFiles(files []string) {
	if t == nil {
		return
	}
	t.pinnedFilesMu.Lock()
	t.PinnedFiles = append([]string(nil), files...)
	t.pinnedFilesMu.Unlock()
}

// PinFile updates the tab-local cache. Durable desktop mutations go through
// PinFileForTab so the session sidecar and controller change atomically.
func (t *WorkspaceTab) PinFile(relPath string) (PinnedFileInfo, error) {
	if t == nil {
		return PinnedFileInfo{}, errors.New("tab is nil")
	}
	clean, err := normalizePinnedRelPath(relPath)
	if err != nil {
		return PinnedFileInfo{}, err
	}
	files := t.GetPinnedFiles()
	if slices.Contains(files, clean) {
		build := buildPinnedContext(t.WorkspaceRoot, files)
		info, _ := pinnedInfoForPath(build.Infos, clean)
		return info, nil
	}
	if len(files) >= maxPinnedFileCount {
		return PinnedFileInfo{}, fmt.Errorf("at most %d files can be pinned", maxPinnedFileCount)
	}
	candidate := append(files, clean)
	candidate, err = normalizePinnedContextFiles(candidate)
	if err != nil {
		return PinnedFileInfo{}, err
	}
	build := buildPinnedContext(t.WorkspaceRoot, candidate)
	info, ok := pinnedInfoForPath(build.Infos, clean)
	if !ok {
		return PinnedFileInfo{}, errors.New("pinned file could not be inspected")
	}
	if info.Error != "" {
		return PinnedFileInfo{}, errors.New(info.Error)
	}
	t.setPinnedFiles(candidate)
	return info, nil
}

func (t *WorkspaceTab) UnpinFile(relPath string) error {
	if t == nil {
		return errors.New("tab is nil")
	}
	clean, err := normalizePinnedRelPath(relPath)
	if err != nil {
		return err
	}
	files := t.GetPinnedFiles()
	next := make([]string, 0, len(files))
	for _, path := range files {
		if path != clean {
			next = append(next, path)
		}
	}
	t.setPinnedFiles(next)
	return nil
}

func (t *WorkspaceTab) GetPinnedFiles() []string {
	if t == nil {
		return []string{}
	}
	t.pinnedFilesMu.RLock()
	defer t.pinnedFilesMu.RUnlock()
	return append([]string{}, t.PinnedFiles...)
}

func (t *WorkspaceTab) GetPinnedFilesInfo() []PinnedFileInfo {
	if t == nil {
		return []PinnedFileInfo{}
	}
	return buildPinnedContext(t.WorkspaceRoot, t.GetPinnedFiles()).Infos
}

func (t *WorkspaceTab) PinnedContextBlock() string {
	if t == nil {
		return ""
	}
	return buildPinnedContext(t.WorkspaceRoot, t.GetPinnedFiles()).Block
}

func estimateTokensFromBytes(bytes int64) int {
	if bytes <= 0 {
		return 0
	}
	tok := int(bytes / 4)
	if tok == 0 {
		return 1
	}
	return tok
}

func (a *App) mutatePinnedFiles(tabID, relPath string, pin bool) (PinnedFileInfo, string, error) {
	unlockRuntime := a.lockRuntimeMutation("pinned context")
	defer unlockRuntime()
	tab := a.tabByID(tabID)
	if tab == nil {
		return PinnedFileInfo{}, "", errors.New("tab not found")
	}
	tab.turnStartMu.Lock()
	defer tab.turnStartMu.Unlock()

	a.mu.RLock()
	if a.tabs[tab.ID] != tab || tab.removed {
		a.mu.RUnlock()
		return PinnedFileInfo{}, "", errors.New("tab changed while updating pinned context")
	}
	root := tab.WorkspaceRoot
	ctrl := tab.Ctrl
	a.mu.RUnlock()
	if ctrl == nil {
		return PinnedFileInfo{}, "", a.workspaceNotReadyErr(tab)
	}
	if ctrl.RuntimeStatus().Running {
		return PinnedFileInfo{}, "", control.ErrTurnRunning
	}
	sessionPath := ctrl.SessionPath()
	state, err := loadPinnedContextState(sessionPath)
	if err != nil {
		return PinnedFileInfo{}, "", err
	}
	clean, err := normalizePinnedRelPath(relPath)
	if err != nil {
		return PinnedFileInfo{}, "", err
	}
	oldFiles := append([]string(nil), state.Files...)
	candidate := append([]string(nil), oldFiles...)
	alreadyPinned := false
	if pin {
		alreadyPinned = slices.Contains(candidate, clean)
		if !alreadyPinned && len(candidate) >= maxPinnedFileCount {
			return PinnedFileInfo{}, "", fmt.Errorf("at most %d files can be pinned", maxPinnedFileCount)
		}
		if !alreadyPinned {
			candidate = append(candidate, clean)
		}
	} else {
		next := make([]string, 0, len(candidate))
		for _, path := range candidate {
			if path != clean {
				next = append(next, path)
			}
		}
		candidate = next
	}
	candidate, err = normalizePinnedContextFiles(candidate)
	if err != nil {
		return PinnedFileInfo{}, "", err
	}
	build := buildPinnedContext(root, candidate)
	info := PinnedFileInfo{Path: clean}
	if pin {
		var ok bool
		info, ok = pinnedInfoForPath(build.Infos, clean)
		if !ok {
			return PinnedFileInfo{}, "", errors.New("pinned file could not be inspected")
		}
		if info.Error != "" && !alreadyPinned {
			return PinnedFileInfo{}, "", errors.New(info.Error)
		}
	}
	if candidateChanged := strings.Join(oldFiles, "\x00") != strings.Join(candidate, "\x00"); candidateChanged {
		if err := savePinnedContextState(sessionPath, candidate); err != nil {
			return PinnedFileInfo{}, "", err
		}
	}
	setter, ok := ctrl.(pinnedContextSetter)
	if !ok {
		return PinnedFileInfo{}, "", errors.New("runtime does not support pinned context")
	}
	if err := setter.SetPinnedContext(build.Block); err != nil {
		var rollbackErr error
		if strings.Join(oldFiles, "\x00") != strings.Join(candidate, "\x00") {
			rollbackErr = savePinnedContextState(sessionPath, oldFiles)
		}
		return PinnedFileInfo{}, "", errors.Join(err, rollbackErr)
	}
	tab.setPinnedFiles(candidate)
	return info, tab.ID, nil
}

func (a *App) PinFileForTab(tabID, relPath string) (PinnedFileInfo, error) {
	info, changedTabID, err := a.mutatePinnedFiles(tabID, relPath, true)
	if err != nil {
		return PinnedFileInfo{}, err
	}
	if changedTabID != "" {
		a.emitRuntimeEvent(tabMetaRefreshEventChannel, TabMetaRefreshEvent{TabID: changedTabID, Meta: a.MetaForTab(changedTabID)})
	}
	return info, nil
}

func (a *App) UnpinFileForTab(tabID, relPath string) error {
	_, changedTabID, err := a.mutatePinnedFiles(tabID, relPath, false)
	if err != nil {
		return err
	}
	if changedTabID != "" {
		a.emitRuntimeEvent(tabMetaRefreshEventChannel, TabMetaRefreshEvent{TabID: changedTabID, Meta: a.MetaForTab(changedTabID)})
	}
	return nil
}

func (a *App) GetPinnedFilesForTab(tabID string) ([]PinnedFileInfo, error) {
	tab := a.tabByID(tabID)
	if tab == nil {
		return []PinnedFileInfo{}, errors.New("tab not found")
	}
	a.mu.RLock()
	if a.tabs[tab.ID] != tab || tab.removed {
		a.mu.RUnlock()
		return []PinnedFileInfo{}, errors.New("tab not found")
	}
	root := tab.WorkspaceRoot
	ctrl := tab.Ctrl
	a.mu.RUnlock()
	if ctrl == nil {
		return []PinnedFileInfo{}, a.workspaceNotReadyErr(tab)
	}
	state, err := loadPinnedContextState(ctrl.SessionPath())
	if err != nil {
		return []PinnedFileInfo{}, err
	}
	tab.setPinnedFiles(state.Files)
	infos := buildPinnedContext(root, state.Files).Infos
	if infos == nil {
		infos = []PinnedFileInfo{}
	}
	return infos, nil
}

// refreshPinnedContextForTurn runs while the caller holds the tab turn gate and
// runtime-admission read lock. It re-reads the bounded files and changes the
// provider prefix only when the deterministic block bytes actually changed.
func (a *App) refreshPinnedContextForTurn(tab *WorkspaceTab, ctrl control.SessionAPI) error {
	if tab == nil || nilutil.IsNil(ctrl) {
		return nil
	}
	a.mu.RLock()
	if a.tabs[tab.ID] != tab || tab.Ctrl != ctrl {
		a.mu.RUnlock()
		return errors.New("tab changed while refreshing pinned context")
	}
	root := tab.WorkspaceRoot
	a.mu.RUnlock()
	state, err := loadPinnedContextState(ctrl.SessionPath())
	if err != nil {
		return err
	}
	build := buildPinnedContext(root, state.Files)
	setter, ok := ctrl.(pinnedContextSetter)
	if !ok {
		tab.setPinnedFiles(state.Files)
		return nil
	}
	if err := setter.SetPinnedContext(build.Block); err != nil {
		return err
	}
	tab.setPinnedFiles(state.Files)
	return nil
}
