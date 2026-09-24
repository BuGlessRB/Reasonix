package cli

import (
	"strings"
)

func renderBranchTree(tree string) string {
	lines := strings.Split(tree, "\n")
	for i, line := range lines {
		lines[i] = renderBranchTreeLine(line)
	}
	return strings.Join(lines, "\n")
}

func renderBranchTreeLine(line string) string {
	if line == "branches:" {
		return accent(line)
	}
	joint := strings.LastIndex(line, "├─ ")
	if alt := strings.LastIndex(line, "└─ "); alt > joint {
		joint = alt
	}
	if joint < 0 {
		return line
	}
	treePrefix := line[:joint+len("├─ ")]
	parts := strings.SplitN(line[joint+len("├─ "):], "  ", 3)
	if len(parts) < 3 {
		return line
	}
	id, title, meta := parts[0], parts[1], parts[2]

	turns := meta
	current := ""
	if before, after, ok := strings.Cut(meta, "  "); ok {
		turns = before
		if strings.TrimSpace(after) == "current" {
			current = "  " + accent("current")
		} else if strings.TrimSpace(after) != "" {
			current = "  " + after
		}
	}
	return dim(treePrefix) + dim(id) + "  " + title + "  " + dim(turns) + current
}
