package main

import "testing"

const refusalSource = `package p

import "net/http"

func h(w http.ResponseWriter) {
	http.Error(w, "no", http.StatusInternalServerError)
	refuse(w, http.StatusBadRequest, "a.b", "no", nil)
}
`

func TestRefusalPathFlagsPlainHTTPErrorInServe(t *testing.T) {
	s := parseBytes("internal/serve/x.go", []byte(refusalSource))
	got := checkRefusalPath(s)
	if len(got) != 1 {
		t.Fatalf("findings = %d, want 1: %v", len(got), got)
	}
	if got[0].Rule != ruleRefusalPath || got[0].Weight != 1 {
		t.Fatalf("a pass/fail rule must weigh one: %+v", got[0])
	}
}

// The rule is about the refusal contract one package owes its frontends, not
// about http.Error everywhere: a CLI or a worker has no Reason to send.
func TestRefusalPathIgnoresPackagesWithoutThatContract(t *testing.T) {
	for _, rel := range []string{"internal/cli/x.go", "internal/serve/x_test.go", "cmd/reasonix/main.go"} {
		if got := checkRefusalPath(parseBytes(rel, []byte(refusalSource))); len(got) != 0 {
			t.Fatalf("%s was flagged: %v", rel, got)
		}
	}
}
