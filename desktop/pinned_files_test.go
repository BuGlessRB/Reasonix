package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
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
