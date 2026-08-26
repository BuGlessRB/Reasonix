package main

import (
	"fmt"
	"go/ast"
	"slices"
	"strings"
)

// An exported kernel API with no production caller anywhere in the tree is how
// a capability dies quietly: deleting the one frontend that called it leaves
// the implementation, its tests, and its doc comments all intact and green.
// That is what left config recovery reading a snapshot nothing wrote. Tests do
// not count as callers — they are exactly what keeps such a corpse warm.
type orphanScan struct {
	decls map[string][]orphanDecl
	uses  map[string]int
}

type orphanDecl struct {
	file string
	line int
	kind string
}

func newOrphanScan() *orphanScan {
	return &orphanScan{decls: map[string][]orphanDecl{}, uses: map[string]int{}}
}

// observe records one file's exported kernel declarations and, separately, every
// identifier it reads. Both halves span the whole tree — the desktop module is
// a caller of internal/, so a scan that stopped at one module would call its
// APIs dead.
func (o *orphanScan) observe(src *sourceFile) {
	if src.file == nil || strings.HasSuffix(src.rel, "_test.go") {
		return
	}
	kernel := strings.HasPrefix(src.rel, "internal/") && !testSupportPkg(src.rel)
	for _, decl := range src.file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Name == nil {
			continue
		}
		name := fn.Name.Name
		if kernel && ast.IsExported(name) && !orphanExempt(name) {
			kind := "func"
			if fn.Recv != nil {
				kind = "method"
			}
			o.decls[name] = append(o.decls[name], orphanDecl{
				file: src.rel, line: src.fset.Position(fn.Pos()).Line, kind: kind,
			})
		}
	}
	ast.Inspect(src.file, func(n ast.Node) bool {
		switch v := n.(type) {
		case *ast.FuncDecl:
			// The declaration's own name is not a use of it; its body is.
			if v.Recv != nil {
				ast.Inspect(v.Recv, func(r ast.Node) bool { o.countIdent(r); return true })
			}
			if v.Type != nil {
				ast.Inspect(v.Type, func(t ast.Node) bool { o.countIdent(t); return true })
			}
			if v.Body != nil {
				ast.Inspect(v.Body, func(b ast.Node) bool { o.countIdent(b); return true })
			}
			return false
		default:
			o.countIdent(n)
		}
		return true
	})
}

func (o *orphanScan) countIdent(n ast.Node) {
	if id, ok := n.(*ast.Ident); ok {
		o.uses[id.Name]++
	}
}

// testSupportPkg marks packages that exist to serve tests across package
// boundaries. Their exports cannot move into _test.go, so "no production
// caller" is their normal state rather than a finding.
func testSupportPkg(rel string) bool {
	dir := rel
	if i := strings.LastIndex(rel, "/"); i >= 0 {
		dir = rel[:i]
	}
	base := dir
	if i := strings.LastIndex(dir, "/"); i >= 0 {
		base = dir[i+1:]
	}
	return strings.HasPrefix(base, "test") || strings.HasSuffix(base, "test") || strings.HasSuffix(base, "testutil")
}

// orphanExempt skips names the language or a runtime calls for us, where no
// source reference proves anything.
func orphanExempt(name string) bool {
	switch name {
	case "Main", "TestMain":
		return true
	}
	// Go calls these through interfaces the standard library owns.
	return slices.Contains([]string{
		"Error", "String", "Read", "Write", "Close", "Unwrap", "Is", "As",
		"MarshalJSON", "UnmarshalJSON", "ServeHTTP",
	}, name)
}

// findings reports every exported kernel function whose only appearances in
// non-test code are its own declarations.
func (o *orphanScan) findings() []Finding {
	var out []Finding
	for name, decls := range o.decls {
		if o.uses[name] > 0 {
			continue
		}
		for _, d := range decls {
			out = append(out, Finding{
				File: d.file, Line: d.line, Rule: ruleOrphan, Weight: 1,
				Msg: fmt.Sprintf("exported %s %s has no caller outside tests; wire it or delete it", d.kind, name),
			})
		}
	}
	return out
}
