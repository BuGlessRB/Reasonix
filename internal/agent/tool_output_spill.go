package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"reasonix/internal/store"
)

// Oversize tool output goes to a file; the context keeps a pointer. A head+tail
// sketch is a guess about which part mattered, made by the wrong party — only
// the model knows, and read_file plus grep let it decide.

// Head lines that ride along, so the model can judge whether to open the file.
const spilledOutputHeadLines = 20

// The threshold is window-relative: a fixed cap is wrong at both ends, being a
// rounding error in a 1M window and larger than a 20K one. Windows at or above
// ~128K keep the historical 32KB exactly.
const (
	toolOutputBudgetRatio = 0.10
	minToolOutputBytes    = 4 * 1024
	spillBytesPerToken    = 3 // deliberately low; CJK runs near 1
)

// toolOutputBudget is the byte ceiling for one first-visible tool result.
func (a *Agent) toolOutputBudget() int {
	if a == nil || a.contextWindow <= 0 {
		return maxToolOutputBytes
	}
	n := int(float64(a.contextWindow) * spillBytesPerToken * toolOutputBudgetRatio)
	return min(max(n, minToolOutputBytes), maxToolOutputBytes)
}

// spillToolOutput writes body beside the session and returns the pointer text
// that replaces it. ok is false only when there is nowhere the model could read
// the file back from, and the caller falls back to truncation.
func (a *Agent) spillToolOutput(body, toolName, toolCallID string) (string, bool) {
	dir := store.SessionOutputsDir(a.SessionPath())
	if dir == "" {
		dir = a.archiveOutputsDir()
	}
	if dir == "" {
		return "", false
	}
	name := spillFileName(toolName, toolCallID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", false
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		return "", false
	}
	return spillPointer(path, body, toolName), true
}

// archiveOutputsDir is where an agent with no transcript of its own spills.
// Sub-agents are the case that matters: they carry no session path, so without
// this every oversize result they saw was truncated while the lossless path sat
// one field away. The archive already holds content moved out of context, and
// they inherit its location from the parent.
func (a *Agent) archiveOutputsDir() string {
	root := strings.TrimSpace(a.archiveDir)
	if root == "" {
		return ""
	}
	return filepath.Join(root, "outputs")
}

// spillFileName keys the file by tool call, so a directory listing reads as the
// turn's history and a re-read finds the same file.
func spillFileName(toolName, toolCallID string) string {
	base := strings.TrimSpace(toolCallID)
	if base == "" {
		base = strings.TrimSpace(toolName)
	}
	if base == "" {
		base = "output"
	}
	safe := make([]rune, 0, len(base))
	for _, r := range base {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			safe = append(safe, r)
		default:
			safe = append(safe, '-')
		}
	}
	trimmed := strings.Trim(string(safe), "-")
	if trimmed == "" {
		trimmed = "output"
	}
	if len(trimmed) > 96 {
		trimmed = trimmed[:96]
	}
	return trimmed + ".txt"
}

// spillPointer is what the model sees: the size, the path, how to get the rest,
// then the opening lines.
func spillPointer(path, body, toolName string) string {
	lines := strings.Count(body, "\n") + 1
	var b strings.Builder
	fmt.Fprintf(&b, "[%s output kept out of context: %d lines, %s]\n", displayToolName(toolName), lines, humanBytes(len(body)))
	fmt.Fprintf(&b, "Full output: %s\n", path)
	b.WriteString("Read a window of it with read_file (offset/limit), or search it with grep. It is not in this conversation.\n")
	if head := headLines(body, spilledOutputHeadLines); head != "" {
		fmt.Fprintf(&b, "\nFirst %d lines:\n%s\n", spilledOutputHeadLines, head)
	}
	return b.String()
}

func displayToolName(name string) string {
	if n := strings.TrimSpace(name); n != "" {
		return n
	}
	return "tool"
}

func headLines(body string, n int) string {
	lines := strings.SplitN(body, "\n", n+1)
	if len(lines) > n {
		lines = lines[:n]
	}
	return strings.Join(lines, "\n")
}

func humanBytes(n int) string {
	switch {
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.0f KB", float64(n)/(1<<10))
	default:
		return fmt.Sprintf("%d B", n)
	}
}

// boundToolOutput is the single gate every tool result passes before it becomes
// context. The second return is the truncation notice; spilling has none,
// because nothing was lost.
func (a *Agent) boundToolOutput(body, toolName, toolCallID string) (string, string) {
	if len(body) <= a.toolOutputBudget() {
		return body, ""
	}
	if pointer, ok := a.spillToolOutput(body, toolName, toolCallID); ok {
		return pointer, ""
	}
	return truncateToolOutputFor(body, toolName, toolCallID, a.toolOutputBudget())
}
