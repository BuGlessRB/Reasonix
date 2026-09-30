package boot

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"reasonix/internal/contract/event"
	"reasonix/internal/contract/provider"
	"reasonix/internal/ext/installsource"
)

type bundledReferenceProvider struct {
	effectRecordingProvider
	reference string
}

func (*bundledReferenceProvider) Name() string { return "boot-bundled-reference" }

func (p *bundledReferenceProvider) Stream(_ context.Context, req provider.Request) (<-chan provider.Chunk, error) {
	p.mu.Lock()
	p.reqs = append(p.reqs, req)
	first := len(p.reqs) == 1
	p.mu.Unlock()
	ch := make(chan provider.Chunk, 2)
	if first {
		args, err := json.Marshal(map[string]string{"path": p.reference})
		if err != nil {
			return nil, err
		}
		ch <- provider.Chunk{Type: provider.ChunkToolCall, ToolCall: &provider.ToolCall{ID: "read-format", Name: "read_file", Arguments: string(args)}}
	} else {
		ch <- provider.Chunk{Type: provider.ChunkText, Text: "format read"}
	}
	ch <- provider.Chunk{Type: provider.ChunkDone}
	close(ch)
	return ch, nil
}

func TestEffectBundledSkillReferenceReachesProviderAfterCopyInstall(t *testing.T) {
	source := robustTempDir(t)
	if err := os.CopyFS(source, os.DirFS(filepath.Join("..", "..", "..", "examples", "release-note-kit"))); err != nil {
		t.Fatal(err)
	}
	reference := filepath.Join("skills", "release-note", "references", "format.md")
	wantReference, err := os.ReadFile(filepath.Join(source, reference))
	if err != nil {
		t.Fatal(err)
	}
	home := isolateConfigHome(t)
	t.Setenv("REASONIX_HOME", filepath.Join(home, ".reasonix"))
	workspace := robustTempDir(t)
	t.Chdir(workspace)
	writeFile(t, workspace, "reasonix.toml", `
default_model = "test-model"

[agent]
system_prompt = "BASE"

[environment]
enabled = false

[[providers]]
name = "test-model"
kind = "boot-bundled-reference"
model = "x"
`)
	approveWorkspace(t, workspace)
	installer := installsource.NewTool(installsource.Options{ProjectRoot: workspace, HomeDir: home, RequireApprovedPlan: true})
	request := map[string]any{"source": source, "kind": "plugin", "mode": "copy"}
	for _, apply := range []bool{false, true} {
		request["apply"] = apply
		args, err := json.Marshal(request)
		if err != nil {
			t.Fatal(err)
		}
		out, err := installer.Execute(t.Context(), args)
		if err != nil {
			t.Fatalf("install_source: %v", err)
		}
		var result struct {
			OK      bool   `json:"ok"`
			Applied bool   `json:"applied"`
			PlanID  string `json:"planId"`
		}
		if err := json.Unmarshal([]byte(out), &result); err != nil || !result.OK || result.Applied != apply || result.PlanID == "" {
			t.Fatalf("install_source apply=%t: %s, err=%v", apply, out, err)
		}
		request["planId"] = result.PlanID
	}
	if err := os.RemoveAll(source); err != nil {
		t.Fatal(err)
	}
	rec := &bundledReferenceProvider{}
	provider.Register("boot-bundled-reference", func(provider.Config) (provider.Provider, error) { return rec, nil })
	ctrl, err := Build(t.Context(), Options{Sink: event.Discard})
	if err != nil {
		t.Fatal(err)
	}
	defer ctrl.Close()
	skillPath := ""
	for _, sk := range ctrl.SlashSkills() {
		if sk.SlashName() == "release-note-kit:release-note" {
			skillPath = sk.Path
		}
	}
	if skillPath == "" || strings.HasPrefix(skillPath, source+string(filepath.Separator)) {
		t.Fatalf("copied skill path = %q", skillPath)
	}
	rec.reference = filepath.Join(filepath.Dir(skillPath), "references", "format.md")
	if got, err := os.ReadFile(rec.reference); err != nil || string(got) != string(wantReference) {
		t.Fatalf("copied reference = %q, err=%v", got, err)
	}
	ctrl.Submit("/release-note-kit:release-note draft the selected changes")
	deadline := time.Now().Add(30 * time.Second)
	for ctrl.Running() {
		if time.Now().After(deadline) {
			t.Fatal("bundled skill turn did not finish")
		}
		time.Sleep(time.Millisecond)
	}
	requests := rec.requests()
	if len(requests) != 2 {
		t.Fatalf("provider received %d requests, want invocation and reference result", len(requests))
	}
	for _, req := range requests {
		if strings.Contains(systemMessage(req.Messages), "Validation not verified from the selected evidence.") {
			t.Fatal("reference content leaked into the cache-stable prefix")
		}
	}
	pinned, read := false, ""
	for _, message := range requests[1].Messages {
		pinned = pinned || message.Role == provider.RoleUser && strings.Contains(message.Content, "<skill-pin name=\"release-note\">") && strings.Contains(message.Content, skillPath)
		if message.Role == provider.RoleTool && message.ToolCallID == "read-format" {
			read = message.Content
		}
	}
	if !pinned || read == "" {
		t.Fatalf("copied skill/source or reference content did not reach provider: pinned=%t, read=%q", pinned, read)
	}
	for line := range strings.SplitSeq(string(wantReference), "\n") {
		line = strings.TrimSpace(line)
		if line != "" && !strings.Contains(read, line) {
			t.Errorf("reference line missing from provider tool result: %q", line)
		}
	}
}
