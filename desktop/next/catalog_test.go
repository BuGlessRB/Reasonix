package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strings"
	"testing"
)

// The version panel and the installer each build their own update.Options, and
// a release listed by one catalog cannot be installed from another. An Options
// that omits IndexURL now fails at fetch, but only once a user asks for an
// update; this asserts on the package's source so the mistake is caught at
// test time, and because the failure mode is a new call site, not a changed one.
func TestEveryUpdaterOptionsSetsIndexURL(t *testing.T) {
	fset := token.NewFileSet()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read package dir: %v", err)
	}
	found := 0
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		// Every file, whatever its build tags: an Options behind a tag reads the
		// wrong catalog just as surely as one in the default build, and ParseDir
		// left that to a deprecated package-association rule.
		file, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		ast.Inspect(file, func(n ast.Node) bool {
			lit, ok := n.(*ast.CompositeLit)
			if !ok || !isUpdateOptions(lit.Type) {
				return true
			}
			found++
			for _, elt := range lit.Elts {
				kv, ok := elt.(*ast.KeyValueExpr)
				if !ok {
					continue
				}
				if id, ok := kv.Key.(*ast.Ident); ok && id.Name == "IndexURL" {
					return true
				}
			}
			t.Errorf("%s: update.Options does not set IndexURL, so its fetch has no catalog to read", fset.Position(lit.Pos()))
			return true
		})
	}
	if found == 0 {
		t.Fatal("no update.Options literal found; this guard is no longer watching anything")
	}
}

func isUpdateOptions(expr ast.Expr) bool {
	sel, ok := expr.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "Options" {
		return false
	}
	id, ok := sel.X.(*ast.Ident)
	return ok && id.Name == "update"
}
