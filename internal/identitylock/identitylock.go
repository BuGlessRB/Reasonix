// Package identitylock combines filesystem path identity with filelock's
// process-local queue and cross-process advisory lock.
package identitylock

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"reasonix/internal/filelock"
	"reasonix/internal/pathidentity"
)

type Mode = filelock.Mode

const (
	ModeExclusive = filelock.ModeExclusive
	ModeShared    = filelock.ModeShared
)

var ErrHeld = filelock.ErrHeld

func Acquire(ctx context.Context, path string) (func(), error) {
	return AcquireMode(ctx, path, ModeExclusive)
}

func AcquireMode(ctx context.Context, path string, mode Mode) (func(), error) {
	accessPath, key, err := resolve(path)
	if err != nil {
		return nil, err
	}
	return filelock.AcquireModeWithKey(ctx, accessPath, key, mode)
}

func AcquireWithExternalTimeout(ctx context.Context, path string, timeout time.Duration) (func(), error) {
	accessPath, key, err := resolve(path)
	if err != nil {
		return nil, err
	}
	return filelock.AcquireWithExternalTimeoutAndKey(ctx, accessPath, key, timeout)
}

func TryAcquire(path string) (func(), error) {
	return TryAcquireMode(path, ModeExclusive)
}

func TryAcquireMode(path string, mode Mode) (func(), error) {
	accessPath, key, err := resolve(path)
	if err != nil {
		return nil, err
	}
	return filelock.TryAcquireModeWithKey(accessPath, key, mode)
}

func resolve(path string) (string, string, error) {
	baseDir := ""
	if !filepath.IsAbs(strings.TrimSpace(path)) {
		var err error
		baseDir, err = os.Getwd()
		if err != nil {
			return "", "", fmt.Errorf("resolve file lock identity: %w", err)
		}
	}
	identity, err := pathidentity.Resolve(path, pathidentity.Options{BaseDir: baseDir, FollowLeaf: true})
	if err != nil {
		return "", "", fmt.Errorf("resolve file lock identity: %w", err)
	}
	return identity.AccessPath, identity.Key, nil
}
