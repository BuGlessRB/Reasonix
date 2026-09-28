package boot

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"runtime"
	"strings"

	"reasonix/internal/contract/config"
)

func rememberedPermissionPath(roots config.Roots, workspaceRoot string) string {
	if strings.TrimSpace(workspaceRoot) == "" {
		return ""
	}
	root, err := filepath.Abs(workspaceRoot)
	if err != nil {
		root = filepath.Clean(workspaceRoot)
	}
	if real, err := filepath.EvalSymlinks(root); err == nil {
		root = real
	}
	if runtime.GOOS == "windows" {
		root = strings.ToLower(root)
	}
	sum := sha256.Sum256([]byte(root))
	return filepath.Join(roots.Dir(config.RootState), "permissions", hex.EncodeToString(sum[:])+".toml")
}

func rememberedPermissionRules(roots config.Roots, workspaceRoot string) ([]string, error) {
	if strings.TrimSpace(workspaceRoot) == "" {
		return nil, nil
	}
	path := rememberedPermissionPath(roots, workspaceRoot)
	cfg, err := config.LoadForEditReadOnlyStrict(path)
	if err != nil {
		return nil, fmt.Errorf("load remembered permission rules from %s: %w", path, err)
	}
	return cfg.Permissions.Allow, nil
}
