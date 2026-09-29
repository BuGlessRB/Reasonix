package config

import "testing"

func TestSkillNameRejectsDefaultIgnorableCharacters(t *testing.T) {
	for _, name := range []string{
		"code-review\uFE0F", // variation selector
		"code\u034Freview",  // combining grapheme joiner
		"\u3164",            // Hangul filler
		"\u180Bname",        // Mongolian free variation selector
		"\u17B4name",        // Khmer inherent vowel sign
		"\U000E0101name",    // supplemental variation selector
		"\u115Fname",        // Hangul choseong filler
		"\uFFA0",            // halfwidth Hangul filler
	} {
		if IsValidSkillName(name) || SkillNameKey(name) != "" {
			t.Errorf("default-ignorable skill name %q was accepted", name)
		}
	}
	if !IsValidSkillName("中文技能") || !IsValidSkillName("Cafe\u0301") {
		t.Fatal("ordinary Unicode skill names should remain valid")
	}
}
