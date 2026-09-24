package agent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"
)

// The catalog reached 41 KB and spilled past the 32 KiB cap: a model that asked
// what it had was handed a pointer to a file. Descriptions were two thirds of it.
func TestTheCatalogFitsInContext(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("testdata", "capability_list.json"))
	if err != nil {
		t.Skipf("no recorded catalog to measure: %v", err)
	}
	var doc struct {
		Capabilities []struct {
			Description string `json:"description"`
		} `json:"capabilities"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	before, after := 0, 0
	for _, c := range doc.Capabilities {
		before += len(c.Description)
		after += len(capabilityLead(c.Description))
	}
	t.Logf("%d entries: description bytes %d -> %d", len(doc.Capabilities), before, after)
	if after >= before/2 {
		t.Fatalf("descriptions %d -> %d; the listing still carries most of the text", before, after)
	}
	if total := len(raw) - before + after; total > maxToolOutputBytes {
		t.Fatalf("listing would still be %d bytes, over the %d cap that spills it", total, maxToolOutputBytes)
	}
}

func TestALeadIsPickableAndWhole(t *testing.T) {
	long := "Dispatch 2-64 sub-agent tasks as a small dependency graph. Each item may select a profile, model, effort, tools, write_paths, or read_only."
	lead := capabilityLead(long)
	if lead != "Dispatch 2-64 sub-agent tasks as a small dependency graph." {
		t.Fatalf("lead = %q, want the first sentence", lead)
	}
	// No sentence break: bounded on a word, and never mid-character.
	cjk := strings.Repeat("在隔离的子代理里探索代码库", 30)
	got := capabilityLead(cjk)
	if len(got) > capabilityLeadBytes+4 {
		t.Fatalf("lead is %d bytes, over the bound", len(got))
	}
	if !utf8.ValidString(got) {
		t.Fatalf("lead cut a character in half: %q", got)
	}
	if capabilityLead("") != "" || capabilityLead("   ") != "" {
		t.Fatal("an absent description became text")
	}
	if got := capabilityLead("Short one."); got != "Short one." {
		t.Fatalf("a short description was rewritten: %q", got)
	}
}
