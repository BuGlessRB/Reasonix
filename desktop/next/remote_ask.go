package main

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"reasonix/internal/remote"
)

// The bus channel a question travels on. Must match WAILS_REMOTE_ASK in
// desktop/frontend-next/src/port/sse.ts.
const wailsRemoteAsk = "reasonix:remote-ask"

// askPatience bounds a question nobody answers. The link is blocked while it
// waits, and a window that was closed mid-connect would otherwise hold the
// dial open until the process ends.
const askPatience = 3 * time.Minute

// RemoteAsk is a question the link layer must have answered before it can go
// on. It reaches the window as an event and the answer comes back through a
// binding, because unlike everything else here the caller is blocked on it.
type RemoteAsk struct {
	ID   string `json:"id"`
	Kind string `json:"kind"` // hostkey | passphrase | password
	Host string `json:"host"`
	// Set for hostkey: what the machine presented, for the person to compare
	// against what they were told it should be.
	Address     string `json:"address,omitempty"`
	KeyType     string `json:"keyType,omitempty"`
	Fingerprint string `json:"fingerprint,omitempty"`
	// Set for passphrase: which key file is locked.
	IdentityFile string `json:"identityFile,omitempty"`
}

type remoteAnswer struct {
	ok   bool
	text string
}

// askBroker turns the link layer's blocking callbacks into a question on the
// bus and an answer from a binding.
type askBroker struct {
	shell *App

	mu      sync.Mutex
	seq     int
	waiting map[string]chan remoteAnswer
}

func newAskBroker(shell *App) *askBroker {
	return &askBroker{shell: shell, waiting: map[string]chan remoteAnswer{}}
}

// prompts are what the pool hands the link layer. Without a window there is
// nobody to ask, so both stay nil and the strict path applies: a first-seen key
// is refused rather than accepted quietly.
func (b *askBroker) prompts() attachPrompts {
	return attachPrompts{Secret: b.askSecret, HostKey: b.askHostKey}
}

func (b *askBroker) askHostKey(ctx context.Context, q remote.HostKeyQuestion) (bool, error) {
	answer, err := b.ask(ctx, RemoteAsk{
		Kind:        "hostkey",
		Host:        q.Host,
		Address:     q.Address,
		KeyType:     q.KeyType,
		Fingerprint: q.Fingerprint,
	})
	if err != nil {
		return false, err
	}
	return answer.ok, nil
}

func (b *askBroker) askSecret(ctx context.Context, kind remote.SecretKind, host, identityFile string) (string, error) {
	answer, err := b.ask(ctx, RemoteAsk{
		Kind:         kind.String(),
		Host:         host,
		IdentityFile: identityFile,
	})
	if err != nil {
		return "", err
	}
	if !answer.ok {
		return "", errors.New("remote: cancelled")
	}
	return answer.text, nil
}

func (b *askBroker) ask(ctx context.Context, q RemoteAsk) (remoteAnswer, error) {
	wctx := b.shell.window()
	if wctx == nil {
		// No window to ask. Refusing is the safe direction: the alternative is
		// accepting a key nobody looked at.
		return remoteAnswer{}, errors.New("remote: no window to ask")
	}
	reply := make(chan remoteAnswer, 1)
	b.mu.Lock()
	b.seq++
	q.ID = fmt.Sprintf("ask%d", b.seq)
	b.waiting[q.ID] = reply
	b.mu.Unlock()
	defer b.forget(q.ID)

	runtime.EventsEmit(wctx, wailsRemoteAsk, q)
	select {
	case answer := <-reply:
		return answer, nil
	case <-ctx.Done():
		return remoteAnswer{}, ctx.Err()
	case <-time.After(askPatience):
		return remoteAnswer{}, errors.New("remote: nobody answered")
	}
}

func (b *askBroker) forget(id string) {
	b.mu.Lock()
	delete(b.waiting, id)
	b.mu.Unlock()
}

// AnswerRemote is how the window replies. Answering an id twice, or one that
// already timed out, is a no-op rather than an error: a dialog can be closed
// by both a click and a teardown.
func (a *App) AnswerRemote(id string, ok bool, text string) {
	if a.asks == nil {
		return
	}
	a.asks.mu.Lock()
	reply := a.asks.waiting[id]
	delete(a.asks.waiting, id)
	a.asks.mu.Unlock()
	if reply != nil {
		reply <- remoteAnswer{ok: ok, text: text}
	}
}

// window returns the Wails context, or nil before the window comes up.
func (a *App) window() context.Context {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.ctx
}
