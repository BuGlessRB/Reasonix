package mcpsetup

import (
	"net/url"
	"sort"
	"strings"
)

// Redact replaces a value whose key or content looks like a credential. It is
// what any surface printing an MCP config has to run first — a server block goes
// into bug reports and screenshots far more often than it gets read once.
func Redact(key, value string) string {
	if SensitiveKey(key) || SensitiveValue(value) {
		return "<redacted>"
	}
	return value
}

// SensitiveKey reports whether a config key names a credential.
func SensitiveKey(key string) bool {
	lower := strings.ToLower(strings.TrimSpace(key))
	for _, needle := range []string{"auth", "token", "secret", "credential", "api_key", "api-key", "apikey", "cookie"} {
		if strings.Contains(lower, needle) {
			return true
		}
	}
	return false
}

// SensitiveQueryKey adds the bare "key" query parameter, which is a credential
// in a URL and an ordinary word anywhere else.
func SensitiveQueryKey(key string) bool {
	return strings.EqualFold(strings.TrimSpace(key), "key") || SensitiveKey(key)
}

// SensitiveValue reports whether a value carries a credential regardless of the
// key it is filed under.
func SensitiveValue(value string) bool {
	lower := strings.ToLower(value)
	for _, needle := range []string{"access_token", "id_token", "refresh_token", "api_key", "api-key", "apikey", "bearer "} {
		if strings.Contains(lower, needle) {
			return true
		}
	}
	return false
}

// RedactURL masks credential-bearing query parameters while keeping the endpoint
// readable — the host is the part the user needs to recognise.
func RedactURL(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return raw
	}
	u, err := url.Parse(trimmed)
	if err != nil || u == nil {
		if SensitiveValue(raw) {
			return "<redacted>"
		}
		return raw
	}
	q := u.Query()
	changed := false
	for key := range q {
		if SensitiveQueryKey(key) {
			q.Set(key, "<redacted>")
			changed = true
		}
	}
	if !changed {
		return raw
	}
	u.RawQuery = q.Encode()
	return u.String()
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
