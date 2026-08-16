package shellparse

import (
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

// SinglePipelineStages returns the stage sources of a command that is exactly
// one pipeline, left to right, so a caller holding bash's PIPESTATUS can line
// each status up with the stage that produced it. ok is false for anything
// else: once `&&`, `;` or a background job is in play, PIPESTATUS describes
// only whichever pipeline ran last, and lining it up would be a guess.
func SinglePipelineStages(command string) ([]string, bool) {
	if strings.TrimSpace(command) == "" {
		return nil, false
	}
	file, err := ParseBash(command)
	if err != nil || HasHereDoc(file) || len(file.Stmts) != 1 {
		return nil, false
	}
	stmt := file.Stmts[0]
	if stmt == nil || stmt.Negated || stmt.Background || stmt.Coprocess || stmt.Disown {
		return nil, false
	}
	binary, ok := stmt.Cmd.(*syntax.BinaryCmd)
	if !ok || (binary.Op != syntax.Pipe && binary.Op != syntax.PipeAll) {
		return nil, false
	}
	var stages []string
	if !appendPipelineStages(command, stmt, &stages) {
		return nil, false
	}
	return stages, len(stages) > 1
}

func appendPipelineStages(source string, stmt *syntax.Stmt, stages *[]string) bool {
	if stmt == nil || stmt.Negated || stmt.Background || stmt.Coprocess || stmt.Disown {
		return false
	}
	switch cmd := stmt.Cmd.(type) {
	case *syntax.BinaryCmd:
		if cmd.Op != syntax.Pipe && cmd.Op != syntax.PipeAll {
			return false
		}
		return appendPipelineStages(source, cmd.X, stages) &&
			appendPipelineStages(source, cmd.Y, stages)
	case *syntax.CallExpr:
		*stages = append(*stages, sourceForStmt(source, stmt))
		return true
	default:
		return false
	}
}
