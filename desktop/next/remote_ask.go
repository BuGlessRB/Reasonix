package main

import (
	"context"
	"errors"

	"reasonix/internal/remote"
	"reasonix/internal/serve"
)

// askPrompts turns the link layer's blocking callbacks into canonical
// questions. Nothing here holds one: the broker does, and every shell finds it
// the same way — a window that is not polling yet simply has not looked.
type askPrompts struct{ asks *serve.AskBroker }

// prompts are what the pool hands the link layer. A broker that does not exist
// leaves both nil, and the strict path applies: a first-seen key is refused
// rather than accepted quietly.
func (p askPrompts) prompts() attachPrompts {
	if p.asks == nil {
		return attachPrompts{}
	}
	return attachPrompts{Secret: p.secret, HostKey: p.hostKey}
}

func (p askPrompts) hostKey(ctx context.Context, q remote.HostKeyQuestion) (bool, error) {
	answer, err := p.asks.Ask(ctx, serve.Ask{
		Kind:        "hostkey",
		Host:        q.Host,
		Address:     q.Address,
		KeyType:     q.KeyType,
		Fingerprint: q.Fingerprint,
	})
	if err != nil {
		return false, err
	}
	return answer.OK, nil
}

func (p askPrompts) secret(ctx context.Context, kind remote.SecretKind, host, identityFile string) (string, error) {
	answer, err := p.asks.Ask(ctx, serve.Ask{
		Kind:         kind.String(),
		Host:         host,
		IdentityFile: identityFile,
	})
	if err != nil {
		return "", err
	}
	if !answer.OK {
		return "", errors.New("remote: cancelled")
	}
	return answer.Text, nil
}
