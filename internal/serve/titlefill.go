package serve

import (
	"context"
	"sync"
	"time"

	"reasonix/internal/nilutil"
)

// A cold title cache cost one model round-trip per session, serialized inside
// GET /sessions: 29 sessions with 17 misses took 11.7s before the list could
// render. A title is decoration on a navigation list, so the request never
// waits for one — the preview stands in and the generated title lands in the
// cache for the next read.
const (
	titleFillWorkers = 4
	titleFillTimeout = 30 * time.Second
)

type titleJob struct {
	name   string
	source string
	mod    int64
}

type titleFiller struct {
	mu      sync.Mutex
	pending map[string]struct{}
	queue   []titleJob
	active  int
}

func newTitleFiller() *titleFiller {
	return &titleFiller{pending: map[string]struct{}{}}
}

// scheduleTitle queues one generation, deduplicated by session name so a burst
// of list requests cannot stack N copies of the same call.
func (s *Server) scheduleTitle(name, source string, mod int64) {
	if nilutil.IsNil(s.titleProv) || source == "" {
		return
	}
	f := s.fill
	f.mu.Lock()
	if _, dup := f.pending[name]; dup {
		f.mu.Unlock()
		return
	}
	f.pending[name] = struct{}{}
	f.queue = append(f.queue, titleJob{name: name, source: source, mod: mod})
	spawn := f.active < titleFillWorkers
	if spawn {
		f.active++
	}
	f.mu.Unlock()
	if spawn {
		go s.drainTitles()
	}
}

func (s *Server) drainTitles() {
	f := s.fill
	for {
		f.mu.Lock()
		if len(f.queue) == 0 {
			f.active--
			f.mu.Unlock()
			return
		}
		job := f.queue[0]
		f.queue = f.queue[1:]
		f.mu.Unlock()

		ctx, cancel := context.WithTimeout(context.Background(), titleFillTimeout)
		title := s.generateTitle(ctx, job.source)
		cancel()
		if title != "" {
			s.titles.put(job.name, title, job.source, job.mod)
		}

		f.mu.Lock()
		delete(f.pending, job.name)
		f.mu.Unlock()
	}
}
