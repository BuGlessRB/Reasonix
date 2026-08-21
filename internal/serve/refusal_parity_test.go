package serve

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// codeArg is where each helper carries the dotted code, so a new one is added
// here rather than by teaching a regex another shape.
var codeArg = map[string]int{"refuse": 2, "busy": 1, "coded": 0, "busyErr": 0, "refusal": 1}

// serveRefusalCodes reads the codes this package can send, from the syntax
// rather than from the text: a scan that matched on wording would be the very
// defect the coded refusal exists to end.
func serveRefusalCodes(t *testing.T) map[string]string {
	t.Helper()
	out := map[string]string{}
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read package dir: %v", err)
	}
	fset := token.NewFileSet()
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		ast.Inspect(file, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			id, ok := call.Fun.(*ast.Ident)
			if !ok {
				return true
			}
			at, ok := codeArg[id.Name]
			if !ok || at >= len(call.Args) {
				return true
			}
			lit, ok := call.Args[at].(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				return true
			}
			code, err := strconv.Unquote(lit.Value)
			if err == nil && strings.Contains(code, ".") {
				out[code] = name
			}
			return true
		})
	}
	return out
}

func translatedCodes(t *testing.T) map[string]bool {
	t.Helper()
	path := filepath.Join("..", "..", "desktop", "frontend-next", "src", "i18n", "kernel.ts")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Skipf("no frontend catalogue: %v", err)
	}
	out := map[string]bool{}
	for line := range strings.SplitSeq(string(data), "\n") {
		key, _, ok := strings.Cut(strings.TrimSpace(line), ":")
		if !ok {
			continue
		}
		if code, err := strconv.Unquote(key); err == nil && strings.Contains(code, ".") {
			out[code] = true
		}
	}
	return out
}

// A code the frontend cannot say degrades to the kernel's English, which no
// reader asked for. This is what kernel.ts exports `codes` for.
func TestEveryRefusalCodeIsTranslated(t *testing.T) {
	said := translatedCodes(t)
	if len(said) == 0 {
		t.Fatal("no codes read from the frontend catalogue; this guard is watching nothing")
	}
	var missing []string
	for code, file := range serveRefusalCodes(t) {
		if !said[code] {
			missing = append(missing, code+" ("+file+")")
		}
	}
	if len(missing) > 0 {
		t.Fatalf("refusal codes with no wording in kernel.ts:\n  %s", strings.Join(missing, "\n  "))
	}
}
