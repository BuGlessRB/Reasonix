// rule_group.go — tools that one rule name answers for.
package permission

// A rule group is a set of tools one grant covers: approving an edit answers
// for every file writer, and approving a site answers for loading, reading and
// operating it alike.
const (
	fileMutationGroup = "file_mutation"
	browserGroup      = "browser"
)

// IsBrowserTool reports whether a tool operates the agent's browser.
func IsBrowserTool(toolName string) bool { return groupOf(toolName) == browserGroup }

func groupOf(toolName string) string {
	switch toolName {
	case "write_file", "edit_file", "multi_edit", "move_file", "notebook_edit", "delete_range", "delete_symbol":
		return fileMutationGroup
	case "browser_open", "browser_read", "browser_act":
		return browserGroup
	}
	return ""
}

// inGroup reports whether toolName belongs to the group a canonical rule tool
// names. A rule tool that names no group covers no tool through this.
func inGroup(ruleTool, toolName string) bool {
	return ruleTool != "" && groupOf(toolName) == ruleTool
}

// groupGrantRule is the grant a group records. File writers are granted as a
// whole and a site by its origin; a browser call that names no site has no
// grant any later call could reuse.
func groupGrantRule(toolName, subject string) (string, bool) {
	switch groupOf(toolName) {
	case fileMutationGroup:
		return "Edit", true
	case browserGroup:
		if subject != "" {
			return "Browser=" + subject, true
		}
	}
	return "", false
}
