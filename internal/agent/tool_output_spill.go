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

// A spill is a fixed price: the instructions, plus at most the same again in
// preview. Counting the preview in lines would instead make a wide result cost
// more to leave context than a narrow one — the opposite of the point.

// spillToolOutput writes body beside the session and returns the pointer that
// replaces it. ok is false when there is nowhere the model could read the file
// back from, or when spilling would not pay for itself. There is deliberately
// no size threshold: context pays the pointer, the pointer is a pure function
// of the body, so the decision is derived rather than configured.
func (a *Agent) spillToolOutput(body, toolName, toolCallID string) (string, bool) {
	dir := store.SessionOutputsDir(a.SessionPath())
	if dir == "" {
		dir = a.archiveOutputsDir()
	}
	if dir == "" {
		return "", false
	}
	path := filepath.Join(dir, spillFileName(toolName, toolCallID))
	pointer := spillPointer(path, body, toolName)
	if !spillPays(len(body), len(pointer)) {
		return "", false
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", false
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		return "", false
	}
	return pointer, true
}

// spillPays reports whether moving a result out of context earns its keep: what
// is saved has to be worth at least what is spent. Spending is the pointer that
// stays behind, and a spill the model has to read back costs a whole extra turn,
// so breaking even is not enough to justify one.
func spillPays(body, pointer int) bool {
	return body-pointer >= pointer
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
// then as much of the start as the instructions themselves cost.
func spillPointer(path, body, toolName string) string {
	instr := spillInstructions(path, body, toolName)
	head := headWithin(body, len(instr))
	if head == "" {
		return instr
	}
	return instr + "\nOpening lines:\n" + head + "\n"
}

// spillInstructions is the irreducible part: where the output went and how to
// read it back. Its size is what a spill really costs, so it also bounds the
// preview — the model is told the shape, not shown a guess at the substance.
func spillInstructions(path, body, toolName string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "[%s output kept out of context: %d lines, %s]\n", displayToolName(toolName), strings.Count(body, "\n")+1, humanBytes(len(body)))
	fmt.Fprintf(&b, "Full output: %s\n", path)
	b.WriteString("Read a window of it with read_file (offset/limit), or search it with grep. It is not in this conversation.\n")
	return b.String()
}

// headWithin returns the whole leading lines of body that fit in max bytes, so a
// preview never cuts mid-line and never exceeds its budget.
func headWithin(body string, max int) string {
	end := 0
	for end < len(body) {
		nl := strings.IndexByte(body[end:], '\n')
		if nl < 0 {
			if len(body) <= max {
				end = len(body)
			}
			break
		}
		if end+nl+1 > max {
			break
		}
		end += nl + 1
	}
	return strings.TrimRight(body[:end], "\n")
}

func displayToolName(name string) string {
	if n := strings.TrimSpace(name); n != "" {
		return n
	}
	return "tool"
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
	if pointer, ok := a.spillToolOutput(body, toolName, toolCallID); ok {
		return pointer, ""
	}
	if len(body) <= maxToolOutputBytes {
		return body, ""
	}
	return truncateToolOutputFor(body, toolName, toolCallID, maxToolOutputBytes)
}
