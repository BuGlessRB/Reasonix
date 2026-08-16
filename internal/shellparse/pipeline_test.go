package shellparse

import (
	"strings"
	"testing"
)

func TestSinglePipelineStages(t *testing.T) {
	stages, ok := SinglePipelineStages("go test ./... 2>&1 | tail -5")
	if !ok {
		t.Fatal("a plain pipeline must decompose")
	}
	if len(stages) != 2 || !strings.HasPrefix(stages[0], "go test ./...") || stages[1] != "tail -5" {
		t.Fatalf("stages = %q", stages)
	}

	stages, ok = SinglePipelineStages("go test ./... | grep -v '^ok' | head -20")
	if !ok || len(stages) != 3 {
		t.Fatalf("three-stage pipeline = %q, ok=%v", stages, ok)
	}
}

// PIPESTATUS only describes the pipeline that ran last, so anything that could
// run more than one — or none — has no stage list to line statuses up with.
func TestSinglePipelineStagesRejectsAmbiguousShapes(t *testing.T) {
	for _, command := range []string{
		"go test ./...",
		"go vet ./... && go test ./... | tail -5",
		"go build ./...; go test ./... | tail",
		"go test ./... | tail -5 &",
		"! go test ./... | tail",
		"",
	} {
		if stages, ok := SinglePipelineStages(command); ok {
			t.Errorf("%q decomposed to %q, want rejected", command, stages)
		}
	}
}
