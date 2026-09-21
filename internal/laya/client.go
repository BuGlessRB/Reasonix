// Package laya connects Reasonix to local or self-hosted Laya runtimes.
package laya

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os/exec"
	"strings"

	"reasonix/internal/typesafe"
)

const resultMarker = "__REASONIX_LAYA_RESULT__"

type HTTPClient struct {
	HTTP    *http.Client
	BaseURL string
	APIKey  func() string
}

func (c HTTPClient) Evaluate(ctx context.Context, request typesafe.Request) (typesafe.Response, error) {
	base := strings.TrimRight(strings.TrimSpace(c.BaseURL), "/")
	endpoint, err := url.Parse(base + "/v1/systemone")
	if err != nil || endpoint.Scheme == "" || endpoint.Host == "" {
		return typesafe.Response{}, fmt.Errorf("invalid Laya gateway URL %q", base)
	}
	body, err := json.Marshal(request)
	if err != nil {
		return typesafe.Response{}, err
	}
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), bytes.NewReader(body))
	if err != nil {
		return typesafe.Response{}, err
	}
	httpRequest.Header.Set("Content-Type", "application/json")
	if c.APIKey != nil {
		if key := strings.TrimSpace(c.APIKey()); key != "" {
			httpRequest.Header.Set("Authorization", "Bearer "+key)
		}
	}
	client := c.HTTP
	if client == nil {
		client = http.DefaultClient
	}
	response, err := client.Do(httpRequest)
	if err != nil {
		return typesafe.Response{}, fmt.Errorf("call Laya gateway: %w", err)
	}
	defer response.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(response.Body, 4<<20))
	if err != nil {
		return typesafe.Response{}, err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return typesafe.Response{}, &typesafe.HTTPError{Status: response.StatusCode, Body: strings.TrimSpace(string(responseBody))}
	}
	var result typesafe.Response
	if err := json.Unmarshal(responseBody, &result); err != nil {
		return typesafe.Response{}, fmt.Errorf("decode Laya gateway response: %w", err)
	}
	return result, nil
}

type LocalClient struct {
	Python string
	Model  string
}

func (c LocalClient) Evaluate(ctx context.Context, request typesafe.Request) (typesafe.Response, error) {
	python := strings.TrimSpace(c.Python)
	if python == "" {
		python = "python"
	}
	model := strings.TrimSpace(c.Model)
	if model != "" && model != "auto" {
		request.Model = model
	}
	payload, err := json.Marshal(request)
	if err != nil {
		return typesafe.Response{}, err
	}
	cmd := exec.CommandContext(ctx, python, "-u", "-c", localBridge)
	cmd.Stdin = bytes.NewReader(payload)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return typesafe.Response{}, fmt.Errorf("run local Laya (%s): %w: %s", python, err, strings.TrimSpace(string(output)))
	}
	index := bytes.LastIndex(output, []byte(resultMarker))
	if index < 0 {
		return typesafe.Response{}, errors.New("local Laya returned no decision result")
	}
	var result typesafe.Response
	if err := json.Unmarshal(bytes.TrimSpace(output[index+len(resultMarker):]), &result); err != nil {
		return typesafe.Response{}, fmt.Errorf("decode local Laya response: %w", err)
	}
	return result, nil
}

const localBridge = `
import json, sys
from laya import Router

request = json.load(sys.stdin)
router = Router()
model = request.get("model")
if model in (None, "", "auto", "laya-auto"):
    model = None
result = router.predict(request["state"], request["questions"], model=model)
if "model" not in result:
    result["model"] = result.get("routing", {}).get("model", model or "auto")
print("__REASONIX_LAYA_RESULT__" + json.dumps(result, ensure_ascii=False))
`
