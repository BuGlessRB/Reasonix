// Package storage measures what the runtime keeps on disk: how much each
// declared root holds, and what the volume under it has left. It reads the root
// table rather than a list of its own, so a root declared in internal/config is
// accounted for here without this package being told about it.
package storage

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"reasonix/internal/config"
)

// Root is one root as measured: where it resolved to, whether a user may move
// it, and what it costs. Bytes and Files are what the walk actually counted —
// a surface that offers to delete things reports measured sizes, never
// estimates, because that number is what the decision is made on.
type Root struct {
	ID          config.RootID
	Dir         string
	Relocatable bool
	// PinnedBy names the environment variable holding this root, "" when none
	// does. A root that is pinned cannot be moved from the application.
	PinnedBy string
	Bytes    int64
	Files    int64
	// Missing is a root that has never been written. It is not an error: a
	// fresh install has no worktrees, and reporting zero is the honest answer.
	Missing bool
	// Err is set when the walk could not finish. Bytes and Files then describe
	// what was reachable, so a permission-denied subtree understates rather
	// than blanks the report.
	Err string
	// Volume describes the filesystem Dir sits on. Roots sharing a volume
	// report the same one, which is what lets a reader see that moving one of
	// them buys nothing.
	Volume Volume
}

// Volume is the free/total pair for the filesystem under a path, plus the
// mount point (a drive on Windows) so a reader recognises which disk it is.
type Volume struct {
	Path  string
	Free  int64
	Total int64
}

// Survey measures every declared root. It never fails as a whole: a root that
// cannot be read carries its own Err, because a single unreadable directory
// must not deny the user the numbers for the rest.
func Survey(ctx context.Context) []Root {
	ids := config.RootIDs()
	out := make([]Root, 0, len(ids))
	volumes := map[string]Volume{}
	for _, id := range ids {
		root := Root{
			ID:          id,
			Dir:         config.RootDir(id),
			Relocatable: config.RootRelocatable(id),
			PinnedBy:    config.RootPinnedBy(id),
		}
		if root.Dir == "" {
			root.Missing = true
			out = append(out, root)
			continue
		}
		root.Bytes, root.Files, root.Missing, root.Err = measureRoot(ctx, id, root.Dir)
		key := volumeKey(root.Dir)
		vol, ok := volumes[key]
		if !ok {
			vol = readVolume(root.Dir)
			volumes[key] = vol
		}
		root.Volume = vol
		out = append(out, root)
	}
	return out
}

// measureRoot sizes what a root actually owns. A root sharing its directory
// with another counts only its declared entries, or the report would credit
// state with the configuration sitting beside it.
func measureRoot(ctx context.Context, id config.RootID, dir string) (bytes, files int64, missing bool, errText string) {
	owned := config.RootOwns(id)
	if len(owned) == 0 {
		return measure(ctx, dir)
	}
	missing = true
	for _, name := range owned {
		b, f, miss, err := measure(ctx, filepath.Join(dir, name))
		bytes += b
		files += f
		if !miss {
			missing = false
		}
		if err != "" && errText == "" {
			errText = err
		}
	}
	return bytes, files, missing, errText
}

// measure walks dir. It counts what it can reach and reports the first refusal
// rather than aborting: an unreadable subtree is a smaller number plus a
// reason, which is more use than no number at all.
func measure(ctx context.Context, dir string) (bytes, files int64, missing bool, errText string) {
	// Asked before the walk rather than inferred from it: a root nothing has
	// written yet and a root that refused to be read are different answers,
	// and the walk reports both as the same first callback error.
	if _, err := os.Stat(dir); err != nil {
		if os.IsNotExist(err) {
			return 0, 0, true, ""
		}
		return 0, 0, false, err.Error()
	}
	walkErr := filepath.WalkDir(dir, func(_ string, entry fs.DirEntry, err error) error {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		if err != nil {
			if errText == "" {
				errText = err.Error()
			}
			return nil
		}
		if entry.IsDir() {
			return nil
		}
		info, infoErr := entry.Info()
		if infoErr != nil {
			if errText == "" {
				errText = infoErr.Error()
			}
			return nil
		}
		files++
		bytes += info.Size()
		return nil
	})
	if walkErr != nil && errText == "" {
		errText = walkErr.Error()
	}
	return bytes, files, false, errText
}

// volumeKey groups roots that sit on one filesystem, so the free-space probe
// runs once per disk instead of once per root.
func volumeKey(dir string) string {
	vol := filepath.VolumeName(dir)
	if vol != "" {
		return strings.ToLower(vol)
	}
	return "/"
}

// SortedByBytes orders a survey largest first, which is the order a reader
// deciding what to move or delete wants to read it in.
func SortedByBytes(roots []Root) []Root {
	out := append([]Root(nil), roots...)
	sort.SliceStable(out, func(i, j int) bool { return out[i].Bytes > out[j].Bytes })
	return out
}
