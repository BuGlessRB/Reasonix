package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"strings"
	"testing"

	"reasonix/desktop/internal/update"
)

func TestStudioCatalogIsNotTheDesktopLine(t *testing.T) {
	if studioCatalog == "" {
		t.Fatal("studioCatalog is empty, so update.Options would fall back to the desktop catalog")
	}
	if studioCatalog == update.IndexURL {
		t.Fatalf("studioCatalog is the desktop line's catalog (%q); its entries name desktop artifacts", update.IndexURL)
	}
}

// The version panel and the installer each build their own update.Options, and
// a release listed by one catalog cannot be installed from another. An Options
// that omits IndexURL silently reads the desktop line, so this asserts on the
// package's source rather than on any single call: the failure mode is a new
// call site, not a changed one.
func TestEveryUpdaterOptionsSetsIndexURL(t *testing.T) {
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, ".", func(fi fs.FileInfo) bool {
		return !strings.HasSuffix(fi.Name(), "_test.go")
	}, 0)
	if err != nil {
		t.Fatalf("parse package: %v", err)
	}
	found := 0
	for _, pkg := range pkgs {
		for _, file := range pkg.Files {
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
				t.Errorf("%s: update.Options does not set IndexURL, so it reads the desktop catalog", fset.Position(lit.Pos()))
				return true
			})
		}
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
