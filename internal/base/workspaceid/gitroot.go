package workspaceid

import (
	"context"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"reasonix/internal/base/proc"
)

// rootCommitTimeout bounds the one git call a repository costs per process. A
// hung filesystem must not hold up a settings render that only wants a key.
const rootCommitTimeout = 2 * time.Second

// rootCommits memoizes the resolved first commit per shared git dir, so several
// call sites asking about one project spawn git once per process.
var rootCommits sync.Map

// rootCommitID returns the repository's first commit: the identity that stays
// the same when the working tree is moved or renamed, unlike the git-dir path.
// It asks git because walking the object store is O(history) and measured in
// seconds on a large repository; the answer is memoized per process.
func rootCommitID(commonDir string) string {
	if cached, ok := rootCommits.Load(commonDir); ok {
		return cached.(string)
	}
	// A shallow clone's history starts at its grafted boundary, which moves when
	// the clone is deepened; the path digest is the stable answer until then.
	if shallow, err := os.ReadFile(filepath.Join(commonDir, "shallow")); err == nil && strings.TrimSpace(string(shallow)) != "" {
		return ""
	}
	ctx, cancel := context.WithTimeout(context.Background(), rootCommitTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", "--no-replace-objects", "--git-dir="+commonDir, "rev-list", "--max-parents=0", "HEAD")
	proc.HideWindow(cmd)
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	// Unrelated histories merged together have several roots; the smallest id
	// keeps the choice independent of traversal order.
	root := ""
	for line := range strings.SplitSeq(string(out), "\n") {
		if line = strings.TrimSpace(line); !isHexSHA(line) {
			continue
		}
		if root == "" || line < root {
			root = line
		}
	}
	if root == "" {
		return ""
	}
	rootCommits.Store(commonDir, root)
	return root
}

func isHexSHA(s string) bool {
	if len(s) != 40 {
		return false
	}
	_, err := hex.DecodeString(s)
	return err == nil
}
