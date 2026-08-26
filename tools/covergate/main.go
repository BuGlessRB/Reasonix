// covergate holds the declared-sensitive paths to the coverage they already
// have. The paths that decide what the agent may execute and reach were the
// least-tested part of the kernel when this was written — below the tree's
// median, not above it. A floor per path, ratcheted like repolint's budgets,
// is what keeps that from drifting back.
package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"reasonix/internal/instruction"
)

func main() {
	root := flag.String("root", ".", "repository root")
	baselinePath := flag.String("baseline", "", "baseline file (default <root>/tools/covergate/baseline.json)")
	update := flag.Bool("update", false, "rewrite the baseline from the current tree")
	allowDrop := flag.Bool("allow-drop", false, "let -update lower a floor. Raising one needs nothing; lowering one gives up coverage on a path the project called sensitive, which is the diff a reviewer has to be shown on purpose")
	flag.Parse()

	if *baselinePath == "" {
		*baselinePath = filepath.Join(*root, "tools", "covergate", "baseline.json")
	}
	globs, err := sensitiveGlobs(*root)
	if err != nil {
		fail(err)
	}
	if len(globs) == 0 {
		fail(fmt.Errorf("no `sensitive:` paths declared in the project's host checks"))
	}
	measured, err := measure(*root, globs)
	if err != nil {
		fail(err)
	}
	prev, _ := load(*baselinePath)

	if *update {
		if dropped := drops(prev, measured); len(dropped) > 0 && !*allowDrop {
			fmt.Fprintln(os.Stderr, "covergate: this -update would lower a floor on a declared-sensitive path:")
			for _, d := range dropped {
				fmt.Fprintln(os.Stderr, "  "+d)
			}
			fmt.Fprintln(os.Stderr, "\nAdd tests, or pass -allow-drop and justify the diff in the pull request.")
			os.Exit(1)
		}
		if err := write(*baselinePath, measured); err != nil {
			fail(err)
		}
		fmt.Printf("wrote %s (%d sensitive paths)\n", *baselinePath, len(measured))
		return
	}

	var below []string
	for _, path := range sortedKeys(measured) {
		floor, recorded := prev[path]
		if recorded && measured[path] < floor {
			below = append(below, fmt.Sprintf("%s: %.1f%% is below its %.1f%% floor", path, measured[path], floor))
		}
	}
	if len(below) > 0 {
		for _, b := range below {
			fmt.Fprintln(os.Stderr, "covergate: "+b)
		}
		fmt.Fprintln(os.Stderr, "\nThese paths decide what the agent may execute and reach. Restore the\ncoverage, or run `go run ./tools/covergate -update -allow-drop` and\njustify the diff in the pull request.")
		os.Exit(1)
	}
	fmt.Printf("covergate: clean (%d sensitive paths at or above their floor)\n", len(measured))
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "covergate:", err)
	os.Exit(2)
}

// sensitiveGlobs reads the same `sensitive:` bullets the runtime reads, so a
// path declared once is both reviewed harder and held to its coverage.
func sensitiveGlobs(root string) ([]string, error) {
	var docs []instruction.Document
	for _, name := range []string{"REASONIX.md", "AGENTS.md", "CLAUDE.md"} {
		body, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			continue
		}
		docs = append(docs, instruction.Document{Path: name, Body: string(body)})
	}
	return instruction.ExtractSensitivePaths(docs), nil
}

// packagesFor maps the declared globs onto the packages that must be run.
func packagesFor(globs []string) []string {
	seen := map[string]bool{}
	var pkgs []string
	for _, glob := range globs {
		dir := strings.TrimSuffix(strings.TrimSuffix(glob, "**"), "/")
		if filepath.Ext(dir) != "" {
			dir = filepath.Dir(dir)
		}
		if dir == "" || seen[dir] {
			continue
		}
		seen[dir] = true
		pkgs = append(pkgs, "./"+dir+"/...")
	}
	sort.Strings(pkgs)
	return pkgs
}

func measure(root string, globs []string) (map[string]float64, error) {
	profile := filepath.Join(os.TempDir(), "covergate.out")
	defer os.Remove(profile)
	args := append([]string{"test", "-count=1", "-coverprofile=" + profile}, packagesFor(globs)...)
	cmd := exec.Command("go", args...)
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "LANG=en_US.UTF-8")
	if out, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("go test: %w\n%s", err, out)
	}
	return fromProfile(profile, globs)
}

// fromProfile aggregates statements per declared path. Per file, not per
// package: a project may call one file sensitive inside a package that is not.
func fromProfile(profile string, globs []string) (map[string]float64, error) {
	f, err := os.Open(profile)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	total := map[string][2]int{}
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		file, stmts, covered, ok := parseProfileLine(scanner.Text())
		if !ok {
			continue
		}
		for _, glob := range globs {
			if !matchesGlob(glob, file) {
				continue
			}
			t := total[glob]
			t[0] += stmts
			if covered > 0 {
				t[1] += stmts
			}
			total[glob] = t
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	out := map[string]float64{}
	for _, glob := range globs {
		t, ok := total[glob]
		if !ok || t[0] == 0 {
			continue
		}
		// Round once, here, so the number compared is the number recorded; a
		// floor held to more precision than it is written with fails on itself.
		out[glob] = round1(float64(t[1]) / float64(t[0]) * 100)
	}
	return out, nil
}

func parseProfileLine(line string) (file string, stmts, covered int, ok bool) {
	// reasonix/internal/pkg/file.go:12.34,15.6 3 1
	colon := strings.LastIndex(line, ":")
	if colon < 0 {
		return "", 0, 0, false
	}
	fields := strings.Fields(line[colon+1:])
	if len(fields) != 3 {
		return "", 0, 0, false
	}
	stmts, err1 := strconv.Atoi(fields[1])
	covered, err2 := strconv.Atoi(fields[2])
	if err1 != nil || err2 != nil {
		return "", 0, 0, false
	}
	file = strings.TrimPrefix(line[:colon], "reasonix/")
	return file, stmts, covered, true
}

// matchesGlob handles the two shapes the host checks use: a `dir/**` subtree
// and one exact file.
func matchesGlob(glob, file string) bool {
	if subtree, ok := strings.CutSuffix(glob, "**"); ok {
		return strings.HasPrefix(file, subtree)
	}
	return file == glob
}

func load(path string) (map[string]float64, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return map[string]float64{}, err
	}
	var out map[string]float64
	if err := json.Unmarshal(data, &out); err != nil {
		return map[string]float64{}, err
	}
	return out, nil
}

func round1(v float64) float64 {
	// One decimal: a floor that tracked noise would fail on an unrelated diff.
	r, _ := strconv.ParseFloat(fmt.Sprintf("%.1f", v), 64)
	return r
}

func write(path string, floors map[string]float64) error {
	data, err := json.MarshalIndent(floors, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

func drops(prev, next map[string]float64) []string {
	var out []string
	for _, path := range sortedKeys(next) {
		if floor, ok := prev[path]; ok && next[path] < floor {
			out = append(out, fmt.Sprintf("%s: %.1f%% -> %.1f%%", path, floor, next[path]))
		}
	}
	return out
}

func sortedKeys(m map[string]float64) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
