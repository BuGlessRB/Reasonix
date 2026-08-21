package main

import (
	"go/ast"
	"strings"
)

// A refusal the client can act on carries a stable dotted code: the frontend
// looks the code up to say it in the reader's language, and falls back to the
// English only when it has no wording. http.Error writes prose with no code, so
// a caller sees a status and a sentence nobody translated — and, before the
// transport read plain bodies at all, only the status.
const refusalPackage = "internal/serve/"

// checkRefusalPath flags a plain http.Error where the coded refusal belongs.
func checkRefusalPath(s *sourceFile) []Finding {
	if !strings.HasPrefix(s.rel, refusalPackage) || strings.HasSuffix(s.rel, "_test.go") {
		return nil
	}
	var out []Finding
	ast.Inspect(s.file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok || !isSelector(call.Fun, "http", "Error") {
			return true
		}
		out = append(out, Finding{
			File:   s.rel,
			Line:   s.fset.Position(call.Pos()).Line,
			Rule:   ruleRefusalPath,
			Msg:    "http.Error sends a refusal with no code; use refuse() so the frontend can say it",
			Weight: 1,
		})
		return true
	})
	return out
}

func isSelector(fun ast.Expr, pkg, name string) bool {
	sel, ok := fun.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != name {
		return false
	}
	id, ok := sel.X.(*ast.Ident)
	return ok && id.Name == pkg
}
