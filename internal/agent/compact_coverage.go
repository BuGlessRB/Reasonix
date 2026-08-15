package agent

import (
	"encoding/json"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"reasonix/internal/evidence"
	"reasonix/internal/provider"
	"reasonix/internal/shellparse"
)

// Acceptance used to ask a digest one question: is it smaller? A digest that
// summarized nothing answers yes. So a fold is also measured on the two losses
// that hurt — a change forgotten, and a failure forgotten and so repeated.

// foldCoverage is what one digest kept of the facts a fold had to carry.
type foldCoverage struct {
	Mutations  []string // files the fold changed
	Failures   []string // commands the fold saw fail
	MissingMut []string
	MissingFai []string
}

// Required is how many facts the digest had to carry.
func (c foldCoverage) Required() int { return len(c.Mutations) + len(c.Failures) }

// Missing is how many it dropped.
func (c foldCoverage) Missing() int { return len(c.MissingMut) + len(c.MissingFai) }

// LostAChange reports a forgotten change — the severity line, and one the
// mechanism owns rather than a ratio someone picked: a forgotten change makes
// the agent wrong, a forgotten failure only makes it slow.
func (c foldCoverage) LostAChange() bool { return len(c.MissingMut) > 0 }

// LostEveryChange reports a digest that carried none of the fold's changes.
// That is not a quality judgment needing a threshold — it is a broken digest.
func (c foldCoverage) LostEveryChange() bool {
	return len(c.Mutations) > 0 && len(c.MissingMut) == len(c.Mutations)
}

// Reason names what went missing, for the receipt and the retry instruction.
func (c foldCoverage) Reason() string {
	var parts []string
	if len(c.MissingMut) > 0 {
		parts = append(parts, "changed files not mentioned: "+strings.Join(c.MissingMut, ", "))
	}
	if len(c.MissingFai) > 0 {
		parts = append(parts, "failures not mentioned: "+strings.Join(c.MissingFai, ", "))
	}
	return strings.Join(parts, "; ")
}

// foldFacts derives what a fold must carry forward, through the same pure
// classifier the ledger uses — the live ledger is per-task and never spans a
// fold. readOnly decides whether a call counts as a change.
func foldFacts(region []provider.Message, readOnly func(string) bool) foldCoverage {
	var cov foldCoverage
	seenMut, seenFail := map[string]bool{}, map[string]bool{}
	calls := map[string]provider.ToolCall{}
	for _, m := range region {
		for _, tc := range m.ToolCalls {
			calls[tc.ID] = tc
		}
		if m.Role != provider.RoleTool {
			continue
		}
		call, ok := calls[m.ToolCallID]
		if !ok {
			continue
		}
		failed := isErrorMessage(m)
		rec := evidence.ReceiptFromToolCall(call.Name, json.RawMessage(call.Arguments), !failed, readOnly(call.Name))
		if !failed && rec.Mutation {
			for _, p := range rec.Paths {
				if p = strings.TrimSpace(p); p != "" && !seenMut[p] {
					seenMut[p] = true
					cov.Mutations = append(cov.Mutations, p)
				}
			}
		}
		if failed {
			if cmd := commandSignature(rec.Command); cmd != "" && !seenFail[cmd] {
				seenFail[cmd] = true
				cov.Failures = append(cov.Failures, cmd)
			}
		}
	}
	sort.Strings(cov.Mutations)
	sort.Strings(cov.Failures)
	return cov
}

// measureFoldCoverage checks a digest against the facts its fold produced.
func measureFoldCoverage(region []provider.Message, readOnly func(string) bool, digest string) foldCoverage {
	cov := foldFacts(region, readOnly)
	haystack := strings.ToLower(digest)
	for _, p := range cov.Mutations {
		if !mentionsPath(haystack, p) {
			cov.MissingMut = append(cov.MissingMut, p)
		}
	}
	for _, c := range cov.Failures {
		if !strings.Contains(haystack, strings.ToLower(c)) {
			cov.MissingFai = append(cov.MissingFai, c)
		}
	}
	return cov
}

// mentionsPath accepts the full path or the bare file name: insisting on the
// full path would fail a digest that did its job.
func mentionsPath(haystack, p string) bool {
	lower := strings.ToLower(p)
	if strings.Contains(haystack, lower) {
		return true
	}
	base := path.Base(filepath.ToSlash(lower))
	return base != "" && base != "." && base != "/" && strings.Contains(haystack, base)
}

// commandSignature reduces a command to what a digest would repeat: the program
// and its first argument.
func commandSignature(command string) string {
	fields, malformed := shellparse.StaticFields(strings.TrimSpace(command))
	if malformed != "" || len(fields) == 0 {
		if trimmed := strings.TrimSpace(command); trimmed != "" {
			return firstLine(trimmed)
		}
		return ""
	}
	if len(fields) > 2 {
		fields = fields[:2]
	}
	return strings.Join(fields, " ")
}

// coverageRetryInstruction names what was dropped rather than restating the
// whole contract, so the second attempt spends its budget on the gap.
func coverageRetryInstruction(cov foldCoverage) string {
	var b strings.Builder
	b.WriteString("The previous digest of these messages dropped facts that must survive. Rewrite it, keeping everything it had right and additionally covering:\n")
	for _, p := range cov.MissingMut {
		b.WriteString("- the change made to " + p + ": what was changed and why\n")
	}
	for _, c := range cov.MissingFai {
		b.WriteString("- the failure of `" + c + "`: what failed and how it was resolved, or that it was not\n")
	}
	return b.String()
}

// toolIsReadOnly answers for this agent's registry. An unknown tool reads as
// read-only: over-counting changes buys repair calls for facts no digest can
// carry, and a check that only adds cost should ask for less.
func (a *Agent) toolIsReadOnly(name string) bool {
	if a == nil || a.svc.tools == nil {
		return true
	}
	t, ok := a.svc.tools.Get(name)
	if !ok {
		return true
	}
	return t.ReadOnly()
}
