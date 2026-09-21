package boot

import (
	"net/http"
	"slices"
	"strings"

	"reasonix/internal/config"
	"reasonix/internal/laya"
	"reasonix/internal/tool"
	"reasonix/internal/tool/builtin"
)

func addSystemOne(reg *tool.Registry, enabled []string, cfg *config.Config, client *http.Client) {
	if len(enabled) != 0 && !slices.Contains(enabled, "system_one") {
		return
	}
	spec := builtin.SystemOneSpec{BaseURL: cfg.Tools.SystemOne.BaseURL, Model: cfg.Tools.SystemOne.Model, APIKey: cfg.SystemOneAPIKey}
	layaConfig := cfg.Tools.SystemOne.Laya
	if layaConfig.Local {
		spec.LayaLocal = &laya.LocalClient{Python: layaConfig.Python, Model: layaConfig.Model}
	}
	if strings.TrimSpace(layaConfig.HTTPBaseURL) != "" {
		spec.LayaHTTP = &laya.HTTPClient{HTTP: client, BaseURL: layaConfig.HTTPBaseURL, APIKey: cfg.LayaHTTPAPIKey}
	}
	if !builtin.SystemOneConfigured(spec) {
		return
	}
	spec.HTTP = client
	reg.Add(builtin.NewSystemOne(spec))
}
