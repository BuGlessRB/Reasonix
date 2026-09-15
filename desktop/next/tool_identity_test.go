package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// Names no literal return states: lsp.posTool is built from a slice of
// literals, complete_subtask returns a constant, and skill.subagentSkillTool
// takes its name from the skill it was installed for.
var extraKernelToolNames = []string{
	"lsp_definition", "lsp_references", "lsp_hover", "complete_subtask",
}

// Drawn from a card but registered by nobody: web_search is the provider's own,
// use_capability the proxy every other name arrives through, guardian the host's.
var unregisteredCardNames = []string{"web_search", "use_capability", "guardian_assessment"}

// Below this the scan has stopped seeing the tree rather than found it clean.
const minKernelTools = 45

func TestEveryKernelToolHasACardIdentity(t *testing.T) {
	kernel := kernelToolNames(t)
	if len(kernel) < minKernelTools {
		t.Fatalf("found only %d tool names; the scan is no longer reading the registrations", len(kernel))
	}

	labels := tsRecordKeys(t, filepath.Join("..", "frontend-next", "src", "ui", "icons.ts"), "LABEL")
	for _, name := range kernel {
		if !labels[name] {
			t.Errorf("%s has no entry in icons.ts LABEL: it renders as its own id", name)
		}
	}
}

// The other direction: a name in the tables the kernel no longer registers is a
// row nobody can reach, which is what a rename leaves behind.
func TestNoCardIdentityNamesAToolTheKernelDoesNotRegister(t *testing.T) {
	kernel := map[string]bool{}
	for _, n := range kernelToolNames(t) {
		kernel[n] = true
	}
	for _, n := range unregisteredCardNames {
		kernel[n] = true
	}

	for _, file := range []struct{ path, record string }{
		{filepath.Join("..", "frontend-next", "src", "ui", "icons.ts"), "LABEL"},
		{filepath.Join("..", "frontend-next", "src", "ui", "Sym.tsx"), "BY_TOOL"},
	} {
		for name := range tsRecordKeys(t, file.path, file.record) {
			if !kernel[name] {
				t.Errorf("%s names %q, which no kernel registration returns", file.record, name)
			}
		}
	}
}

var (
	toolName   = regexp.MustCompile(`func \((?:\w+ )?\*?(\w+)\) Name\(\) string\s*{\s*return "([a-z_]+)"`)
	toolSchema = regexp.MustCompile(`func \((?:\w+ )?\*?(\w+)\) Schema\(\) json\.RawMessage`)
)

func kernelToolNames(t *testing.T) []string {
	t.Helper()
	seen := map[string]bool{}
	for _, name := range extraKernelToolNames {
		seen[name] = true
	}
	named := map[string]map[string]string{}
	schemas := map[string]map[string]bool{}
	root := filepath.Join("..", "..", "internal")
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return err
		}
		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		pkg := filepath.Dir(path)
		if named[pkg] == nil {
			named[pkg], schemas[pkg] = map[string]string{}, map[string]bool{}
		}
		for _, m := range toolSchema.FindAllStringSubmatch(string(src), -1) {
			schemas[pkg][m[1]] = true
		}
		for _, m := range toolName.FindAllStringSubmatch(string(src), -1) {
			named[pkg][m[1]] = m[2]
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for pkg, types := range named {
		for typ, name := range types {
			if schemas[pkg][typ] {
				seen[name] = true
			}
		}
	}
	out := make([]string, 0, len(seen))
	for n := range seen {
		out = append(out, n)
	}
	return out
}

// The keys of a `const <name>: Record<string, string> = { … }` literal, read as
// text because the two sides meet nowhere a compiler could check them.
func tsRecordKeys(t *testing.T, path, record string) map[string]bool {
	t.Helper()
	src, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	_, body, ok := strings.Cut(string(src), "const "+record)
	if !ok {
		t.Fatalf("%s no longer declares %s; this test cannot see the table", path, record)
	}
	table, _, ok := strings.Cut(body, "\n};")
	if !ok {
		t.Fatalf("%s: %s is not closed by a line of its own", path, record)
	}
	keys := map[string]bool{}
	for _, m := range regexp.MustCompile(`([a-z_][a-z0-9_]*)\s*:\s*"`).FindAllStringSubmatch(table, -1) {
		keys[m[1]] = true
	}
	if len(keys) == 0 {
		t.Fatalf("%s: read no keys out of %s", path, record)
	}
	return keys
}
