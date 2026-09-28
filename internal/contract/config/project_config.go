package config

import (
	"log/slog"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

// Project config files. The plain file is reasonix.toml in the project root.
// The hidden file is <root>/.reasonix/config.toml, used only when the user-global
// [project_config] hidden setting is on; the plain file then still wins when
// present, and the hidden file is where new configs are created.
const (
	projectConfigFile  = "reasonix.toml"
	projectConfigLocal = ".reasonix" + string(filepath.Separator) + "config.toml"
)

// projectConfigCandidates returns the plain and hidden project config paths for
// a resolved root.
func projectConfigCandidates(root string) (plain, local string) {
	root = resolveRoot(root)
	plain, local = projectConfigFile, projectConfigLocal
	if root != "." {
		plain = filepath.Join(root, projectConfigFile)
		local = filepath.Join(root, projectConfigLocal)
	}
	return plain, local
}

// projectConfigPathForRoot resolves root's project config. With hidden off it is
// the plain file. With hidden on, the plain file wins when it exists and the
// hidden file is otherwise both the read and the creation target.
func projectConfigPathForRoot(root string, hidden bool) string {
	plain, local := projectConfigCandidates(root)
	if !hidden {
		return plain
	}
	if _, err := os.Lstat(plain); err == nil {
		return plain
	}
	return local
}

// projectConfigBothExist reports whether root holds both the plain and the
// hidden project config, the ambiguous case where the plain file wins.
func projectConfigBothExist(root string) bool {
	plain, local := projectConfigCandidates(root)
	_, plainErr := os.Lstat(plain)
	_, localErr := os.Lstat(local)
	return plainErr == nil && localErr == nil
}

// projectConfigReadPaths lists the project files a plugin migration must scan:
// the plain file alone, or both once the hidden location is in use.
func projectConfigReadPaths(root string) []string {
	plain, local := projectConfigCandidates(root)
	if processRoots().projectConfigHidden() {
		return []string{plain, local}
	}
	return []string{plain}
}

// ProjectConfigPath resolves root's project config against the process binding.
func ProjectConfigPath(root string) string { return processRoots().ProjectConfigPath(root) }

// ProjectConfigPath resolves root's project config against this binding. The
// hidden location is used only when the user-global config opts in.
func (r Roots) ProjectConfigPath(root string) string {
	return projectConfigPathForRoot(root, r.projectConfigHidden())
}

// projectConfigHidden reports whether the user-global config opts into the
// hidden <root>/.reasonix/config.toml project config. It reads the user config
// alone: a repository's reasonix.toml can never turn it on.
func (r Roots) projectConfigHidden() bool {
	path := r.userConfigLoadPath()
	if path == "" {
		return false
	}
	var partial Config
	if _, err := decodeTOMLFile(path, &partial); err != nil {
		return false
	}
	return partial.ProjectConfigHidden
}

// projectConfigPathForLoad picks the project config file from the user-global
// hidden choice and access-checks it before the load reads it.
func projectConfigPathForLoad(root string, hidden bool) (string, error) {
	path := projectConfigPathForRoot(root, hidden)
	if hidden && projectConfigBothExist(root) {
		slog.Warn("project config: reasonix.toml and .reasonix/config.toml both exist; reasonix.toml wins", "root", root)
	}
	if _, err := resolveConfigAccessPath(path, false); err != nil {
		return "", err
	}
	return path, nil
}

// holdProjectConfigHidden keeps the user-global choice across the project merge
// and warns when the project file tried to set it.
func holdProjectConfigHidden(cfg *Config, hidden bool, projectMeta toml.MetaData) {
	cfg.ProjectConfigHidden = hidden
	if projectMeta.IsDefined("project_config_hidden") {
		cfg.addLoadWarning("project reasonix.toml sets project_config_hidden, which is user-global; it was ignored")
	}
}
