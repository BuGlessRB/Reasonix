package cli

// Every exit funnels through shutdownAndQuit so the transcript beyond the last
// snapshot survives (#5879). Which snapshot it takes is the rest of that promise:
// the plain one gives it up when another instance holds this session's lock.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"
)

// shutdownCase returns the body of shutdownAndQuit.
func shutdownCase(t *testing.T) []ast.Stmt {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), "chat_tui_shutdown.go", nil, 0)
	if err != nil {
		t.Fatalf("parse chat_tui_shutdown.go: %v", err)
	}
	var body []ast.Stmt
	ast.Inspect(file, func(n ast.Node) bool {
		fn, ok := n.(*ast.FuncDecl)
		if !ok || fn.Name.Name != "shutdownAndQuit" || fn.Body == nil {
			return true
		}
		body = fn.Body.List
		return false
	})
	if body == nil {
		t.Fatal("no shutdownAndQuit; the shutdown path moved")
	}
	return body
}

func TestShutdownTakesTheRecoveringSnapshot(t *testing.T) {
	var took []string
	for _, stmt := range shutdownCase(t) {
		ast.Inspect(stmt, func(n ast.Node) bool {
			if sel, ok := n.(*ast.SelectorExpr); ok {
				switch sel.Sel.Name {
				case "Snapshot", "SnapshotForShutdown", "SnapshotActivity", "SnapshotRewrite":
					took = append(took, sel.Sel.Name)
				}
			}
			return true
		})
	}
	if len(took) != 1 || took[0] != "SnapshotForShutdown" {
		t.Fatalf("shutdown takes %v, want exactly [SnapshotForShutdown] — a plain "+
			"Snapshot loses the transcript when the session file lock is held elsewhere", took)
	}
}

// The failure has nowhere to be drawn: the alt-screen is gone by the time anyone
// could read it. Discarding it here discards the fact that this session's
// transcript never reached disk, so it must ride out on the model.
func TestFailedShutdownSaveIsNotDiscarded(t *testing.T) {
	kept := false
	for _, stmt := range shutdownCase(t) {
		ast.Inspect(stmt, func(n ast.Node) bool {
			assign, ok := n.(*ast.AssignStmt)
			if !ok || len(assign.Rhs) != 1 || len(assign.Lhs) != 1 {
				return true
			}
			call, ok := assign.Rhs[0].(*ast.CallExpr)
			if !ok {
				return true
			}
			if sel, ok := call.Fun.(*ast.SelectorExpr); !ok || sel.Sel.Name != "SnapshotForShutdown" {
				return true
			}
			if id, ok := assign.Lhs[0].(*ast.Ident); ok && id.Name == "_" {
				t.Error("the final save's error is assigned to _; a lost transcript would be silent")
			}
			kept = true
			return false
		})
	}
	if !kept {
		t.Fatal("the final save's result is not kept; nothing can report a lost transcript")
	}
}
