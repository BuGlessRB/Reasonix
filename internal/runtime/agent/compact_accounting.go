package agent

import (
	"context"
	"sync"

	"reasonix/internal/contract/provider"
)

// CompactionUsage is what one maintenance transaction spent with the provider:
// every summarizer call it made, including the ones whose answer was thrown
// away. It is not the size of the context that was worked on — a receipt's
// InputTokens is that, and the two are routinely orders apart.
type CompactionUsage struct {
	// Calls is billed summarizer calls. A call the provider never charged for
	// is not one, which is what makes this comparable to the usage ledger.
	Calls int `json:"calls,omitempty"`
	// RequestAttempts is the provider requests those calls represent: a call
	// the provider retried internally is one call and several requests.
	RequestAttempts  int `json:"request_attempts,omitempty"`
	InputTokens      int `json:"input_tokens,omitempty"`
	OutputTokens     int `json:"output_tokens,omitempty"`
	ReasoningTokens  int `json:"reasoning_tokens,omitempty"`
	CacheHitTokens   int `json:"cache_hit_tokens,omitempty"`
	CacheMissTokens  int `json:"cache_miss_tokens,omitempty"`
	CacheWriteTokens int `json:"cache_write_tokens,omitempty"`
}

type compactionSpendKey struct{}

// compactionSpend accumulates a transaction's bill. It is reached through the
// context rather than passed down because the alternative is every call site
// remembering to return its usage, and the paths that lost usage were the ones
// that discarded a result: a discarded answer is still a charge.
type compactionSpend struct {
	mu    sync.Mutex
	total CompactionUsage
}

// withCompactionSpend opens a transaction. A nested call keeps the outer one so
// the whole compaction lands in a single bill.
func withCompactionSpend(ctx context.Context) (context.Context, *compactionSpend) {
	if ctx == nil {
		ctx = context.Background()
	}
	if spend, _ := ctx.Value(compactionSpendKey{}).(*compactionSpend); spend != nil {
		return ctx, spend
	}
	spend := &compactionSpend{}
	return context.WithValue(ctx, compactionSpendKey{}, spend), spend
}

func compactionSpendFrom(ctx context.Context) *compactionSpend {
	if ctx == nil {
		return nil
	}
	spend, _ := ctx.Value(compactionSpendKey{}).(*compactionSpend)
	return spend
}

// record adds one billed summarizer call. Called where the usage event is
// emitted, so what a transaction reports and what the usage ledger receives
// cannot come apart.
func (s *compactionSpend) record(u *provider.Usage) {
	if s == nil || u == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.total.Calls++
	s.total.RequestAttempts += max(1, u.RequestCount)
	s.total.InputTokens += u.PromptTokens
	s.total.OutputTokens += u.CompletionTokens
	s.total.ReasoningTokens += u.ReasoningTokens
	s.total.CacheHitTokens += u.CacheHitTokens
	s.total.CacheMissTokens += u.CacheMissTokens
	s.total.CacheWriteTokens += u.CacheWriteTokens
}

func (s *compactionSpend) read() CompactionUsage {
	if s == nil {
		return CompactionUsage{}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.total
}
