package tui

import (
	"strings"
	"testing"

	"reasonix/internal/base/i18n"
	"reasonix/internal/contract/eventwire"
)

// inChinese draws with the Chinese catalogue for the rest of the test.
func inChinese(t *testing.T) {
	t.Helper()
	was := i18n.M
	i18n.M = i18n.Chinese
	t.Cleanup(func() { i18n.M = was })
}

// The menu says how to drive it every time it opens, in the reader's
// language, as 1.x's does — not only past eight rows, and not in English.
func TestMenuHintIsAlwaysShownInTheUILanguage(t *testing.T) {
	inChinese(t)
	m, _ := testModel(t)
	m.composer.SetValue("/tr")
	m.Update(completionMsg{line: "/tr", c: Completion{Kind: "slash", To: 3, Items: []CompletionItem{{Label: "/tree", Insert: "/tree"}}}})
	if got := strings.Join(m.menuLines(), "\n"); !strings.Contains(got, i18n.Chinese.CompHintSlash) {
		t.Fatalf("menu:\n%s", got)
	}
}

func TestTranscriptRowsTheKernelDoesNotWordAreTranslated(t *testing.T) {
	inChinese(t)
	for _, got := range []string{
		stallText(&eventwire.ProgressWatch{Cause: "tokens", TokenMultiple: 3, PromptTokens: 90000}),
		stallText(&eventwire.ProgressWatch{Cause: "perseveration"}),
		stallText(&eventwire.ProgressWatch{IdleRounds: 6}),
	} {
		if strings.Contains(got, "Whether to keep going") {
			t.Fatalf("stall notice is English: %q", got)
		}
	}
	m, _ := testModel(t)
	m.picker = &sessionPicker{query: "zzz"}
	if got := strings.Join(m.pickerPanel(), "\n"); !strings.Contains(got, i18n.Chinese.ResumePickNoMatch) || strings.Contains(got, "Type to filter") {
		t.Fatalf("picker:\n%s", got)
	}
}
