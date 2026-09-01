package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"reasonix/internal/agent"
	"reasonix/internal/control"
	"reasonix/internal/event"
	"reasonix/internal/provider"
	"reasonix/internal/tool"
)

func TestTabPinFile(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "api_spec.md")
	content := "# API Specification\n\n- Endpoint: /v1/chat\n- Method: POST\n"
	if err := os.WriteFile(filePath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	tab := &WorkspaceTab{
		ID:            "tab-1",
		WorkspaceRoot: dir,
	}

	info, err := tab.PinFile("api_spec.md")
	if err != nil {
		t.Fatalf("unexpected error pinning file: %v", err)
	}
	if info.Path != "api_spec.md" {
		t.Fatalf("expected path api_spec.md, got %q", info.Path)
	}
	if info.SizeBytes != int64(len(content)) {
		t.Fatalf("expected size %d, got %d", len(content), info.SizeBytes)
	}
	if info.TokenEstimate <= 0 {
		t.Fatalf("expected positive token estimate, got %d", info.TokenEstimate)
	}

	// Test idempotency
	info2, err := tab.PinFile("api_spec.md")
	if err != nil {
		t.Fatalf("unexpected error on duplicate pin: %v", err)
	}
	if info2.Path != "api_spec.md" {
		t.Fatalf("expected path api_spec.md, got %q", info2.Path)
	}
	if len(tab.GetPinnedFiles()) != 1 {
		t.Fatalf("expected exactly 1 pinned file, got %d", len(tab.GetPinnedFiles()))
	}

	// Test non-existent file
	if _, err := tab.PinFile("missing.txt"); err == nil {
		t.Fatal("expected error for non-existent file, got nil")
	}

	// Test directory pin rejection
	subDir := filepath.Join(dir, "subdir")
	if err := os.Mkdir(subDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := tab.PinFile("subdir"); err == nil {
		t.Fatal("expected error for directory pin, got nil")
	}

	// Test path traversal rejection
	if _, err := tab.PinFile("../outside.txt"); err == nil {
		t.Fatal("expected error for path traversal, got nil")
	}
}

func TestTabPinSymlinkOutsideWorkspaceForbidden(t *testing.T) {
	wsDir := t.TempDir()
	outsideDir := t.TempDir()
	outsideFile := filepath.Join(outsideDir, "secret.key")
	if err := os.WriteFile(outsideFile, []byte("SUPER_SECRET"), 0o600); err != nil {
		t.Fatal(err)
	}

	symlinkPath := filepath.Join(wsDir, "escape_link.txt")
	if err := os.Symlink(outsideFile, symlinkPath); err != nil {
		t.Skipf("symlink creation not supported or permitted: %v", err)
	}

	tab := &WorkspaceTab{
		ID:            "tab-symlink",
		WorkspaceRoot: wsDir,
	}

	_, err := tab.PinFile("escape_link.txt")
	if err == nil {
		t.Fatal("expected error when pinning symlink pointing outside workspace, got nil")
	}
	if !strings.Contains(err.Error(), "symlink traversal forbidden") {
		t.Fatalf("expected symlink traversal error message, got: %v", err)
	}
}

func TestTabPinSymlinkInsideWorkspaceAllowed(t *testing.T) {
	wsDir := t.TempDir()
	realFile := filepath.Join(wsDir, "real_config.json")
	content := `{"allowed": true}`
	if err := os.WriteFile(realFile, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	symlinkPath := filepath.Join(wsDir, "config_link.json")
	if err := os.Symlink(realFile, symlinkPath); err != nil {
		t.Skipf("symlink creation not supported or permitted: %v", err)
	}

	tab := &WorkspaceTab{
		ID:            "tab-symlink-ok",
		WorkspaceRoot: wsDir,
	}

	info, err := tab.PinFile("config_link.json")
	if err != nil {
		t.Fatalf("expected pinning valid inside-workspace symlink to succeed, got: %v", err)
	}
	if info.Path != "config_link.json" {
		t.Fatalf("expected path config_link.json, got %q", info.Path)
	}
	if info.SizeBytes != int64(len(content)) {
		t.Fatalf("expected size %d, got %d", len(content), info.SizeBytes)
	}
}

func TestTabPinFileSizeLimit(t *testing.T) {
	dir := t.TempDir()
	bigFile := filepath.Join(dir, "big.dat")
	data := make([]byte, maxPinnedFileSize+1)
	if err := os.WriteFile(bigFile, data, 0o644); err != nil {
		t.Fatal(err)
	}

	tab := &WorkspaceTab{
		ID:            "tab-1",
		WorkspaceRoot: dir,
	}

	if _, err := tab.PinFile("big.dat"); err == nil {
		t.Fatal("expected error for file exceeding size limit, got nil")
	}
}

func TestTabUnpinFile(t *testing.T) {
	dir := t.TempDir()
	f1 := filepath.Join(dir, "f1.txt")
	f2 := filepath.Join(dir, "f2.txt")
	if err := os.WriteFile(f1, []byte("f1"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(f2, []byte("f2"), 0o644); err != nil {
		t.Fatal(err)
	}

	tab := &WorkspaceTab{
		ID:            "tab-1",
		WorkspaceRoot: dir,
	}

	if _, err := tab.PinFile("f1.txt"); err != nil {
		t.Fatal(err)
	}
	if _, err := tab.PinFile("f2.txt"); err != nil {
		t.Fatal(err)
	}
	if len(tab.GetPinnedFiles()) != 2 {
		t.Fatalf("expected 2 pinned files, got %d", len(tab.GetPinnedFiles()))
	}

	if err := tab.UnpinFile("f1.txt"); err != nil {
		t.Fatal(err)
	}
	pinned := tab.GetPinnedFiles()
	if len(pinned) != 1 || pinned[0] != "f2.txt" {
		t.Fatalf("expected [f2.txt], got %v", pinned)
	}

	// Unpin non-existent should succeed without error
	if err := tab.UnpinFile("nonexistent.txt"); err != nil {
		t.Fatalf("unexpected error unpinning non-existent file: %v", err)
	}
}

func TestTabPinnedContextBlock(t *testing.T) {
	dir := t.TempDir()
	f1 := filepath.Join(dir, "schema.sql")
	sqlContent := "CREATE TABLE users (id INT PRIMARY KEY, name TEXT);"
	if err := os.WriteFile(f1, []byte(sqlContent), 0o644); err != nil {
		t.Fatal(err)
	}

	tab := &WorkspaceTab{
		ID:            "tab-1",
		WorkspaceRoot: dir,
	}

	if _, err := tab.PinFile("schema.sql"); err != nil {
		t.Fatal(err)
	}

	block := tab.PinnedContextBlock()
	if !strings.Contains(block, "<pinned_context>") {
		t.Fatalf("expected <pinned_context> in block, got:\n%s", block)
	}
	if !strings.Contains(block, `<file path="schema.sql">`) {
		t.Fatalf("expected file path tag in block, got:\n%s", block)
	}
	if !strings.Contains(block, sqlContent) {
		t.Fatalf("expected sql content in block, got:\n%s", block)
	}

	infoList := tab.GetPinnedFilesInfo()
	if len(infoList) != 1 {
		t.Fatalf("expected 1 info item, got %d", len(infoList))
	}
	if infoList[0].Path != "schema.sql" || infoList[0].SizeBytes != int64(len(sqlContent)) {
		t.Fatalf("unexpected info item: %+v", infoList[0])
	}
}

func TestPinnedContextEndToEndProviderRequest(t *testing.T) {
	dir := t.TempDir()
	docPath := filepath.Join(dir, "architecture.md")
	docContent := "# Architecture\nUse clean layered architecture without global singletons.\n"
	if err := os.WriteFile(docPath, []byte(docContent), 0o644); err != nil {
		t.Fatal(err)
	}

	tab := &WorkspaceTab{
		ID:            "tab-e2e",
		WorkspaceRoot: dir,
	}

	if _, err := tab.PinFile("architecture.md"); err != nil {
		t.Fatalf("pin file: %v", err)
	}

	baseSystem := "You are a helpful coding assistant."
	pinnedBlock := tab.PinnedContextBlock()
	initialSystem := baseSystem + "\n\n" + pinnedBlock

	prov := &capturingProvider{}
	exec := agent.New(prov, tool.NewRegistry(), agent.NewSession(initialSystem), agent.Options{}, event.Discard)
	ctrl := control.New(control.Options{
		Runner:       exec,
		Executor:     exec,
		SystemPrompt: initialSystem,
		SessionDir:   dir,
		SessionPath:  filepath.Join(dir, "session.jsonl"),
		Label:        "test-e2e",
		Sink:         event.Discard,
	})
	tab.Ctrl = ctrl

	// 1. Execute first user turn and assert provider receives pinned_context block
	if err := ctrl.RunTurn(context.Background(), "How should we design the service?"); err != nil {
		t.Fatalf("RunTurn: %v", err)
	}

	reqMsgs := prov.lastRequestMessages(t)
	if len(reqMsgs) == 0 {
		t.Fatal("expected provider to receive messages, got none")
	}
	sysMsg := reqMsgs[0]
	if sysMsg.Role != provider.RoleSystem {
		t.Fatalf("expected first message to be system role, got %s", sysMsg.Role)
	}
	if !strings.Contains(sysMsg.Content, "<pinned_context>") {
		t.Fatalf("expected <pinned_context> in system prompt sent to provider, got:\n%s", sysMsg.Content)
	}
	if !strings.Contains(sysMsg.Content, `<file path="architecture.md">`) {
		t.Fatalf("expected architecture.md file tag in provider system prompt, got:\n%s", sysMsg.Content)
	}
	if !strings.Contains(sysMsg.Content, docContent) {
		t.Fatalf("expected architecture content in provider system prompt, got:\n%s", sysMsg.Content)
	}

	// 2. Unpin the file and sync tab pinned context dynamically
	if err := tab.UnpinFile("architecture.md"); err != nil {
		t.Fatalf("UnpinFile: %v", err)
	}
	updatedSysPrompt := updatePinnedContextInSystemPrompt(ctrl.SystemPrompt(), tab.PinnedContextBlock())
	ctrl.UpdateSystemPrompt(updatedSysPrompt)

	// 3. Execute second turn and assert provider system prompt no longer has pinned_context
	if err := ctrl.RunTurn(context.Background(), "Next question"); err != nil {
		t.Fatalf("RunTurn 2: %v", err)
	}

	reqMsgs2 := prov.lastRequestMessages(t)
	sysMsg2 := reqMsgs2[0]
	if strings.Contains(sysMsg2.Content, "<pinned_context>") {
		t.Fatalf("expected <pinned_context> to be removed after unpin, got:\n%s", sysMsg2.Content)
	}
}

func TestTabPinnedFilesPersistence(t *testing.T) {
	isolateDesktopUserDirs(t)
	cfgDir := desktopConfigDir()

	entry := desktopTabEntry{
		ID:            "tab-persist",
		Scope:         "project",
		WorkspaceRoot: "/tmp/fake-workspace",
		TopicID:       "topic-1",
		PinnedFiles:   []string{"README.md", "docs/api.md"},
	}

	// Save
	entries := []desktopTabEntry{entry}
	app := &App{}
	app.saveTabsWrite(cfgDir, entries, "tab-persist", 1)

	// Load
	f := loadTabsFile()
	if len(f.Tabs) != 1 {
		t.Fatalf("expected 1 saved tab, got %d", len(f.Tabs))
	}
	loaded := f.Tabs[0]
	if len(loaded.PinnedFiles) != 2 {
		t.Fatalf("expected 2 pinned files in loaded tab, got %d", len(loaded.PinnedFiles))
	}
	if loaded.PinnedFiles[0] != "README.md" || loaded.PinnedFiles[1] != "docs/api.md" {
		t.Fatalf("expected [README.md docs/api.md], got %v", loaded.PinnedFiles)
	}
}
