package serve

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"reasonix/internal/base/testenv"
	"reasonix/internal/contract/provider"
	"reasonix/internal/state/sessionstore"
)

// seedVersionedSession writes a conversation and one version a rewind cut
// from it, and returns the workspace, the conversation and the version.
func seedVersionedSession(t *testing.T) (root, parentPath, version string) {
	t.Helper()
	home := testenv.TempDir(t)
	t.Setenv("REASONIX_HOME", home)
	t.Setenv("REASONIX_STATE_HOME", home)
	root = testenv.TempDir(t)
	dir := SessionDirFor(root)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	parentPath = filepath.Join(dir, "20260924-120000-deepseek.jsonl")
	parent := sessionstore.NewSession("sys")
	var cut []provider.Message
	for i := range 3 {
		parent.Add(provider.Message{Role: provider.RoleUser, Content: fmt.Sprintf("ask %d", i)})
		parent.Add(provider.Message{Role: provider.RoleAssistant, Content: fmt.Sprintf("answer %d", i)})
	}
	cut = append(cut, parent.Snapshot()...)
	cut = append(cut, provider.Message{Role: provider.RoleUser, Content: "the prompt later edited"},
		provider.Message{Role: provider.RoleAssistant, Content: "the reply it replaced"})
	if err := parent.Save(parentPath); err != nil {
		t.Fatal(err)
	}
	version, err := sessionstore.SaveSupersededVersion(parentPath, cut)
	if err != nil {
		t.Fatal(err)
	}
	return root, parentPath, version
}

// The sidebar shows one row for the conversation, with the version under it.
func TestSidebarHangsVersionsUnderTheirConversation(t *testing.T) {
	root, parentPath, version := seedVersionedSession(t)
	rows := NewHub(HubOptions{}).workspaceSessions(root, map[string]string{})
	if len(rows) != 1 || rows[0].Path != parentPath {
		t.Fatalf("rows = %+v, want only the conversation", rows)
	}
	if len(rows[0].Versions) != 1 || rows[0].Versions[0].Path != version || rows[0].Versions[0].Turns != 4 {
		t.Fatalf("versions = %+v, want the one cut version with its 4 turns", rows[0].Versions)
	}
}

// A version nobody lists must not outlive its conversation.
func TestRemovingAConversationRemovesItsVersions(t *testing.T) {
	root, parentPath, version := seedVersionedSession(t)
	if err := removeSessionFiles(SessionDirFor(root), parentPath); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(version); !os.IsNotExist(err) {
		t.Fatalf("the version outlived its conversation: %v", err)
	}
}
