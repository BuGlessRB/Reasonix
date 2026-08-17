// Bookkeeping for servers that connect after the call that asked for them. The
// generation number is what lets a later result be recognised as stale, and the
// cancel funcs are what a Close still has to reach.
package plugin

import "context"

// registerDeferredCancel records a background connect so Close can reach it, and
// returns the generation the caller's result will be checked against. Zero means
// the host is closed and the caller must not start.
func (h *Host) registerDeferredCancel(name string, cancel context.CancelFunc) uint64 {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		cancel()
		return 0
	}
	if h.deferredCancels == nil {
		h.deferredCancels = make(map[string][]context.CancelFunc)
	}
	if h.deferredGenerations == nil {
		h.deferredGenerations = make(map[string]uint64)
	}
	generation := h.deferredGenerations[name]
	if generation == 0 {
		generation = 1
		h.deferredGenerations[name] = generation
	}
	h.deferredCancels[name] = append(h.deferredCancels[name], cancel)
	return generation
}

// beginDeferredSpawn joins the wait group Close drains, reporting false when the
// host is already closed. The Add happens under h.mu so it cannot race the Wait.
func (h *Host) beginDeferredSpawn() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		return false
	}
	h.deferredWG.Add(1)
	return true
}

func (h *Host) endDeferredSpawn() { h.deferredWG.Done() }
