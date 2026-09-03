package agent

import (
	"slices"
	"testing"

	"reasonix/internal/provider"
)

func msgs(texts ...string) []provider.Message {
	out := make([]provider.Message, len(texts))
	for i, t := range texts {
		out[i] = provider.Message{Role: provider.RoleUser, Content: t}
	}
	return out
}

func shapeWith(body []provider.Message) PrefixShape {
	s := CaptureShape("sys", nil, 0)
	s.BodyChain = BodyChain(body)
	return s
}

// The prefix hashes cover the system prompt and the tools, so a miss on an
// unchanged prefix used to have no attribution at all. An append leaves every
// carried message where it was: that is the provider's miss, and it says so.
func TestAnAppendLeavesTheCarriedBodyUnchanged(t *testing.T) {
	prev := shapeWith(msgs("a", "b"))
	cur := shapeWith(msgs("a", "b", "c"))
	d := CompareShape(prev, cur, nil, nil)
	if d.BodyChanged {
		t.Fatal("an append reported the carried body as changed")
	}
	if d.CarriedMessages != 2 {
		t.Fatalf("CarriedMessages = %d, want 2", d.CarriedMessages)
	}
	if d.PrefixChanged {
		t.Fatalf("PrefixChanged = true, reasons %v", d.PrefixChangeReasons)
	}
}

// A message rewritten behind the boundary moves every chain entry after it, so
// the host can say the miss was its own doing rather than the endpoint's.
func TestARewriteBehindTheBoundaryIsObserved(t *testing.T) {
	prev := shapeWith(msgs("a", "b"))
	cur := shapeWith(msgs("a", "rewritten", "c"))
	d := CompareShape(prev, cur, nil, nil)
	if !d.BodyChanged {
		t.Fatal("a rewritten carried message was not observed")
	}
	if d.BodyHash == "" {
		t.Fatal("no body hash to compare against next round")
	}
	if !d.PrefixChanged {
		t.Fatal("an observed body change left the round looking clean")
	}
}

// A rewrite the session declared already has a name. The observation exists for
// the one nothing declared, so only that case mints its own reason.
func TestOnlyAnUndeclaredRewriteMintsItsOwnReason(t *testing.T) {
	prev := shapeWith(msgs("a", "b"))
	cur := shapeWith(msgs("a", "folded"))

	silent := CompareShape(prev, cur, nil, nil)
	if !slices.Contains(silent.PrefixChangeReasons, "body_unreported") {
		t.Fatalf("reasons = %v, want body_unreported", silent.PrefixChangeReasons)
	}

	declared := CompareShape(prev, cur, nil, []string{"compact_auto"})
	if slices.Contains(declared.PrefixChangeReasons, "body_unreported") {
		t.Fatalf("reasons = %v; a declared rewrite must not be reported twice", declared.PrefixChangeReasons)
	}
	if !declared.BodyChanged {
		t.Fatal("a declared rewrite still changed the body and must say so")
	}
}

// The first round of a session has nothing to compare against.
func TestNothingCarriedIsNotAChange(t *testing.T) {
	d := CompareShape(PrefixShape{}, shapeWith(msgs("a")), nil, nil)
	if d.BodyChanged || d.CarriedMessages != 0 || d.BodyHash != "" {
		t.Fatalf("first round reported carried=%d changed=%v hash=%q", d.CarriedMessages, d.BodyChanged, d.BodyHash)
	}
}
