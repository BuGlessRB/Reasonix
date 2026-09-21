package builtin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"reasonix/internal/laya"
	"reasonix/internal/tool"
	"reasonix/internal/typesafe"
)

type SystemOneSpec struct {
	HTTP      *http.Client
	BaseURL   string
	Model     string
	APIKey    func() string
	LayaLocal *laya.LocalClient
	LayaHTTP  *laya.HTTPClient
}

type systemOne struct{ spec SystemOneSpec }

func NewSystemOne(spec SystemOneSpec) tool.Tool { return systemOne{spec: spec} }

func SystemOneConfigured(spec SystemOneSpec) bool {
	return spec.LayaLocal != nil || spec.LayaHTTP != nil || spec.APIKey != nil && strings.TrimSpace(spec.APIKey()) != ""
}

func (systemOne) Name() string { return "system_one" }

func (systemOne) Description() string {
	return "Evaluate state with a configured System One decision backend: TypeSafe, local Laya, or a self-hosted Laya HTTP gateway. Ask typed Noul, Choice, or Score questions and receive complete probabilities, confidence, routing metadata, and usage."
}

func (systemOne) Schema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"backend":{"type":"string","enum":["typesafe","laya-local","laya-http"],"description":"Configured decision backend; optional only when exactly one is available"},"state":{"description":"Text or structured JSON state to evaluate"},"questions":{"type":"object","description":"Named questions; answer keys match these ids","additionalProperties":{"type":"object","properties":{"type":{"type":"string","enum":["noul","choice","score"]},"instructions":{"description":"Question text or structured instructions"},"criteria":{"description":"Noul: optional true/false object; Choice: 2-255 option map; Score: ordered array of 2-10 levels"}},"required":["type","instructions"],"additionalProperties":false}}},"required":["state","questions"],"additionalProperties":false}`)
}

func (systemOne) ReadOnly() bool     { return true }
func (systemOne) PlanModeSafe() bool { return true }
func (systemOne) SnipHint() tool.SnipHint {
	return tool.SnipHint{Head: 120, Tail: 8, HeadChars: 12000, TailChars: 1000}
}

func (s systemOne) ProviderVisible(context.Context) bool {
	return SystemOneConfigured(s.spec)
}

func (systemOne) Unavailable(context.Context) tool.Refusal {
	return tool.Refusal{Code: "typesafe.not_configured", Message: "system_one requires a TypeSafe API key"}
}

func (s systemOne) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var input struct {
		Backend   string                       `json:"backend"`
		State     any                          `json:"state"`
		Questions map[string]typesafe.Question `json:"questions"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return "", err
	}
	if !s.ProviderVisible(ctx) {
		return "", errors.New("system_one requires TYPESAFE_API_KEY or tools.system_one.api_key_env")
	}
	backend, err := s.backend(input.Backend)
	if err != nil {
		return "", err
	}
	request := typesafe.Request{State: input.State, Questions: input.Questions}
	var response typesafe.Response
	switch backend {
	case "laya-local":
		request.Model = s.spec.LayaLocal.Model
		response, err = s.spec.LayaLocal.Evaluate(ctx, request)
	case "laya-http":
		request.Model = "auto"
		response, err = s.spec.LayaHTTP.Evaluate(ctx, request)
	default:
		request.Model = strings.TrimSpace(s.spec.Model)
		if request.Model == "" {
			request.Model = "jev-latest"
		}
		response, err = (typesafe.Client{HTTP: s.spec.HTTP, BaseURL: s.spec.BaseURL, APIKey: s.spec.APIKey}).Evaluate(ctx, request)
	}
	if err != nil {
		return "", err
	}
	out, err := json.Marshal(response)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

func (s systemOne) backend(requested string) (string, error) {
	available := make([]string, 0, 3)
	if s.spec.APIKey != nil && strings.TrimSpace(s.spec.APIKey()) != "" {
		available = append(available, "typesafe")
	}
	if s.spec.LayaLocal != nil {
		available = append(available, "laya-local")
	}
	if s.spec.LayaHTTP != nil {
		available = append(available, "laya-http")
	}
	requested = strings.TrimSpace(requested)
	if requested == "" && len(available) == 1 {
		return available[0], nil
	}
	for _, backend := range available {
		if backend == requested {
			return backend, nil
		}
	}
	if requested == "" {
		return "", fmt.Errorf("system_one backend is required; available: %s", strings.Join(available, ", "))
	}
	return "", fmt.Errorf("system_one backend %q is not configured; available: %s", requested, strings.Join(available, ", "))
}
