package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

const (
	maxPinnedFileSize  = 64 * 1024  // 64 KiB per pinned file
	maxTotalPinnedSize = 256 * 1024 // 256 KiB total pinned files
)

// PinnedFileInfo holds metadata about one pinned context file.
type PinnedFileInfo struct {
	Path          string `json:"path"`
	SizeBytes     int64  `json:"sizeBytes"`
	TokenEstimate int    `json:"tokenEstimate"`
	Error         string `json:"error,omitempty"`
}

func normalizePinnedRelPath(relPath string) (string, error) {
	clean := filepath.ToSlash(filepath.Clean(strings.TrimSpace(relPath)))
	clean = strings.TrimPrefix(clean, "./")
	clean = strings.TrimPrefix(clean, "/")
	if clean == "" || clean == "." {
		return "", errors.New("invalid empty path")
	}
	if clean == ".." || strings.HasPrefix(clean, "../") {
		return "", errors.New("path traversal outside workspace is forbidden")
	}
	return clean, nil
}

// validatePinnedFileInWorkspace validates that a relative file path belongs to the workspace
// and resolves its physical path (protecting against symlink escapes).
func validatePinnedFileInWorkspace(root, relPath string) (string, string, os.FileInfo, error) {
	clean, err := normalizePinnedRelPath(relPath)
	if err != nil {
		return "", "", nil, err
	}
	if root == "" {
		return "", "", nil, errors.New("tab has no workspace root")
	}
	absPath := filepath.Join(root, filepath.FromSlash(clean))

	// Resolve symlinks to verify the actual physical target
	realTarget, err := filepath.EvalSymlinks(absPath)
	if err != nil {
		return "", "", nil, fmt.Errorf("cannot resolve file path: %w", err)
	}

	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		realRoot = filepath.Clean(root)
	}

	relToRoot, err := filepath.Rel(realRoot, realTarget)
	if err != nil || relToRoot == ".." || strings.HasPrefix(relToRoot, ".."+string(filepath.Separator)) {
		return "", "", nil, errors.New("file resolves outside workspace root (symlink traversal forbidden)")
	}

	st, err := os.Stat(realTarget)
	if err != nil {
		return "", "", nil, err
	}
	if st.IsDir() {
		return "", "", nil, errors.New("directories cannot be pinned as context files")
	}
	return clean, realTarget, st, nil
}

// PinFile adds a workspace file to the tab's standing pinned context.
func (tab *WorkspaceTab) PinFile(relPath string) (PinnedFileInfo, error) {
	if tab == nil {
		return PinnedFileInfo{}, errors.New("tab is nil")
	}
	clean, _, st, err := validatePinnedFileInWorkspace(tab.WorkspaceRoot, relPath)
	if err != nil {
		return PinnedFileInfo{}, err
	}
	if st.Size() > maxPinnedFileSize {
		return PinnedFileInfo{}, fmt.Errorf("file size (%d bytes) exceeds maximum pinned file limit of %d bytes", st.Size(), maxPinnedFileSize)
	}

	tab.pinnedFilesMu.Lock()
	defer tab.pinnedFilesMu.Unlock()

	// Check if already pinned
	for _, p := range tab.PinnedFiles {
		if p == clean {
			tok := estimateTokensFromBytes(st.Size())
			return PinnedFileInfo{Path: clean, SizeBytes: st.Size(), TokenEstimate: tok}, nil
		}
	}

	// Calculate total size including existing files
	var totalSize int64
	for _, p := range tab.PinnedFiles {
		if _, _, pSt, err := validatePinnedFileInWorkspace(tab.WorkspaceRoot, p); err == nil && !pSt.IsDir() {
			totalSize += pSt.Size()
		}
	}
	if totalSize+st.Size() > maxTotalPinnedSize {
		return PinnedFileInfo{}, fmt.Errorf("total pinned size (%d bytes) would exceed maximum allowance of %d bytes", totalSize+st.Size(), maxTotalPinnedSize)
	}

	tab.PinnedFiles = append(tab.PinnedFiles, clean)
	tok := estimateTokensFromBytes(st.Size())
	return PinnedFileInfo{Path: clean, SizeBytes: st.Size(), TokenEstimate: tok}, nil
}

// UnpinFile removes a file from the tab's pinned context.
func (tab *WorkspaceTab) UnpinFile(relPath string) error {
	if tab == nil {
		return errors.New("tab is nil")
	}
	clean, err := normalizePinnedRelPath(relPath)
	if err != nil {
		return err
	}

	tab.pinnedFilesMu.Lock()
	defer tab.pinnedFilesMu.Unlock()

	next := make([]string, 0, len(tab.PinnedFiles))
	for _, p := range tab.PinnedFiles {
		if p != clean {
			next = append(next, p)
		}
	}
	tab.PinnedFiles = next
	return nil
}

// GetPinnedFiles returns a slice of currently pinned relative file paths.
func (tab *WorkspaceTab) GetPinnedFiles() []string {
	if tab == nil {
		return nil
	}
	tab.pinnedFilesMu.RLock()
	defer tab.pinnedFilesMu.RUnlock()
	return append([]string(nil), tab.PinnedFiles...)
}

// GetPinnedFilesInfo returns metadata for each pinned file.
func (tab *WorkspaceTab) GetPinnedFilesInfo() []PinnedFileInfo {
	if tab == nil {
		return nil
	}
	tab.pinnedFilesMu.RLock()
	pinned := append([]string(nil), tab.PinnedFiles...)
	root := tab.WorkspaceRoot
	tab.pinnedFilesMu.RUnlock()

	if len(pinned) == 0 {
		return nil
	}

	infos := make([]PinnedFileInfo, 0, len(pinned))
	for _, rel := range pinned {
		clean, _, st, err := validatePinnedFileInWorkspace(root, rel)
		if err != nil {
			infos = append(infos, PinnedFileInfo{Path: rel, Error: err.Error()})
			continue
		}
		tok := estimateTokensFromBytes(st.Size())
		infos = append(infos, PinnedFileInfo{
			Path:          clean,
			SizeBytes:     st.Size(),
			TokenEstimate: tok,
		})
	}
	return infos
}

// PinnedContextBlock generates the standing XML block to inject into the system prompt.
func (tab *WorkspaceTab) PinnedContextBlock() string {
	if tab == nil {
		return ""
	}
	tab.pinnedFilesMu.RLock()
	pinned := append([]string(nil), tab.PinnedFiles...)
	root := tab.WorkspaceRoot
	tab.pinnedFilesMu.RUnlock()

	if len(pinned) == 0 || root == "" {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("\n\n<pinned_context>\n")
	sb.WriteString("The following files are pinned by the user as continuous standing reference. Their contents and constraints MUST be strictly adhered to:\n\n")

	for _, rel := range pinned {
		clean, realTarget, _, err := validatePinnedFileInWorkspace(root, rel)
		if err != nil {
			sb.WriteString(fmt.Sprintf("<file path=%q error=%q />\n\n", rel, err.Error()))
			continue
		}
		data, err := os.ReadFile(realTarget)
		if err != nil {
			sb.WriteString(fmt.Sprintf("<file path=%q error=%q />\n\n", clean, err.Error()))
			continue
		}
		if len(data) > maxPinnedFileSize {
			data = data[:maxPinnedFileSize]
		}
		sb.WriteString(fmt.Sprintf("<file path=%q>\n%s\n</file>\n\n", clean, string(data)))
	}
	sb.WriteString("</pinned_context>")
	return sb.String()
}

func estimateTokensFromBytes(bytes int64) int {
	if bytes <= 0 {
		return 0
	}
	// Roughly ~3.5 chars per token for code/prose
	tok := int(bytes / 4)
	if tok == 0 {
		return 1
	}
	return tok
}

// EstimateTokensFromString computes rough token count for a string.
func estimateTokensFromString(s string) int {
	runes := utf8.RuneCountInString(s)
	if runes <= 0 {
		return 0
	}
	tok := runes / 3
	if tok == 0 {
		return 1
	}
	return tok
}

func updatePinnedContextInSystemPrompt(currentPrompt, newBlock string) string {
	startIdx := strings.Index(currentPrompt, "\n\n<pinned_context>")
	if startIdx < 0 {
		startIdx = strings.Index(currentPrompt, "<pinned_context>")
	}
	if startIdx >= 0 {
		endIdx := strings.Index(currentPrompt[startIdx:], "</pinned_context>")
		if endIdx >= 0 {
			endPos := startIdx + endIdx + len("</pinned_context>")
			base := strings.TrimRight(currentPrompt[:startIdx], "\n") + currentPrompt[endPos:]
			base = strings.TrimSpace(base)
			if strings.TrimSpace(newBlock) == "" {
				return base
			}
			return base + "\n\n" + strings.TrimSpace(newBlock)
		}
	}
	if strings.TrimSpace(newBlock) == "" {
		return currentPrompt
	}
	return strings.TrimRight(currentPrompt, "\n") + "\n\n" + strings.TrimSpace(newBlock)
}

func (a *App) syncTabPinnedContextLocked(tab *WorkspaceTab) {
	if tab == nil || tab.Ctrl == nil {
		return
	}
	ctrl := tab.Ctrl
	current := ctrl.SystemPrompt()
	newBlock := tab.PinnedContextBlock()
	updated := updatePinnedContextInSystemPrompt(current, newBlock)
	ctrl.UpdateSystemPrompt(updated)
}

// PinFileForTab pins a relative file path to the tab's standing context.
func (a *App) PinFileForTab(tabID string, relPath string) (PinnedFileInfo, error) {
	a.mu.Lock()
	tab := a.tabByIDLocked(tabID)
	if tab == nil {
		a.mu.Unlock()
		return PinnedFileInfo{}, errors.New("tab not found")
	}
	info, err := tab.PinFile(relPath)
	if err != nil {
		a.mu.Unlock()
		return PinnedFileInfo{}, err
	}
	a.saveTabsLocked()
	a.syncTabPinnedContextLocked(tab)
	a.mu.Unlock()

	a.emitRuntimeEvent(tabMetaRefreshEventChannel, TabMetaRefreshEvent{TabID: tab.ID, Meta: a.MetaForTab(tab.ID)})
	return info, nil
}

// UnpinFileForTab removes a pinned file from the tab's context.
func (a *App) UnpinFileForTab(tabID string, relPath string) error {
	a.mu.Lock()
	tab := a.tabByIDLocked(tabID)
	if tab == nil {
		a.mu.Unlock()
		return errors.New("tab not found")
	}
	if err := tab.UnpinFile(relPath); err != nil {
		a.mu.Unlock()
		return err
	}
	a.saveTabsLocked()
	a.syncTabPinnedContextLocked(tab)
	a.mu.Unlock()

	a.emitRuntimeEvent(tabMetaRefreshEventChannel, TabMetaRefreshEvent{TabID: tab.ID, Meta: a.MetaForTab(tab.ID)})
	return nil
}

// GetPinnedFilesForTab retrieves all pinned files for a tab.
func (a *App) GetPinnedFilesForTab(tabID string) ([]PinnedFileInfo, error) {
	a.mu.RLock()
	tab := a.tabByIDLocked(tabID)
	a.mu.RUnlock()
	if tab == nil {
		return nil, errors.New("tab not found")
	}
	return tab.GetPinnedFilesInfo(), nil
}
