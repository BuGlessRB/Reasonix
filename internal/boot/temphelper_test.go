package boot

import (
	"testing"

	"reasonix/internal/testenv"
)

// robustTempDir is the name boot's call sites already use for testenv.TempDir,
// whose cleanup retries RemoveAll. Teardown here can leave a job goroutine or an
// MCP writer holding a file for a few milliseconds after Close returns, which
// plain t.TempDir turns into a red test with every assertion passed.
func robustTempDir(t *testing.T) string {
	t.Helper()
	return testenv.TempDir(t)
}
