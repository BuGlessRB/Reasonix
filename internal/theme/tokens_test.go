package theme

import (
	"fmt"
	"math"
	"strings"
	"testing"
)

// A value lands inside a stylesheet, so the question is not "is this sensible"
// but "can this leave the property it was written into". Everything that could
// close a declaration, open a function, or fetch a URL has to be refused.
func TestTokenValuesCannotEscapeTheirDeclaration(t *testing.T) {
	escapes := []struct{ name, value string }{
		{"bg", "red; background-image: url(https://evil.example/x.png)"},
		{"bg", "#fff; }"},
		{"bg", "var(--err)"},
		{"radiusSm", "8px; position: fixed"},
		{"radiusSm", "calc(100% - 2px)"},
		{"radiusSm", "var(--r-md)"},
		{"fontUi", `Arial; } body { display: none`},
		{"fontUi", `local("x"), url(https://evil.example/f.woff2)`},
		{"fontUi", "Arial\\3b color:red"},
	}
	for _, esc := range escapes {
		if validToken(esc.name, esc.value) {
			t.Errorf("validToken(%q, %q) accepted a value that can leave its declaration", esc.name, esc.value)
		}
	}
}

func TestTokenValuesAcceptWhatAPackActuallyWrites(t *testing.T) {
	ok := []struct{ name, value string }{
		{"bg", "#0F0D0B"},
		{"accent", "#d89b5a"},
		{"accentFg", "#fff"},
		{"bgSoft", "#11223344"},
		{"radiusXs", "3px"},
		{"radiusMd", "0.5rem"},
		{"radiusSm", "0"},
		{"fontUi", `-apple-system, "Segoe UI", sans-serif`},
		{"fontMono", `ui-monospace, "SF Mono", Menlo, monospace`},
		{"fontUi", "苹方, PingFang SC, sans-serif"},
	}
	for _, entry := range ok {
		if !validToken(entry.name, entry.value) {
			t.Errorf("validToken(%q, %q) refused a value a pack would reasonably write", entry.name, entry.value)
		}
	}
}

// A radius is a size, and a size has a ceiling: past it the value is not a
// rounder card, it is a different shape.
func TestLengthHasACeiling(t *testing.T) {
	if validToken("radiusMd", "999px") {
		t.Error("a 999px radius was accepted; that is a pill, not a rounded corner")
	}
	if !validToken("radiusMd", "64px") {
		t.Error("the documented ceiling was refused")
	}
}

// A name outside the vocabulary is refused whatever its value looks like: that
// is what stops a pack from reaching a variable the frontend never meant to
// hand over, including the ones that carry meaning.
func TestUnknownAndReservedNamesAreRefused(t *testing.T) {
	for _, name := range []string{"ok", "err", "radiusPill", "bgColor", ""} {
		if validToken(name, "#ffffff") {
			t.Errorf("validToken accepted %q, which is not in the vocabulary", name)
		}
	}
}

// The packs that ship are held to the vocabulary they document. Introducing
// this check found four dead tokens in every one of them — chat/sidebar/
// workspace/workspaceFiles, named for regions of an editor this frontend does
// not have — which had been silently dropped since the packs were ported.
func TestShippedPacksUseOnlyRealTokens(t *testing.T) {
	packs := listBuiltin()
	if len(packs) == 0 {
		t.Fatal("no packs ship; the embed is broken")
	}
	for _, pack := range packs {
		if len(pack.Warnings) > 0 {
			t.Errorf("%s: %v", pack.ID, pack.Warnings)
		}
	}
}

// The pack still loads when one value is wrong, and says which one. Dropping
// the whole pack would cost the author every good token for one typo; dropping
// the token silently would leave them with no way to find it.
func TestDecodeKeepsGoodTokensAndReportsBadOnes(t *testing.T) {
	pack, err := decode([]byte(`{
      "schemaVersion": 1,
      "name": "Partial",
      "tokens": {
        "light": {"bg": "#ffffff", "fg": "#000000", "radiusSm": "huge", "glow": "#ff0000"},
        "dark":  {"bg": "#000000", "fg": "#ffffff"}
      }
    }`), "partial")
	if err != nil {
		t.Fatal(err)
	}
	if got := pack.Tokens["light"]["bg"]; got != "#ffffff" {
		t.Fatalf("good token was lost: %q", got)
	}
	if _, present := pack.Tokens["light"]["radiusSm"]; present {
		t.Error("an invalid length survived into the pack")
	}
	if len(pack.Warnings) != 2 {
		t.Fatalf("warnings = %v, want one for the bad value and one for the unknown name", pack.Warnings)
	}
}

// Contrast is a property of a pair, so it is the one thing the token validator
// cannot see: every colour in a pack is a legal value on its own. What we ship
// is held to AA on the surfaces its text lands on, because the default
// appearance already is — below that line a pack does not read as themed, it
// reads as a window that has gone wrong.
func TestShippedPacksMeetTextContrast(t *testing.T) {
	const aa = 4.5
	inks := []string{"fg", "fgDim", "fgFaint"}
	surfaces := []string{"bg", "bgSoft", "panel", "bgElev"}
	packs := listBuiltin()
	if len(packs) == 0 {
		t.Fatal("no packs ship; the embed is broken")
	}
	for _, pack := range packs {
		for _, scheme := range []string{"light", "dark"} {
			tokens := pack.Tokens[scheme]
			for _, ink := range inks {
				for _, surface := range surfaces {
					fg, ok := tokens[ink]
					bg, ok2 := tokens[surface]
					if !ok || !ok2 {
						continue
					}
					if got := contrastRatio(t, fg, bg); got < aa {
						t.Errorf("%s %s: %s %s on %s %s is %.2f, below AA %.2f",
							pack.ID, scheme, ink, fg, surface, bg, got, aa)
					}
				}
			}
		}
	}
}

func contrastRatio(t *testing.T, a, b string) float64 {
	t.Helper()
	la, lb := relativeLuminance(t, a), relativeLuminance(t, b)
	if la < lb {
		la, lb = lb, la
	}
	return (la + 0.05) / (lb + 0.05)
}

func relativeLuminance(t *testing.T, hex string) float64 {
	t.Helper()
	var r, g, b uint8
	if _, err := fmt.Sscanf(strings.TrimPrefix(hex, "#"), "%02x%02x%02x", &r, &g, &b); err != nil {
		t.Fatalf("%q is not a six-digit hex colour: %v", hex, err)
	}
	lin := func(v uint8) float64 {
		c := float64(v) / 255
		if c <= 0.04045 {
			return c / 12.92
		}
		return math.Pow((c+0.055)/1.055, 2.4)
	}
	return 0.2126*lin(r) + 0.7152*lin(g) + 0.0722*lin(b)
}
