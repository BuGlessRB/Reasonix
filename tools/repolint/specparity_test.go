package main

import (
	"strings"
	"testing"
)

// The two drifts this rule exists to catch, reduced to the smallest documents
// that carry them: a paragraph that never crossed, and a table row that did not.
const parityEnglish = `### 3.14 Fleet is a small dependency graph

A fleet item may declare ` + "`id`" + ` and ` + "`depends_on`" + `.

| Given to the child | Where it comes from |
| --- | --- |
| The task text | the user turn itself |
| A dependency's final answer | ` + "`<upstream-results>`" + ` |

The edge carries the dependency's answer, not only its order.
`

// findingsFor locks §3.14 alone, so a fixture may carry the one section a case
// is about rather than every section the real rule locks.
func findingsFor(en, zh string) []Finding {
	enSections, enOrder := parseSpecSections([]byte(en))
	zhSections, zhOrder := parseSpecSections([]byte(zh))
	return specParityFindings([]string{"3.14"}, enSections, enOrder, zhSections, zhOrder)
}

func TestSpecParityAcceptsAMatchingTranslation(t *testing.T) {
	zh := strings.NewReplacer(
		"Fleet is a small dependency graph", "fleet 是一张小依赖图",
		"A fleet item may declare", "fleet item 可以声明",
		"Given to the child", "交给子智能体的",
		"Where it comes from", "来源",
		"The task text", "任务文本",
		"the user turn itself", "user turn 本身",
		"A dependency's final answer", "依赖的最终答案",
		"The edge carries the dependency's answer, not only its order.",
		"边携带的是依赖的答案，不只是它的次序。",
	).Replace(parityEnglish)
	if got := findingsFor(parityEnglish, zh); len(got) != 0 {
		t.Fatalf("a faithful translation must produce no findings, got %+v", got)
	}
}

func TestSpecParityCatchesAParagraphThatNeverCrossed(t *testing.T) {
	zh := strings.Replace(parityEnglish,
		"\nThe edge carries the dependency's answer, not only its order.\n", "\n", 1)
	got := findingsFor(parityEnglish, zh)
	if len(got) != 1 {
		t.Fatalf("findings = %+v, want exactly one", got)
	}
	if !strings.Contains(got[0].Msg, "2 blocks here, 3 in") {
		t.Fatalf("message must name the block counts, got %q", got[0].Msg)
	}
	if got[0].Rule != ruleSpecParity || got[0].File != specChinese {
		t.Fatalf("finding = %+v, want the %s rule on %s", got[0], ruleSpecParity, specChinese)
	}
}

func TestSpecParityCatchesAMissingTableRow(t *testing.T) {
	zh := strings.Replace(parityEnglish,
		"| A dependency's final answer | `<upstream-results>` |\n", "", 1)
	got := findingsFor(parityEnglish, zh)
	if len(got) != 1 {
		t.Fatalf("findings = %+v, want exactly one", got)
	}
	if !strings.Contains(got[0].Msg, "table block 2 has 3 lines here and 4") {
		t.Fatalf("message must name the missing row, got %q", got[0].Msg)
	}
}

func TestSpecParityCatchesASectionOnlyOneSideHas(t *testing.T) {
	got := findingsFor(parityEnglish, "")
	if len(got) != 2 {
		t.Fatalf("findings = %+v, want a heading-sequence finding and a missing-section one", got)
	}
	if !strings.Contains(got[0].Msg, "§3.14 has a heading in") {
		t.Fatalf("heading finding = %q", got[0].Msg)
	}
	if !strings.Contains(got[1].Msg, "§3.14 is locked for translation parity but is missing here") {
		t.Fatalf("missing-section finding = %q", got[1].Msg)
	}
}

// TestSpecParityIgnoresProseWrapping is the false-positive guard: English wraps
// at 80 columns and Chinese does not, so line counts inside a paragraph must
// never be compared.
func TestSpecParityIgnoresProseWrapping(t *testing.T) {
	en := "### 3.14 x\n\nOne sentence split\nacross three\nsource lines.\n"
	zh := "### 3.14 x\n\n一句话写在一行里。\n"
	if got := findingsFor(en, zh); len(got) != 0 {
		t.Fatalf("prose wrapping must not be a finding, got %+v", got)
	}
}

// TestSpecParityHoldsTheRealTree pins the invariant the rule was written for:
// every locked section is in parity right now, so any later drift is new.
func TestSpecParityHoldsTheRealTree(t *testing.T) {
	if got := checkSpecParity("../.."); len(got) != 0 {
		for _, f := range got {
			t.Errorf("%s:%d: %s", f.File, f.Line, f.Msg)
		}
		t.Fatalf("the two SPECs have drifted in %d locked place(s)", len(got))
	}
}
