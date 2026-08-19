package billing

import "testing"

// Every catalog provider must declare at least one official endpoint, or its
// rows become unreachable: a rate card is only matched against the catalog
// after the entry's host identifies the vendor.
func TestEveryCatalogProviderHasAnOfficialEndpoint(t *testing.T) {
	declared := map[string]bool{}
	for _, host := range officialEndpointHosts {
		declared[host] = true
	}
	for _, entry := range OfficialCatalog() {
		if !declared[entry.Provider] {
			t.Errorf("catalog provider %q has no official endpoint host; its prices can never match", entry.Provider)
		}
	}
}

func TestOfficialProviderForEndpoint(t *testing.T) {
	cases := []struct {
		name    string
		baseURL string
		want    string
	}{
		{"official deepseek", "https://api.deepseek.com", "deepseek"},
		{"official deepseek anthropic path", "https://api.deepseek.com/anthropic", "deepseek"},
		{"official longcat", "https://api.longcat.chat/openai/v1", "longcat"},
		{"official mimo region", "https://token-plan-sgp.xiaomimimo.com/v1", "mimo"},
		{"uppercase host", "https://API.DeepSeek.com/v1", "deepseek"},
		{"proxy that resells the model", "https://my-deepseek-proxy.internal/v1", ""},
		{"vendor name in the path only", "https://gateway.example.com/deepseek/v1", ""},
		{"lookalike registrable domain", "https://api.deepseek.com.evil.test/v1", ""},
		{"empty", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := OfficialProviderForEndpoint(tc.baseURL); got != tc.want {
				t.Fatalf("OfficialProviderForEndpoint(%q) = %q, want %q", tc.baseURL, got, tc.want)
			}
		})
	}
}
