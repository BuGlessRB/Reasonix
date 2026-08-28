package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"

	"reasonix/internal/ablation"
	"reasonix/internal/agent"
	"reasonix/internal/boot"
	"reasonix/internal/event"
	"reasonix/internal/provider"
	_ "reasonix/internal/provider/openai"
	"reasonix/internal/tool"
	_ "reasonix/internal/tool/builtin"
)

// A run is one recall question against a fixture the preflight has already
// cleared. Scoring reads the transcript the turn appended — tool names and
// their JSON arguments — so nothing depends on how the model narrated itself.

const (
	runBaseURL = "https://api.deepseek.com"
	runModel   = "deepseek-chat"
	runTimeout = 4 * time.Minute
)

func realProvider() (provider.Provider, error) {
	key := os.Getenv("DEEPSEEK_API_KEY")
	if key == "" {
		return nil, fmt.Errorf("a real run needs DEEPSEEK_API_KEY")
	}
	return provider.New("openai", provider.Config{
		Name: "contextbench", BaseURL: runBaseURL, Model: runModel, APIKey: key,
	})
}

// runOne asks one task under one arm and scores what the model did.
func runOne(p provider.Provider, t contextTask, arm ablation.Set, armName, root string, cueVisible bool) (contextMetrics, error) {
	f, err := buildFixture(t, arm, filepath.Join(root, t.ID, armName, "session.jsonl"))
	if err != nil {
		return contextMetrics{}, err
	}
	sess, err := agent.LoadSession(f.Path)
	if err != nil {
		return contextMetrics{}, fmt.Errorf("load session: %w", err)
	}
	reg := tool.NewRegistry()
	for _, x := range tool.Builtins() {
		reg.Add(x)
	}
	boot.ApplyUnifiedProviderToolSurface(reg, false, arm)

	a := agent.New(p, reg, sess, agent.Options{
		ContextWindow: fixtureWindow, CompactRatio: 0.5, RecentKeep: 2,
		SessionPath: f.Path, KeepPolicy: agent.KeepErrors, Ablation: arm,
	}, event.Discard)
	a.LoadProjectionSidecar(f.Path)

	before := len(sess.Snapshot())
	ctx, cancel := context.WithTimeout(context.Background(), runTimeout)
	defer cancel()
	runErr := a.Run(ctx, t.Prompt)

	appended := sess.Snapshot()[before:]
	m := scoreRun(appended, t, f.Target, armName, cueVisible)
	m.scoreAnswer(finalAnswer(appended), t)
	if runErr != nil {
		m.FailureStage = "RunError"
	}
	return m, runErr
}

// finalAnswer is the last assistant text the turn produced.
func finalAnswer(msgs []provider.Message) string {
	for _, m := range slices.Backward(msgs) {
		if m.Role == provider.RoleAssistant && strings.TrimSpace(m.Content) != "" {
			return m.Content
		}
	}
	return ""
}

// runExperiment runs one experiment's tasks across its arms.
func runExperiment(experiment, root string) int {
	p, err := realProvider()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	byArm := map[string][]contextMetrics{}
	var all []contextMetrics

	for _, t := range allTasks() {
		if t.Experiment != experiment {
			continue
		}
		for _, arm := range armsFor(t) {
			cueVisible := t.Experiment == experimentIndex && cueExpectedVisible(t, arm.scale)
			m, err := runOne(p, t, armFor(arm.scale, arm.searchOff), arm.name, root, cueVisible)
			if err != nil {
				fmt.Fprintf(os.Stderr, "%s [%s]: %v\n", t.ID, arm.name, err)
			}
			byArm[arm.name] = append(byArm[arm.name], m)
			all = append(all, m)
			fmt.Printf("%-24s %-14s %-18s search=%d hit=%d read=%d recall-tok=%d %v\n",
				t.ID, arm.name, m.FailureStage, m.SearchCalls, m.TargetSearchHits, m.ReadCalls,
				m.RecallReturnedTokens, m.UnexpectedWorkTools)
		}
	}
	reportFunnels(byArm)
	return writeResults(root, all)
}

func cueExpectedVisible(t contextTask, scale string) bool {
	visible, _ := tierScales(t.CueTier)
	return contains(visible, scale)
}

func reportFunnels(byArm map[string][]contextMetrics) {
	names := make([]string, 0, len(byArm))
	for name := range byArm {
		names = append(names, name)
	}
	sort.Strings(names)
	fmt.Println("\n## Funnel")
	for _, name := range names {
		fmt.Println(" ", summarize(name, byArm[name]).line())
	}
	fmt.Println("\n## Where it broke")
	for _, name := range names {
		f := summarize(name, byArm[name])
		stages := make([]string, 0, len(f.Stages))
		for stage, n := range f.Stages {
			stages = append(stages, fmt.Sprintf("%s=%d", stage, n))
		}
		sort.Strings(stages)
		fmt.Printf("  %-12s %s\n", name, strings.Join(stages, " "))
	}
}

func writeResults(root string, all []contextMetrics) int {
	path := filepath.Join(root, "results.jsonl")
	f, err := os.Create(path)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	defer f.Close()
	for _, m := range all {
		if err := writeJSONLine(f, m); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 2
		}
	}
	fmt.Printf("\n%d runs written to %s\n", len(all), path)
	return 0
}
