package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"reasonix/internal/agent"
)

func takeoverViewLocallyOwned(view SessionTakeoverView) bool {
	return view.Mirrored || view.Holder == "external" || view.Holder == "other"
}

// reconcileRemoteTabReclaimOwnership keeps an ambiguous reclaim response from
// changing input authority. Only a successful, generation-fenced ownership
// probe may update the spectator pin.
func (a *App) reconcileRemoteTabReclaimOwnership(
	tabID string,
	client *http.Client,
	base, expectedPath string,
	stillCurrent func(*remoteTab) bool,
) {
	a.goRemoteTabSafe("reclaimOwnershipProbe", func() {
		probeCtx, probeCancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer probeCancel()
		view, err := takeoverOwnership(probeCtx, client, base, expectedPath)
		if err != nil {
			return
		}
		locallyOwned := takeoverViewLocallyOwned(view)
		a.remoteTabMu.Lock()
		current := a.remoteTabs[tabID]
		if !stillCurrent(current) || current.session.takenOver == locallyOwned {
			a.remoteTabMu.Unlock()
			return
		}
		current.session.takenOver = locallyOwned
		meta := remoteTabMetaLocked(current)
		a.remoteTabMu.Unlock()
		a.emitRemoteEvent("remote-tab:updated", meta)
	})
}

// remoteTabOwnershipState fences a session's return from a local writer back
// to Serve. Both fields share that lifetime: a committing reclaim sets them
// and the re-hydrated surface clears them.
//
// reclaimRevision rejects /status payloads reserved before the reclaim
// completed — they still carry the pre-reclaim takenOver=true and would
// re-pin the spectator banner after ownership returned. readyBarrierPending
// defers the re-hydration barrier while a turn is in flight, because firing
// it mid-turn bumps the frontend connection generation, orphans the
// optimistic submission, and leaves a zombie "processing" indicator beside
// the rendered reply.
type remoteTabOwnershipState struct {
	reclaimRevision     uint64
	readyBarrierPending bool
}

// remoteTabReclaimObservation is the tab state a reclaim fences against: it
// releases remoteTabMu for a long poll, then dereferences tab.
type remoteTabReclaimObservation struct {
	tab               *remoteTab
	gen               uint64
	runtimeRevision   uint64
	selectionRevision uint64
}

// observeRemoteTabForReclaim snapshots the tab a reclaim fences against. The
// tab can close or reconnect between the command-target read and this
// snapshot; either is a disconnected tab, not a nil or retired binding.
func (a *App) observeRemoteTabForReclaim(tabID string, client *http.Client) (remoteTabReclaimObservation, error) {
	a.remoteTabMu.Lock()
	defer a.remoteTabMu.Unlock()
	tab := a.remoteTabs[tabID]
	if tab == nil || tab.client != client {
		return remoteTabReclaimObservation{}, fmt.Errorf("remote tab %q is not connected", tabID)
	}
	return remoteTabReclaimObservation{
		tab: tab, gen: tab.gen,
		runtimeRevision: tab.runtime.revision, selectionRevision: tab.selectionRevision,
	}, nil
}

// ReclaimRemoteTabSession takes a mirrored session back from the local
// runtime that took it over. Serve long-polls until the local writer yields,
// so this call can outlast a normal command timeout.
func (a *App) ReclaimRemoteTabSession(tabID string) error {
	if err := a.requireRemoteExecutionProtocol(tabID); err != nil {
		return err
	}
	client, base, expectedPath, err := a.remoteTabCommandTarget(tabID)
	if err != nil {
		return err
	}
	if strings.TrimSpace(expectedPath) == "" {
		return fmt.Errorf("remote tab %q has no active session", tabID)
	}
	observed, err := a.observeRemoteTabForReclaim(tabID, client)
	if err != nil {
		return err
	}
	observedTab, observedGen := observed.tab, observed.gen
	stillCurrent := func(tab *remoteTab) bool {
		return tab != nil && tab == observedTab && tab.client == client && tab.gen == observed.gen &&
			tab.runtime.revision == observed.runtimeRevision && tab.selectionRevision == observed.selectionRevision &&
			agent.CanonicalSessionPath(tab.routing.currentPath) == agent.CanonicalSessionPath(expectedPath)
	}
	reconcileOwnership := func() { a.reconcileRemoteTabReclaimOwnership(tabID, client, base, expectedPath, stillCurrent) }
	// Short timeout: the serve caps un-mirrored reclaims at 10s and mirrored
	// ones use the writer's cooperative heartbeat (seconds, not minutes). A
	// long client-side timeout only hangs the UI button.
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	body, _ := json.Marshal(map[string]any{
		"sessionPath": expectedPath,
		"mode":        "wait",
		"timeoutMs":   15000,
	})
	resp, err := serveDo(ctx, client, http.MethodPost, serveURL(base, "/reclaim"), body)
	if err != nil {
		reconcileOwnership()
		return fmt.Errorf("reclaim session: %w", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
	if resp.StatusCode != http.StatusNoContent {
		errMsg := strings.TrimSpace(string(respBody))
		// A failed reclaim is not proof that ownership changed — generation
		// conflicts and transient 5xx included. Keep the spectator pin until a
		// fenced probe proves this exact binding is no longer locally owned.
		reconcileOwnership()
		return fmt.Errorf("reclaim session: %s", errMsg)
	}
	// Reclaim succeeded: Serve now owns the session again. Clear the spectator
	// pin immediately so the composer un-locks without waiting for the next
	// status poll to observe takenOver=false.
	observedTab.routeEventMu.Lock()
	defer observedTab.routeEventMu.Unlock()
	a.remoteTabMu.Lock()
	if tab := a.remoteTabs[tabID]; stillCurrent(tab) {
		tab.session.takenOver = false
		// Fence status payloads reserved before this reclaim: they may still
		// be in flight and carry the pre-reclaim takenOver=true, which would
		// re-pin the spectator banner the moment ownership returned.
		tab.ownership.reclaimRevision = tab.runtime.revision + 1
		deferBarrier := tab.runtime.running || tab.runtime.pendingPrompt
		tab.ownership.readyBarrierPending = deferBarrier
		meta := remoteTabMetaLocked(tab)
		a.remoteTabMu.Unlock()
		a.emitRemoteEvent("remote-tab:updated", meta)
		// The spectator era froze the projection, so publish the ready barrier
		// to re-hydrate the view and accept the re-owned writer's frames. Defer
		// it mid-turn: the barrier bumps the frontend connection generation.
		if !deferBarrier {
			a.transitionRemoteTabStateLocked(tab, observedGen, "ready", "ready", "")
		}
	} else {
		a.remoteTabMu.Unlock()
	}
	a.goRemoteTabSafe("reclaimStatusRefresh", func() { _, _ = a.RemoteTabStatus(tabID) })
	return nil
}

// remoteSessionTakenOver reports whether a session-entry refusal means the
// session is owned by a local runtime on the serve host. The tab then
// attaches as a read-only spectator instead of dying with the 409. All three
// refusal shapes match: the explicit takeover wording (mirrored session), the
// plain lease wording ("in use by another Reasonix process" — the holder is a
// local window/CLI whose transcript the file-backed /history serves anyway,
// and whose lease /reclaim can take back), and the final-format writer
// wording ("session writer is owned by another runtime" — the identity's
// writer.lock lives with a local runtime).
func remoteSessionTakenOver(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	if strings.Contains(msg, "taken over by a local Reasonix") {
		return true
	}
	if strings.Contains(msg, "writer is owned by another runtime") {
		return true
	}
	return strings.Contains(msg, "in use by another Reasonix process")
}
