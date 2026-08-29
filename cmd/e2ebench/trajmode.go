package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// runTrajMode re-digests already-recorded trajectory files, so digest changes
// can be evaluated against past runs without re-spending provider tokens.
// Wall-clock startup is unknown offline, so the decomposition omits it.
func runTrajMode(dir string) (string, error) {
	paths, err := filepath.Glob(filepath.Join(dir, "*.trajectory.jsonl"))
	if err != nil {
		return "", err
	}
	if len(paths) == 0 {
		return "", fmt.Errorf("no *.trajectory.jsonl files under %s", dir)
	}
	sort.Strings(paths)

	var b strings.Builder
	var results []result
	for _, p := range paths {
		s, err := summarizeTrajectory(p)
		if err != nil {
			return "", fmt.Errorf("%s: %w", p, err)
		}
		id := strings.TrimSuffix(filepath.Base(p), ".trajectory.jsonl")
		results = append(results, result{task: task{ID: id}, Trajectory: s})
		enc, err := json.MarshalIndent(s, "", "  ")
		if err != nil {
			return "", err
		}
		fmt.Fprintf(&b, "### `%s`\n\n```json\n%s\n```\n\n", id, enc)
	}
	return "## Trajectory digest\n\n" + renderTimeAttribution(results) + renderToolSurface(results) + renderRefusals(results) + renderOutcomeProgress(results) + renderCognition(results) + renderMechanismLedger(results) + b.String(), nil
}

func emitTrajMode(dir, outMD string) {
	report, err := runTrajMode(dir)
	if err != nil {
		fmt.Fprintln(os.Stderr, "traj mode:", err)
		os.Exit(1)
	}
	emit(report, outMD, "")
}

// offlineModeArgs is what a mode that reads recorded runs needs; every one of
// them re-digests what a suite run already wrote instead of spending tokens.
type offlineModeArgs struct {
	trajDir  string
	suite    string
	reportIn string
	outMD    string
	addr     string
}

// dispatchOfflineMode runs mode if it is one of the offline ones and reports
// whether it handled it, so main keeps flag parsing and nothing else.
func dispatchOfflineMode(mode string, a offlineModeArgs) bool {
	fail := func(name string, err error) {
		if err != nil {
			fmt.Fprintln(os.Stderr, name+" mode:", err)
			os.Exit(1)
		}
	}
	switch mode {
	case "compare":
		runCompareMode(a.outMD)
	case "traj":
		emitTrajMode(a.trajDir, a.outMD)
	case "barrier":
		report, err := runBarrierMode(a.trajDir)
		fail("barrier", err)
		emit(report, a.outMD, "")
	case "sft":
		fail("sft", runSFTMode(a.trajDir, a.suite, a.reportIn, a.outMD))
	case "serve":
		fail("serve", runServeMode(a.trajDir, a.suite, a.addr))
	default:
		return false
	}
	return true
}
