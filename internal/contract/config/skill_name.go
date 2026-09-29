// skill_name.go — what counts as a skill identifier, and how one is compared.
package config

import (
	"regexp"
	"runtime"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
)

var validMCPServerName = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]{0,63}$`)

// IsValidMCPServerName keeps provider-visible tool identifiers ASCII-only.
func IsValidMCPServerName(name string) bool { return validMCPServerName.MatchString(name) }

// IsValidSkillName reports whether name is a usable skill identifier.
func IsValidSkillName(name string) bool {
	name = norm.NFC.String(name)
	if name == "" || utf8.RuneCountInString(name) > 64 {
		return false
	}
	for i, r := range name {
		if unicode.IsLetter(r) || unicode.IsNumber(r) || i > 0 && (unicode.IsMark(r) || r == '.' || r == '_' || r == '-') {
			continue
		}
		return false
	}
	return true
}

// SkillNameKey normalizes a skill identifier for config comparisons.
func SkillNameKey(name string) string {
	name = strings.TrimSpace(name)
	if !IsValidSkillName(name) {
		return ""
	}
	name = norm.NFC.String(name)
	if runtime.GOOS == "windows" {
		return strings.ToLower(name)
	}
	return name
}
