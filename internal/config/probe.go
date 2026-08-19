// probe.go — identify an unknown endpoint from a base URL and a key.
package config

import (
	"context"
	"fmt"
	"net/http"
	"strings"
)

// Probe is what an endpoint turned out to be. Every field is a guess a user may
// correct, which is why the UI shows them rather than applying them silently:
// an OpenAI-compatible gateway serving Claude answers the same /models call as
// one serving GPT, and only the person holding the key knows which they bought.
type Probe struct {
	Kind string // the protocol catalog kind whose listing answered
	// Kinds are every kind that listing shape may be driven with, in catalog
	// order, so a chooser offers the alternatives instead of hiding them.
	Kinds      []string
	AuthHeader bool     // anthropic-compatible gateway that wants Authorization: Bearer
	Models     []string // chat models only; audio/embedding/rerank ids are dropped
	Default    string   // suggested default — the first chat model
	Efforts    []string // /effort levels Default exposes, "auto" first; empty = none
	Effort     string   // level "auto" resolves to
	Vision     []string // models that look like they accept image input
	// Ambiguous marks a listing that more than one protocol answered. A model
	// list cannot separate an OpenAI gateway from an Anthropic one that also
	// takes Bearer, so the guess is presented as a guess instead of a fact.
	Ambiguous bool
	// NoProxy records that the endpoint answered only with the proxy bypassed —
	// a China-only host behind a foreign exit resets the handshake (#2803),
	// which nobody should be expected to recognise. Discovered, not asked.
	NoProxy bool
}

// ProbeOptions is where to look and how to get there. Both clients are
// optional; nil falls back to a plain client, which honours only the
// environment's proxy.
type ProbeOptions struct {
	BaseURL string
	APIKey  string
	// Client reaches the endpoint the way a saved provider would — through the
	// user's configured proxy.
	Client *http.Client
	// Direct is the same call with the proxy bypassed, tried only after every
	// protocol has failed through Client.
	Direct *http.Client
}

// shape is one (kind, auth) combination to try.
type shape struct {
	kind       string
	authHeader bool
}

// probeShapes are tried in full rather than first-wins: a gateway accepting
// Bearer answers both the OpenAI and the Anthropic listing, and stopping at the
// first success would silently call every such endpoint OpenAI.
var probeShapes = []shape{
	{kind: "openai"},
	{kind: "anthropic"},
	{kind: "anthropic", authHeader: true},
}

// ProbeEndpoint asks an endpoint what it is by listing its models under each
// protocol. It deliberately reuses the model-fetch path a configured provider
// uses — URL candidates, compat suffixes, auth mode — so a probe that succeeds
// proves the same call will succeed after saving.
func ProbeEndpoint(ctx context.Context, opts ProbeOptions) (Probe, error) {
	base := strings.TrimSpace(opts.BaseURL)
	if base == "" {
		return Probe{}, fmt.Errorf("probe: an endpoint address is required")
	}
	p, err := probeThrough(ctx, base, opts.APIKey, opts.Client)
	if err == nil || opts.Direct == nil {
		return p, err
	}
	// Nothing answered through the proxy. A China-only endpoint behind a
	// foreign exit fails exactly like this, so try once more without it before
	// telling the user their endpoint is wrong.
	direct, directErr := probeThrough(ctx, base, opts.APIKey, opts.Direct)
	if directErr != nil {
		return Probe{}, err
	}
	direct.NoProxy = true
	return direct, nil
}

func probeThrough(ctx context.Context, base, apiKey string, client *http.Client) (Probe, error) {
	answered := make(map[shape][]string)
	var firstErr error
	for _, s := range probeShapes {
		chat, err := listChatModels(ctx, s, base, apiKey, client)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		answered[s] = chat
	}
	if len(answered) == 0 {
		if firstErr == nil {
			firstErr = fmt.Errorf("probe: %s did not answer as an OpenAI- or Anthropic-compatible endpoint", base)
		}
		return Probe{}, firstErr
	}
	best := pick(base, answered)
	p := describe(base, best, answered[best])
	p.Ambiguous = len(answered) > 1
	return p, nil
}

// pick chooses among the protocols that answered. The path is the strongest
// evidence available — a gateway that routes Anthropic traffic says so in the
// URL, which is the same knowledge the model-fetch compat suffixes encode —
// and Claude model ids are the next best. Otherwise OpenAI, the common case.
func pick(baseURL string, answered map[shape][]string) shape {
	anthropicish := stripModelFetchCompatSuffix(strings.TrimRight(baseURL, "/")) != ""
	for _, s := range probeShapes {
		models, ok := answered[s]
		if !ok || s.kind != "anthropic" {
			continue
		}
		if anthropicish || allClaude(models) {
			return s
		}
	}
	for _, s := range probeShapes {
		if _, ok := answered[s]; ok {
			return s
		}
	}
	return probeShapes[0]
}

func allClaude(models []string) bool {
	for _, m := range models {
		if !strings.HasPrefix(strings.ToLower(strings.TrimSpace(m)), "claude") {
			return false
		}
	}
	return len(models) > 0
}

func listChatModels(ctx context.Context, s shape, baseURL, apiKey string, client *http.Client) ([]string, error) {
	entry := &ProviderEntry{
		Name:       "probe",
		Kind:       s.kind,
		BaseURL:    baseURL,
		AuthHeader: s.authHeader,
	}
	// The key is held for this call only — a probe runs before there is a
	// provider to store it against, and nothing here writes to disk.
	entry.resolvedAPIKey = strings.TrimSpace(apiKey)
	models, err := entry.FetchModelsVia(ctx, client)
	if err != nil {
		return nil, err
	}
	chat := chatModelsOf(models)
	if len(chat) == 0 {
		// The endpoint answered but offers nothing we can hold a conversation
		// with — a rerank or embedding gateway.
		return nil, fmt.Errorf("probe: %s lists %d models but none of them are chat models", baseURL, len(models))
	}
	return chat, nil
}

// describe fills in what can be told from the endpoint and its model ids.
// These are the same registries a saved provider is read through, so what the
// panel shows is what the runtime will do — which is why the probed base URL
// goes into the entry: reasoning contracts are resolved per endpoint, not per
// kind.
func describe(baseURL string, s shape, chat []string) Probe {
	p := Probe{
		Kind:       s.kind,
		Kinds:      ProtocolsDiscoveredAs(s.kind),
		AuthHeader: s.authHeader,
		Models:     chat,
		Default:    chat[0],
		Vision:     InferVisionModels(chat),
	}
	entry := &ProviderEntry{Kind: s.kind, BaseURL: baseURL, AuthHeader: s.authHeader, Model: p.Default}
	if capability := EffortCapabilityForEntry(entry); capability.Supported {
		p.Efforts, p.Effort = capability.Levels, capability.Default
	}
	return p
}

func chatModelsOf(models []string) []string {
	out := make([]string, 0, len(models))
	for _, m := range models {
		if m = strings.TrimSpace(m); m != "" && IsLikelyChatModel(m) {
			out = append(out, m)
		}
	}
	return out
}
