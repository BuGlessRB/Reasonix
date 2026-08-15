package agentpreset

import "testing"

func TestNormalizeLegacyAndUnknown(t *testing.T) {
	cases := map[string]AgentPreset{
		"":           Balanced,
		"full":       Balanced,
		"balanced":   Balanced,
		"BALANCED":   Balanced,
		"economy":    Balanced,
		"eco":        Balanced,
		"light":      Balanced,
		"lite":       Balanced,
		"delivery":   Delivery,
		"quality":    Delivery,
		"unexpected": Balanced,
	}
	for in, want := range cases {
		if got := Normalize(in); got != want {
			t.Errorf("Normalize(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestLegacyTokenModeRoundTrip(t *testing.T) {
	for _, p := range All() {
		legacy := LegacyTokenMode(p)
		if got := FromLegacyTokenMode(legacy); got != p {
			t.Errorf("FromLegacyTokenMode(%q) = %q, want %q", legacy, got, p)
		}
	}
	// The retired setting resolves rather than failing: an old config that
	// still says economy gets the default, not an error.
	if got := FromLegacyTokenMode("economy"); got != Balanced {
		t.Fatalf("economy -> %q, want balanced", got)
	}
}

func TestPolicyOfIsStablePerPreset(t *testing.T) {
	balanced := PolicyOf(Balanced)
	if balanced.ReviewPolicy.MediumRisk != ReviewConditional {
		t.Fatal("balanced medium risk should be conditional review")
	}
	if !balanced.CapabilityPolicy.SemanticRouterAllowed {
		t.Fatal("balanced should allow semantic router when needed")
	}

	delivery := PolicyOf(Delivery)
	if delivery.ReviewPolicy.MediumRisk != ReviewForced {
		t.Fatal("delivery medium risk must force independent review")
	}
	if delivery.ReviewPolicy.LowRisk != ReviewNone {
		t.Fatal("delivery low risk must not spawn independent reviewer")
	}
	if delivery.VerificationPolicy.Level != VerifyFull {
		t.Fatal("delivery must require full verification")
	}
}

func TestIsValid(t *testing.T) {
	if !IsValid("balanced") || !IsValid("delivery") {
		t.Fatal("canonical names must be valid")
	}
	// "light" joins the aliases: Normalize still answers it, but it is no
	// longer a name anything should write back out.
	if IsValid("light") || IsValid("economy") || IsValid("full") || IsValid("") {
		t.Fatal("legacy aliases must not pass IsValid")
	}
}
