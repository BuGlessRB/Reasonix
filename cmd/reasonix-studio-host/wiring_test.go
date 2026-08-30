package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"
)

// A capability this host implements and does not hand to its hub is one the
// kernel never registers a route for. Nothing fails at compile time, and the
// shell in front of it simply cannot do the thing — which is how remote
// workspaces were missing here while the protocol for them was already green.
func TestTheHostHandsItsHubEveryCapabilityItImplements(t *testing.T) {
	file, err := parser.ParseFile(token.NewFileSet(), "main.go", nil, 0)
	if err != nil {
		t.Fatalf("parse main.go: %v", err)
	}
	keys := map[string]bool{}
	ast.Inspect(file, func(n ast.Node) bool {
		lit, ok := n.(*ast.CompositeLit)
		if !ok {
			return true
		}
		sel, ok := lit.Type.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != "HubOptions" {
			return true
		}
		for _, elt := range lit.Elts {
			if kv, ok := elt.(*ast.KeyValueExpr); ok {
				if key, ok := kv.Key.(*ast.Ident); ok {
					keys[key.Name] = true
				}
			}
		}
		return false
	})
	if len(keys) == 0 {
		t.Fatal("no hub options found; this guard is watching nothing")
	}
	for _, want := range []string{"Tray", "Asks", "Remote", "DecorateSink", "Grant"} {
		if !keys[want] {
			t.Errorf("the hub is built without %s, so nothing can reach what this host implements for it", want)
		}
	}
}
