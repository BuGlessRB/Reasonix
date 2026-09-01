package agent

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

type incompleteReadGrepArgs struct {
	Pattern string
	Path    string
}

type readStrategyReceiptArgs struct {
	ReadID            string   `json:"read_id"`
	SearchToolCallIDs []string `json:"search_tool_call_ids"`
	ReadToolCallIDs   []string `json:"read_tool_call_ids"`
	Conclusion        string   `json:"conclusion"`
}

func parseIncompleteReadGrepArgs(raw json.RawMessage) (incompleteReadGrepArgs, bool) {
	var args struct {
		Pattern string `json:"pattern"`
		Path    string `json:"path"`
	}
	if json.Unmarshal(raw, &args) != nil || strings.TrimSpace(args.Pattern) == "" || strings.TrimSpace(args.Path) == "" {
		return incompleteReadGrepArgs{}, false
	}
	return incompleteReadGrepArgs{Pattern: args.Pattern, Path: args.Path}, true
}

func parseReadStrategyReceiptArgs(raw json.RawMessage) (readStrategyReceiptArgs, bool) {
	var args readStrategyReceiptArgs
	if json.Unmarshal(raw, &args) != nil || strings.TrimSpace(args.ReadID) == "" {
		return readStrategyReceiptArgs{}, false
	}
	return args, true
}

func grepMatchLines(output string) []int {
	var lines []int
	seen := make(map[int]bool)
	for line := range strings.SplitSeq(output, "\n") {
		for start := 0; start < len(line); {
			colon := strings.IndexByte(line[start:], ':')
			if colon < 0 {
				break
			}
			colon += start
			next := strings.IndexByte(line[colon+1:], ':')
			if next < 0 {
				break
			}
			next += colon + 1
			n, err := strconv.Atoi(line[colon+1 : next])
			if err == nil && n > 0 {
				if !seen[n] {
					seen[n] = true
					lines = append(lines, n)
				}
				break
			}
			start = colon + 1
		}
	}
	return lines
}

func (s *incompleteReadState) resetStrategyEvidenceForVersionLocked(entry *incompleteRead, current incompleteReadFileVersion) {
	entry.searches = make(map[string]incompleteReadSearch)
	entry.reads = make(map[string]incompleteReadWindow)
	entry.targetReadID = ""
	entry.targetObserved = nil
	entry.targetEnd = 0
	entry.pendingReceipt = nil
	entry.strategyVersion = current
}

func (s *incompleteReadState) observeStrategySearch(plan *toolCallPlan, output string, visibleFull bool) incompleteReadTransition {
	if plan == nil || plan.incompleteReadRoot == "" || plan.incompleteReadAction != incompleteReadActionStrategySearch {
		return incompleteReadTransition{}
	}
	args, ok := parseIncompleteReadGrepArgs(plan.runArgs)
	if !ok {
		return incompleteReadTransition{}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	entry := s.entries[plan.incompleteReadRoot]
	if entry == nil || entry.phase != incompleteReadStrategy {
		return incompleteReadTransition{}
	}
	transition := incompleteReadTransition{readID: entry.readID, path: entry.path}
	if !visibleFull || strings.Contains(output, "timed out after") {
		return transition
	}
	current := snapshotIncompleteReadFile(entry.path)
	if !sameIncompleteReadFileVersion(entry.strategyVersion, current) {
		s.resetStrategyEvidenceForVersionLocked(entry, current)
	}
	entry.searches[plan.call.ID] = incompleteReadSearch{
		callID: plan.call.ID, pattern: args.Pattern, matchLines: grepMatchLines(output),
	}
	s.roundProgress = true
	transition.strategyProgress = true
	return transition
}

func uniqueNonEmptyIDs(ids []string) ([]string, error) {
	if len(ids) == 0 {
		return nil, fmt.Errorf("at least one tool call id is required")
	}
	seen := make(map[string]bool, len(ids))
	out := make([]string, 0, len(ids))
	for _, raw := range ids {
		id := strings.TrimSpace(raw)
		if id == "" {
			return nil, fmt.Errorf("tool call ids must be non-empty")
		}
		if seen[id] {
			return nil, fmt.Errorf("duplicate tool call id %q", id)
		}
		seen[id] = true
		out = append(out, id)
	}
	return out, nil
}

func (s *incompleteReadState) submitStrategyReceipt(args readStrategyReceiptArgs) (string, error) {
	searchIDs, err := uniqueNonEmptyIDs(args.SearchToolCallIDs)
	if err != nil {
		return "", fmt.Errorf("read strategy receipt: search_tool_call_ids: %w", err)
	}
	readIDs, err := uniqueNonEmptyIDs(args.ReadToolCallIDs)
	if err != nil {
		return "", fmt.Errorf("read strategy receipt: read_tool_call_ids: %w", err)
	}
	conclusion := strings.TrimSpace(args.Conclusion)
	if conclusion == "" {
		return "", fmt.Errorf("read strategy receipt: conclusion is required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	entry := s.entries[strings.TrimSpace(args.ReadID)]
	if entry == nil || entry.phase != incompleteReadStrategy {
		s.roundViolation = true
		return "", fmt.Errorf("read strategy receipt: active read_id %q was not found or still has an unfinished page", args.ReadID)
	}
	current := snapshotIncompleteReadFile(entry.path)
	if !sameIncompleteReadFileVersion(entry.strategyVersion, current) {
		s.resetStrategyEvidenceForVersionLocked(entry, current)
		s.roundViolation = true
		return "", fmt.Errorf("read strategy receipt: the target file changed; repeat grep and explicit read_file windows")
	}

	patterns := make([]string, 0, len(searchIDs))
	var matchedLines []int
	for _, id := range searchIDs {
		search, ok := entry.searches[id]
		if !ok {
			s.roundViolation = true
			return "", fmt.Errorf("read strategy receipt: grep tool call %q is not complete evidence for read_id %q", id, entry.readID)
		}
		patterns = append(patterns, search.pattern)
		matchedLines = append(matchedLines, search.matchLines...)
	}

	ranges := make([]string, 0, len(readIDs))
	overlaps := len(matchedLines) == 0
	for _, id := range readIDs {
		window, ok := entry.reads[id]
		if !ok {
			s.roundViolation = true
			return "", fmt.Errorf("read strategy receipt: read_file tool call %q is not a fully consumed explicit window for read_id %q", id, entry.readID)
		}
		ranges = append(ranges, fmt.Sprintf("%d-%d", window.startLine, window.endLine))
		for _, line := range matchedLines {
			if line >= window.startLine && line <= window.endLine {
				overlaps = true
				break
			}
		}
	}
	if !overlaps {
		s.roundViolation = true
		return "", fmt.Errorf("read strategy receipt: no selected read_file window overlaps a cited grep match line")
	}

	entry.pendingReceipt = &incompleteReadReceipt{
		searchIDs: searchIDs, readIDs: readIDs, conclusion: conclusion,
	}
	s.roundProgress = true
	payload, _ := json.Marshal(map[string]any{
		"read_id":         entry.readID,
		"path":            entry.path,
		"search_patterns": patterns,
		"read_ranges":     ranges,
		"conclusion":      conclusion,
		"status":          "validated_pending_round_boundary",
		"whole_file_read": false,
	})
	return string(payload), nil
}
