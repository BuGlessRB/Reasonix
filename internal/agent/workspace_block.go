package agent

import "strconv"

// WorkspaceBlock renders the transient block naming the directory this turn
// runs in. It rides the turn rather than the system prompt: the path is the
// only per-project text in an otherwise identical prefix, and everything after
// it — the rest of the prompt and every tool schema — misses the provider's
// prefix cache on a project the machine has not warmed.
func WorkspaceBlock(root string) string {
	if root == "" {
		return ""
	}
	return "<workspace>\nCurrent workspace: " + strconv.Quote(root) +
		". Shell commands start there and every tool resolves relative paths from it.\n</workspace>"
}

// WithWorkspace prefixes content with the transient workspace block unless the
// turn already starts with one.
func WithWorkspace(content, root string) string {
	block := WorkspaceBlock(root)
	if block == "" || hasLeadingInjectedBlock(content, "workspace") {
		return content
	}
	return block + "\n\n" + content
}
