package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"reasonix/internal/theme"
)

// The vocabulary a theme pack writes in is the kernel's (internal/theme.Tokens
// decides what is a token and what its value may look like), while the mapping
// onto CSS variables is the frontend's. Neither half can see the other, so a
// token added on one side alone is accepted and then silently never painted —
// or painted and then rejected. This is where the two are held together.
func TestThemeTokenVocabularyMatchesTheFrontend(t *testing.T) {
	src, err := os.ReadFile(filepath.Join("..", "frontend-next", "src", "ui", "theme.ts"))
	if err != nil {
		t.Fatal(err)
	}
	block, ok := surfaceBlock(string(src))
	if !ok {
		t.Fatal("theme.ts no longer declares a SURFACE map; this test cannot see the mapping")
	}
	mapped := map[string]bool{}
	for _, m := range regexp.MustCompile(`(?m)^\s+(\w+):\s*\[`).FindAllStringSubmatch(block, -1) {
		mapped[m[1]] = true
	}
	for _, name := range theme.TokenNames() {
		if !mapped[name] {
			t.Errorf("kernel accepts token %q but theme.ts maps it to nothing, so a pack setting it changes nothing", name)
		}
		delete(mapped, name)
	}
	for name := range mapped {
		t.Errorf("theme.ts maps %q but the kernel drops it as unknown, so a pack setting it is warned about and ignored", name)
	}
}

func surfaceBlock(src string) (string, bool) {
	start := strings.Index(src, "const SURFACE")
	if start < 0 {
		return "", false
	}
	end := strings.Index(src[start:], "\n};")
	if end < 0 {
		return "", false
	}
	return src[start : start+end], true
}

// The meanings a pack cannot take. A theme that could recolour these would let
// a failure render as success, so the list is pinned on both sides: the kernel
// refuses them by omission from its vocabulary, the frontend by never mapping
// them, and this says the frontend still remembers why.
func TestReservedMeaningsAreNotThemeTokens(t *testing.T) {
	for _, reserved := range []string{"ok", "warn", "err", "net", "deleg", "add", "del", "focus"} {
		if _, taken := theme.Tokens[reserved]; taken {
			t.Errorf("%q became a theme token; a pack can now recolour a meaning", reserved)
		}
	}
}
