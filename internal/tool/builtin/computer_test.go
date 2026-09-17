package builtin

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"reasonix/internal/computer"
)

func TestComputerToolsWithoutAHelperSayItIsUnavailable(t *testing.T) {
	ctx := context.Background()
	if _, err := (computerRead{}).Execute(ctx, json.RawMessage(`{"what":"apps"}`)); computer.CodeOf(err) != computer.CodeUnavailable {
		t.Fatalf("computer_read unbound = %v, want %s", err, computer.CodeUnavailable)
	}
	if _, err := (computerAct{}).Execute(ctx, json.RawMessage(`{"app":"com.apple.Notes","steps":[{"action":"key","key":"Enter"}]}`)); computer.CodeOf(err) != computer.CodeUnavailable {
		t.Fatalf("computer_act unbound = %v, want %s", err, computer.CodeUnavailable)
	}
	for _, tool := range ComputerTools(nil) {
		if ComputerBound(tool) {
			t.Errorf("%s reports a helper it does not have", tool.Name())
		}
	}
}

func TestApplicationListMarksTheOnesNeverOperated(t *testing.T) {
	out := renderApps([]computer.App{
		{Bundle: "com.apple.Notes", Name: "Notes", Active: true, Windows: []computer.Window{{Title: "", Bounds: computer.Rect{Width: 800, Height: 600}}}},
		{Bundle: "com.apple.Terminal", Name: "Terminal"},
	})
	if !strings.Contains(out, "* com.apple.Notes — Notes\n") || !strings.Contains(out, `window "(untitled)" 800×600`) {
		t.Fatalf("listing = %q", out)
	}
	if !strings.Contains(out, "com.apple.Terminal — Terminal (never operated: a terminal)") {
		t.Fatalf("a refused application is not marked: %q", out)
	}
}
