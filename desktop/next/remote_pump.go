package main

import (
	"bufio"
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"reasonix/internal/serve"
)

const (
	remoteRetryFloor   = time.Second
	remoteRetryCeiling = 30 * time.Second
	// Long enough for a remote kernel mid-turn to answer, short enough that a
	// link which died without an RST is noticed rather than waited on forever.
	remoteHeaderTimeout = 30 * time.Second
)

// pumpRemoteEvents forwards a proxied pane's stream onto the bus a local pane's
// frames use. The hub's proxy already streams the same frames to a page that
// can read SSE; this exists because the page inside this shell reads the bus
// for every pane, remote included — the asset server holds a response until its
// handler returns — so it retires with that bus rather than before it.
func pumpRemoteEvents(ctx context.Context, rt *serve.Runtime) {
	ep, ok := rt.Remote()
	if !ok {
		return
	}
	client := &http.Client{Transport: &http.Transport{ResponseHeaderTimeout: remoteHeaderTimeout}}
	name := runtimeEventName(rt.ID)
	// Where to resume. The remote numbers its resumable frames and honours
	// Last-Event-ID, so a link that drops mid-turn costs only what its replay
	// log has since dropped — not the rest of the turn.
	var after int64
	wait := remoteRetryFloor
	for ctx.Err() == nil {
		reached, err := streamRemoteEvents(ctx, client, ep, name, after)
		if reached > after {
			after = reached
			// Frames arrived, so whatever ended the stream was not a link that
			// cannot carry one. The next drop starts its backoff over.
			wait = remoteRetryFloor
		}
		if ctx.Err() != nil || (err == nil && reached == 0 && after == 0) {
			// A clean end with nothing ever delivered is the remote closing the
			// pane rather than a connection to retry.
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
		}
		if wait = wait * 2; wait > remoteRetryCeiling {
			wait = remoteRetryCeiling
		}
	}
}

// streamRemoteEvents reads one connection's worth of frames and returns the
// highest resumable id it saw.
func streamRemoteEvents(ctx context.Context, client *http.Client, ep serve.RemoteEndpoint, name string, after int64) (int64, error) {
	// Base names this pane's runtime over there. Without it the request lands
	// on that hub's default runtime — the pane would send its turns to one
	// conversation and listen to another, which reads as nothing happening.
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+ep.Addr+ep.Base+"/events", nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Cookie", serve.TokenCookie+"="+ep.Token)
	if after > 0 {
		req.Header.Set("Last-Event-ID", strconv.FormatInt(after, 10))
	}
	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("remote events: %s", resp.Status)
	}

	var reached int64
	reader := bufio.NewReader(resp.Body)
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return reached, err
		}
		line = strings.TrimRight(line, "\r\n")
		switch {
		case strings.HasPrefix(line, "id:"):
			if seq, err := strconv.ParseInt(strings.TrimSpace(line[3:]), 10, 64); err == nil {
				reached = seq
			}
		case strings.HasPrefix(line, "data:"):
			runtime.EventsEmit(ctx, name, strings.TrimSpace(line[5:]))
		}
	}
}
