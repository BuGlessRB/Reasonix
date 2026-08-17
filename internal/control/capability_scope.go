package control

import (
	"path/filepath"

	"reasonix/internal/config"
	"reasonix/internal/workspaceid"
)

// CapabilityScope names the project a capability listing belongs to. A shell
// holds several projects at once, so a settings pane that silently follows the
// active tab shows different content under an unchanged heading; every listing
// carries this so the surface can say which folder it is answering for.
type CapabilityScope struct {
	Root string `json:"root"`
	Name string `json:"name"`
	Key  string `json:"key"`
	// Repo reports that Trees working trees share these settings — the reason a
	// switch made in the main tree still applies in a worktree cut from it.
	Repo   bool   `json:"repo"`
	Trees  int    `json:"trees,omitempty"`
	Branch string `json:"branch,omitempty"`
	// Overrides counts what this project decided for itself rather than
	// inheriting: how a picker answers "where did I change something".
	Overrides int `json:"overrides"`
}

// CapabilityScope describes the workspace this controller's MCP and skill
// listings answer for.
func (c *Controller) CapabilityScope() CapabilityScope {
	root := c.workspaceRoot
	info := workspaceid.Describe(root)
	scope := CapabilityScope{
		Root:   root,
		Name:   filepath.Base(root),
		Key:    info.Key,
		Repo:   info.RepoDir != "",
		Trees:  info.Trees,
		Branch: info.Branch,
	}
	if rows, err := config.DefaultActivationStore().ProjectOverrides(root); err == nil {
		scope.Overrides = len(rows)
	}
	return scope
}
