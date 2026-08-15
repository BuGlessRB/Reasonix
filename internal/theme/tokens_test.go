package theme

import "testing"

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
