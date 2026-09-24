package agent

import (
	"context"
	"maps"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"

	"reasonix/internal/base/fileutil"
)

// scanReaders bounds how many directories are read at once. The walk is
// dominated by directory reads, which overlap well on every filesystem Studio
// runs on; past a few dozen they only queue in the kernel.
const scanReaders = 16

// scanWorkspaceTo answers what a sequential filepath.WalkDir would: links are
// not entered, VCS stores below the root are skipped, an unreadable entry or a
// stop leaves it incomplete, and more than limit files put it over the limit.
// The limit is an argument so a test reaches that answer with a small tree.
func scanWorkspaceTo(ctx context.Context, root string, limit int) workspaceScan {
	if root == "" {
		return workspaceScan{}
	}
	info, err := os.Lstat(root)
	if err != nil {
		return workspaceScan{state: map[string]pathState{}}
	}
	if !info.IsDir() {
		state := map[string]pathState{}
		if limit > 0 {
			state[root] = pathState{exists: true, size: info.Size(), modTime: info.ModTime().UnixNano()}
		}
		return workspaceScan{state: state, complete: limit > 0, overLimit: limit <= 0}
	}
	w := &scanWalk{ctx: ctx, limit: int64(limit), state: make(map[string]pathState, 4096), readers: make(chan struct{}, scanReaders)}
	w.wg.Add(1)
	go w.dir(root)
	w.wg.Wait()
	return workspaceScan{state: w.state, complete: !w.short.Load(), overLimit: w.over.Load()}
}

type scanWalk struct {
	ctx     context.Context
	limit   int64
	files   atomic.Int64
	short   atomic.Bool // the walk stopped or skipped something
	over    atomic.Bool
	readers chan struct{}
	wg      sync.WaitGroup
	mu      sync.Mutex
	state   map[string]pathState
}

func (w *scanWalk) stopped() bool {
	if w.over.Load() {
		return true
	}
	if w.ctx.Err() != nil {
		w.short.Store(true)
		return true
	}
	return false
}

func (w *scanWalk) dir(path string) {
	defer w.wg.Done()
	if w.stopped() {
		return
	}
	w.readers <- struct{}{}
	entries, err := os.ReadDir(path)
	<-w.readers
	if err != nil {
		w.short.Store(true)
	}
	local := make(map[string]pathState, len(entries))
	for i, e := range entries {
		if i%scanCancelCheckEvery == scanCancelCheckEvery-1 && w.stopped() {
			return
		}
		full := filepath.Join(path, e.Name())
		if e.IsDir() {
			if !fileutil.IsVCSStoreDir(e.Name()) {
				w.wg.Add(1)
				go w.dir(full)
			}
			continue
		}
		if w.files.Add(1) > w.limit {
			w.over.Store(true)
			w.short.Store(true)
			return
		}
		info, err := e.Info()
		if err != nil {
			w.short.Store(true)
			continue
		}
		local[full] = pathState{exists: true, size: info.Size(), modTime: info.ModTime().UnixNano()}
	}
	w.mu.Lock()
	maps.Copy(w.state, local)
	w.mu.Unlock()
}
