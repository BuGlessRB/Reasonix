package laya

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"reasonix/internal/typesafe"
)

func TestHTTPGatewayUsesSystemOneShape(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/systemone" || r.Header.Get("Authorization") != "Bearer gateway-secret" {
			t.Fatalf("request path=%q auth=%q", r.URL.Path, r.Header.Get("Authorization"))
		}
		_, _ = w.Write([]byte(`{"model":"multilingual","answers":{"route":{"type":"choice","choice":"billing","probabilities":{"billing":0.9,"support":0.1},"confidence":0.8}},"routing":{"model":"multilingual","reason":"non-Latin script"}}`))
	}))
	defer server.Close()
	client := HTTPClient{HTTP: server.Client(), BaseURL: server.URL, APIKey: func() string { return "gateway-secret" }}
	response, err := client.Evaluate(context.Background(), typesafe.Request{State: "x", Model: "auto", Questions: map[string]typesafe.Question{"route": {Type: "choice", Instructions: "route", Criteria: map[string]any{"billing": nil, "support": nil}}}})
	if err != nil {
		t.Fatal(err)
	}
	if response.Model != "multilingual" || len(response.Routing) == 0 {
		t.Fatalf("response = %+v", response)
	}
}
