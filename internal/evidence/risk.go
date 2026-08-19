package evidence

import (
	"path/filepath"
	"strings"

	"reasonix/internal/fileutil"
)

// RiskLevel classifies the latest post-mutation change set for adaptive review.
type RiskLevel string

const (
	RiskLow    RiskLevel = "low"
	RiskMedium RiskLevel = "medium"
	RiskHigh   RiskLevel = "high"
)

// highRiskToolHints elevate opaque or privileged mutation surfaces.
var highRiskToolHints = []string{
	"mcp__", "install_source", "install_skill", "plugin",
}

// ClassifyMutationRisk scores the change set after the latest mutation.
// Low: docs/tests/i18n only. Medium: ordinary production code. High: a path the
// project declared sensitive, 10+ paths, or an opaque write — one the host
// proved happened but cannot name a path for. A command it merely failed to
// prove read-only names no change set at all, so it scores nothing.
func ClassifyMutationRisk(receipts []Receipt, after int, sensitive []string) RiskLevel {
	start := max(after+1, 0)
	var paths []string
	seen := map[string]bool{}
	opaque := false
	hasProd := false
	onlyLow := true

	collect := func(r Receipt) bool {
		if !r.Success || !r.Mutation {
			return false
		}
		if len(r.Paths) == 0 && r.MutationEvidence == MutationProven {
			opaque = true
		}
		if toolLooksHighRisk(r.ToolName) {
			return true
		}
		for _, p := range r.Paths {
			if !seen[p] {
				seen[p] = true
				paths = append(paths, p)
			}
		}
		return false
	}

	// Include the mutation receipt itself.
	if after >= 0 && after < len(receipts) {
		if collect(receipts[after]) {
			return RiskHigh
		}
	}
	for i := start; i < len(receipts); i++ {
		if collect(receipts[i]) {
			return RiskHigh
		}
	}
	if opaque {
		return RiskHigh
	}
	if len(paths) == 0 {
		return RiskLow
	}
	if len(paths) >= 10 {
		return RiskHigh
	}
	for _, p := range paths {
		if pathIsDeclaredSensitive(p, sensitive) {
			return RiskHigh
		}
		if !pathLooksLowRisk(p) {
			onlyLow = false
			hasProd = true
		}
	}
	if onlyLow && !hasProd {
		return RiskLow
	}
	return RiskMedium
}

// MutationRiskAfter classifies risk from the ledger using the latest mutation.
func (l *Ledger) MutationRiskAfter(after int, sensitive []string) RiskLevel {
	if l == nil {
		return RiskLow
	}
	l.mu.Lock()
	receipts := append([]Receipt(nil), l.receipts...)
	l.mu.Unlock()
	return ClassifyMutationRisk(receipts, after, sensitive)
}

// PathsSince returns distinct paths from successful mutation/write receipts at
// or after the given index (inclusive of the mutation itself when after >= 0).
func (l *Ledger) PathsSince(after int) []string {
	if l == nil {
		return nil
	}
	start := max(after, 0)
	l.mu.Lock()
	defer l.mu.Unlock()
	seen := map[string]bool{}
	var out []string
	for i := start; i < len(l.receipts); i++ {
		r := l.receipts[i]
		if !r.Success || (!r.Mutation && !r.Write) {
			continue
		}
		for _, p := range r.Paths {
			if p == "" || seen[p] {
				continue
			}
			seen[p] = true
			out = append(out, p)
		}
	}
	return out
}

// pathIsDeclaredSensitive reports whether the project itself named this path.
// The predecessor guessed from spelling and could not tell `internal/auth` from
// `session_write_authority.go`, or a trace file from a data race.
func pathIsDeclaredSensitive(path string, sensitive []string) bool {
	for _, glob := range sensitive {
		if fileutil.MatchSlashGlob(path, glob) {
			return true
		}
	}
	return false
}

func pathLooksLowRisk(path string) bool {
	lower := strings.ToLower(filepath.ToSlash(path))
	base := filepath.Base(lower)
	if strings.HasSuffix(lower, "_test.go") || strings.HasSuffix(lower, "_test.ts") ||
		strings.HasSuffix(lower, ".test.ts") || strings.HasSuffix(lower, ".test.tsx") ||
		strings.HasSuffix(lower, "_spec.ts") || strings.HasSuffix(lower, ".spec.ts") {
		return true
	}
	if strings.Contains(lower, "/testdata/") || strings.Contains(lower, "/__tests__/") ||
		strings.Contains(lower, "/fixtures/") {
		return true
	}
	switch {
	case strings.HasSuffix(base, ".md"), strings.HasSuffix(base, ".mdx"),
		strings.HasSuffix(base, ".txt"), strings.HasSuffix(base, ".rst"):
		return true
	case strings.Contains(lower, "/docs/"), strings.Contains(lower, "/locales/"),
		strings.Contains(lower, "/i18n/"), strings.HasPrefix(base, "readme"):
		return true
	case strings.HasSuffix(base, ".css") && !strings.Contains(lower, "sandbox"):
		// Pure presentation styles are low risk unless mixed with other paths.
		return true
	}
	return false
}

func toolLooksHighRisk(name string) bool {
	lower := strings.ToLower(strings.TrimSpace(name))
	for _, hint := range highRiskToolHints {
		if strings.Contains(lower, hint) {
			return true
		}
	}
	return false
}
