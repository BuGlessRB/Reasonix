package serve

import (
	"encoding/json"
	"log/slog"
	"sync"
	"time"

	"reasonix/internal/billing"
	"reasonix/internal/event"
	"reasonix/internal/eventwire"
)

// Broadcaster is the event.Sink the controller emits to in server mode. It
// marshals each event once and fans it out to every connected SSE subscriber.
// A slow subscriber never back-pressures the agent goroutine: it loses frames
// instead. Which frames it may lose is the whole design — see droppable.
type Broadcaster struct {
	mu              sync.Mutex
	subs            map[*subscriber]struct{}
	ledger          *billing.Ledger
	displayCurrency string
}

// NewBroadcaster returns an empty Broadcaster ready to accept subscribers.
func NewBroadcaster() *Broadcaster {
	return &Broadcaster{subs: map[*subscriber]struct{}{}, ledger: billing.NewLedger()}
}

// droppable reports whether losing this frame degrades the view rather than
// breaking it. A streaming delta is superseded by the text after it and the
// full version is in /history; a prompt, a result or a turn boundary has no
// successor carrying it, and a frontend that misses one waits forever on a
// state that already changed.
func droppable(k event.Kind) bool {
	switch k {
	case event.Text, event.Reasoning, event.ToolProgress, event.CompactionProgress:
		return true
	}
	return false
}

// SetDisplayCurrency rebinds the session ledger to a stored valuation. Empty
// keeps automatic mode: a single original currency is selected and mixed
// currencies remain buckets.
func (b *Broadcaster) SetDisplayCurrency(currency string) {
	if b == nil {
		return
	}
	b.mu.Lock()
	b.displayCurrency = billing.NormalizeCurrency(currency)
	b.mu.Unlock()
}

// ResetSession clears the usage ledger for /new, /resume and /fork.
func (b *Broadcaster) ResetSession() {
	if b == nil {
		return
	}
	b.mu.Lock()
	b.ledger = billing.NewLedger()
	b.mu.Unlock()
}

// SessionCostQuote returns the current aggregate quote without repricing.
func (b *Broadcaster) SessionCostQuote() billing.CostQuote {
	if b == nil {
		return billing.AggregateQuotes(nil, "")
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.ledger == nil {
		b.ledger = billing.NewLedger()
	}
	return b.ledger.Total(b.displayCurrency)
}

// Emit marshals the event to JSON and delivers it to every subscriber. Never
// blocks: a subscriber that has fallen behind loses droppable frames. A marshal
// failure is dropped silently — one bad event shouldn't stall the stream.
func (b *Broadcaster) Emit(e event.Event) {
	data, err := json.Marshal(eventwire.ToWire(e))
	if err != nil {
		return
	}
	drop := droppable(e.Kind)
	b.mu.Lock()
	defer b.mu.Unlock()
	if e.Kind == event.Usage && e.Usage != nil && e.CostQuote != nil {
		if b.ledger == nil {
			b.ledger = billing.NewLedger()
		}
		b.ledger.Add(*e.CostQuote, billing.UsageTokens{
			PromptTokens: e.Usage.PromptTokens, CompletionTokens: e.Usage.CompletionTokens,
			CacheHitTokens: e.Usage.CacheHitTokens, CacheMissTokens: e.Usage.CacheMissTokens,
			CacheWriteTokens: e.Usage.CacheWriteTokens, CacheWriteBilledTokens: e.Usage.CacheWriteBilledTokens,
			Estimated: e.Usage.Estimated,
		}, time.Now().UTC())
	}
	for s := range b.subs {
		s.push(data, drop)
	}
}

// EmitTo delivers an event only to the supplied subscriber. It is used for
// connection-local recovery frames, such as replaying a prompt to a browser
// that attached after the original event was emitted. Normal runtime events
// should continue to use Emit so every subscriber receives them.
func (b *Broadcaster) EmitTo(target <-chan []byte, e event.Event) {
	data, err := json.Marshal(eventwire.ToWire(e))
	if err != nil {
		return
	}
	drop := droppable(e.Kind)
	b.mu.Lock()
	defer b.mu.Unlock()
	for s := range b.subs {
		if (<-chan []byte)(s.ch) != target {
			continue
		}
		s.push(data, drop)
		return
	}
}

// Subscribe registers a new SSE client and returns its channel plus an
// unsubscribe func the handler must call (defer) when the client disconnects.
func (b *Broadcaster) Subscribe() (<-chan []byte, func()) {
	s := newSubscriber()
	b.mu.Lock()
	b.subs[s] = struct{}{}
	b.mu.Unlock()
	return s.ch, func() {
		b.mu.Lock()
		_, ok := b.subs[s]
		delete(b.subs, s)
		b.mu.Unlock()
		if ok {
			s.close()
		}
	}
}

// Subscribers reports the current connection count (for diagnostics/tests).
func (b *Broadcaster) Subscribers() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.subs)
}

// softCap is where droppable frames start being shed. hardCap bounds the queue
// for the case where even the frames that matter are outrunning the client;
// losing the oldest is the least bad option left, since the newest are the ones
// carrying the state that would let the frontend stop waiting.
const (
	softCap = 64
	hardCap = 4096
)

// subscriber is one client's outbound queue. Frames land on the queue and a
// single pump goroutine moves them onto ch, which is what lets Emit stay
// non-blocking while a frame's fate depends on its kind rather than on how full
// a buffer happened to be at the moment it arrived.
type subscriber struct {
	ch   chan []byte
	done chan struct{}
	wake chan struct{}

	mu     sync.Mutex
	queue  [][]byte
	closed bool
	warned bool
}

func newSubscriber() *subscriber {
	s := &subscriber{ch: make(chan []byte, 64), done: make(chan struct{}), wake: make(chan struct{}, 1)}
	go s.pump()
	return s
}

func (s *subscriber) push(data []byte, drop bool) {
	s.mu.Lock()
	if s.closed || (drop && len(s.queue) >= softCap) {
		s.mu.Unlock()
		return
	}
	if len(s.queue) >= hardCap {
		s.queue[0] = nil
		s.queue = s.queue[1:]
		if !s.warned {
			s.warned = true
			slog.Warn("serve: event subscriber too far behind; dropping frames it needs", "queued", hardCap)
		}
	}
	s.queue = append(s.queue, data)
	s.mu.Unlock()
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

// pump owns ch: the only goroutine that sends on it, and the only one that
// closes it — so a client disconnecting mid-send cannot race a send on a closed
// channel.
func (s *subscriber) pump() {
	defer close(s.ch)
	for {
		s.mu.Lock()
		if len(s.queue) == 0 {
			s.mu.Unlock()
			select {
			case <-s.wake:
				continue
			case <-s.done:
				return
			}
		}
		data := s.queue[0]
		s.queue[0] = nil
		s.queue = s.queue[1:]
		s.mu.Unlock()
		select {
		case s.ch <- data:
		case <-s.done:
			return
		}
	}
}

func (s *subscriber) close() {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.closed = true
	s.mu.Unlock()
	close(s.done)
}
