package evidence

import "testing"

// A repository store is written by `git commit` and by `git status` alike, so
// counting one as a changed path sent review coverage after the tree's own
// bookkeeping — and raised an otherwise clean commit to medium risk.
func TestVCSStoreIsNotAChangedPath(t *testing.T) {
	for _, p := range []string{"/w/.git", "/w/.git/index", ".git/COMMIT_EDITMSG", "/w/.hg/store", "/w/.svn/wc.db"} {
		if got := ClassifyPath(p); got != PathVCSStore {
			t.Errorf("ClassifyPath(%q) = %v, want PathVCSStore", p, got)
		}
	}
	receipts := []Receipt{{
		ToolName: "bash", Success: true, Mutation: true,
		Paths: []string{"/w/.git", "/w/.git/index"},
	}}
	if got := ClassifyMutationRisk(receipts, 0, nil); got != RiskLow {
		t.Fatalf("risk for a commit that touched only the store = %v, want %v", got, RiskLow)
	}
}

// An unfamiliar path is code until a rule says otherwise: the zero value must
// not be the one that lowers the bar.
func TestUnknownPathIsProduction(t *testing.T) {
	for _, p := range []string{"internal/agent/agent.go", "src/App.tsx", "Makefile", "weird/thing.zzz"} {
		if got := ClassifyPath(p); got != PathProduction {
			t.Errorf("ClassifyPath(%q) = %v, want PathProduction", p, got)
		}
	}
	for _, p := range []string{"internal/x_test.go", "docs/guide.md", "src/i18n/zh.ts", "pkg/testdata/a.json"} {
		if got := ClassifyPath(p); got != PathSupporting {
			t.Errorf("ClassifyPath(%q) = %v, want PathSupporting", p, got)
		}
	}
}
