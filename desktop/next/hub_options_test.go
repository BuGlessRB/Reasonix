package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"

	"reasonix/internal/appupdate"
	"reasonix/internal/serve"
)

// hubOptionKeys reads the options this shell actually builds its hub with, from
// the syntax rather than by running it: the assembly needs a window, and the
// failure being guarded is silent — an option left out costs nothing at compile
// time and takes the capability behind it off the wire.
func hubOptionKeys(t *testing.T) map[string]bool {
	t.Helper()
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
	return keys
}

// A capability this shell implements and does not hand to its hub is one the
// kernel never registers a route for, so the panel that shows it goes blank and
// the menu that drives it reaches a nil host. It happened once, to Tray, and
// nothing failed until a person clicked.
func TestTheWindowHandsItsHubEveryCapabilityItImplements(t *testing.T) {
	keys := hubOptionKeys(t)
	for _, want := range []string{"Tray", "Update", "Install", "Asks", "Remote", "OnOpen", "OnClose", "DecorateSink", "Grant"} {
		if !keys[want] {
			t.Errorf("the hub is built without %s, so nothing can reach what this window implements for it", want)
		}
	}
}

// appupdate and serve name this capability from two packages that may not
// import each other, so the property lives in the assignment between them: an
// unowned application must leave the field nil, and a nil *capability inside a
// non-nil interface would register the routes and fail nowhere. Asserting it in
// appupdate stops working the moment New returns a concrete type.
func TestAnUnownedApplicationReachesTheHubAsNoUpdateHostAtAll(t *testing.T) {
	var opts serve.HubOptions
	opts.Update = appupdate.New(appupdate.Options{Running: "v1.0.0"})
	if opts.Update != nil {
		t.Fatalf("HubOptions.Update = %#v, want nil so the update routes stay unregistered", opts.Update)
	}
}
