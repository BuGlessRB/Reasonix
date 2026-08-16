package shellparse

import (
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

// ExitZeroImplies returns the source of every command that must have succeeded
// for the whole line to exit zero: both sides of `&&`, only the last stage of a
// pipeline, only the last of several `;`-separated statements, and neither side
// of `||`. ok is false when no static pass can decide it — a backgrounded job,
// negation, or unsupported syntax.
func ExitZeroImplies(command string) ([]string, bool) {
	if strings.TrimSpace(command) == "" {
		return nil, false
	}
	file, err := ParseBash(command)
	if err != nil || HasHereDoc(file) || len(file.Stmts) == 0 {
		return nil, false
	}
	return exitZeroImpliesStmt(command, file.Stmts[len(file.Stmts)-1])
}

func exitZeroImpliesStmt(source string, stmt *syntax.Stmt) ([]string, bool) {
	if stmt == nil || stmt.Negated || stmt.Background || stmt.Coprocess || stmt.Disown {
		return nil, false
	}
	switch cmd := stmt.Cmd.(type) {
	case *syntax.BinaryCmd:
		switch cmd.Op {
		case syntax.AndStmt:
			left, okLeft := exitZeroImpliesStmt(source, cmd.X)
			right, okRight := exitZeroImpliesStmt(source, cmd.Y)
			if !okLeft || !okRight {
				return nil, false
			}
			return append(left, right...), true
		case syntax.OrStmt:
			// Zero can come from either side, so neither one is proven.
			return nil, true
		default:
			return exitZeroImpliesStmt(source, cmd.Y)
		}
	case *syntax.CallExpr:
		return []string{sourceForStmt(source, stmt)}, true
	default:
		return nil, false
	}
}
