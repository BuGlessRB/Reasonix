package main

import (
	"fmt"
	"go/ast"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"strings"
)

// Where the desktop re-declares the kernel's wire types by hand.
const (
	tsWireFile     = "desktop/frontend-next/src/port/wire.ts"
	tsBoundaryFile = "desktop/frontend-next/src/port/boundary.ts"
)

// mirroredWireTypes are the Go types the desktop keeps a second, hand-written
// copy of: the graph, whose consumers all fold the same deltas, and the sandbox
// editor, which draws a security posture a field it cannot read would get
// wrong. Declared and not inferred, for the reason the sensitive paths are: no
// spelling tells a mirrored contract from a struct with json tags.
var mirroredWireTypes = []wireMirror{
	{"internal/agentgraph/graph.go", "Node", tsWireFile, "GraphNode"},
	{"internal/agentgraph/graph.go", "Edge", tsWireFile, "GraphEdge"},
	{"internal/agentgraph/graph.go", "Delta", tsWireFile, "GraphDelta"},
	{"internal/control/boundary.go", "SandboxSettings", tsBoundaryFile, "SandboxSettings"},
}

type wireMirror struct {
	goFile, goType, tsFile, tsType string
}

// wireScan collects the declared types' wire field names while the tree is
// walked, so the comparison costs no second parse.
type wireScan struct{ fields map[string][]string }

func newWireScan() *wireScan { return &wireScan{fields: map[string][]string{}} }

func (w *wireScan) observe(src *sourceFile) {
	if src.file == nil {
		return
	}
	for _, m := range mirroredWireTypes {
		if m.goFile != src.rel {
			continue
		}
		if names, ok := wireFieldNames(src.file, m.goType); ok {
			w.fields[m.goFile+"."+m.goType] = names
		}
	}
}

// findings compares each declared pair both ways. Either direction is a defect:
// a picture cannot show what it was never told, and it cannot expect what
// nothing sends.
func (w *wireScan) findings(root string) []Finding {
	bodies := map[string]string{}
	for _, m := range mirroredWireTypes {
		if _, read := bodies[m.tsFile]; read {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(root, m.tsFile))
		if err != nil {
			return nil
		}
		bodies[m.tsFile] = string(raw)
	}
	return wireParityFindings(mirroredWireTypes, w.fields, bodies)
}

// wireParityFindings is the comparison itself, so a fixture can carry the one
// pair a case is about rather than the tree's real ones.
func wireParityFindings(mirrors []wireMirror, goFields map[string][]string, bodies map[string]string) []Finding {
	var out []Finding
	for _, m := range mirrors {
		fields, declared := goFields[m.goFile+"."+m.goType]
		if !declared {
			out = append(out, Finding{m.goFile, 1, ruleWireParity,
				fmt.Sprintf("%s is declared a mirrored wire type and is not a struct here", m.goType), 1})
			continue
		}
		tsFields, line, found := tsInterfaceFields(bodies[m.tsFile], m.tsType)
		if !found {
			out = append(out, Finding{m.tsFile, 1, ruleWireParity,
				fmt.Sprintf("%s mirrors %s.%s and is not declared here", m.tsType, m.goFile, m.goType), 1})
			continue
		}
		for _, name := range missingFrom(fields, tsFields) {
			out = append(out, Finding{m.tsFile, line, ruleWireParity,
				fmt.Sprintf("%s sends %q and %s cannot read it", m.goType, name, m.tsType), 1})
		}
		for _, name := range missingFrom(tsFields, fields) {
			out = append(out, Finding{m.tsFile, line, ruleWireParity,
				fmt.Sprintf("%s reads %q and %s never sends it", m.tsType, name, m.goType), 1})
		}
	}
	return out
}

func missingFrom(want, have []string) []string {
	var out []string
	for _, name := range want {
		if !slices.Contains(have, name) {
			out = append(out, name)
		}
	}
	return out
}

// wireFieldNames lists what this struct serialises as. A field the encoder skips
// is not part of the contract and is not reported.
func wireFieldNames(file *ast.File, typeName string) ([]string, bool) {
	st, ok := structNamed(file, typeName)
	if !ok {
		return nil, false
	}
	var out []string
	for _, field := range st.Fields.List {
		for _, name := range field.Names {
			if !name.IsExported() {
				continue
			}
			if wire, keep := wireName(field, name.Name); keep {
				out = append(out, wire)
			}
		}
	}
	return out, true
}

func structNamed(file *ast.File, typeName string) (*ast.StructType, bool) {
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok {
			continue
		}
		for _, spec := range gen.Specs {
			ts, ok := spec.(*ast.TypeSpec)
			if !ok || ts.Name.Name != typeName {
				continue
			}
			if st, ok := ts.Type.(*ast.StructType); ok && st.Fields != nil {
				return st, true
			}
		}
	}
	return nil, false
}

func wireName(field *ast.Field, goName string) (string, bool) {
	if field.Tag == nil {
		return goName, true
	}
	tag := reflect.StructTag(strings.Trim(field.Tag.Value, "`")).Get("json")
	if tag == "-" {
		return "", false
	}
	if name, _, _ := strings.Cut(tag, ","); name != "" {
		return name, true
	}
	return goName, true
}

var (
	tsInterfaceRe = regexp.MustCompile(`(?m)^export interface (\w+) \{`)
	tsPropertyRe  = regexp.MustCompile(`^\s*(\w+)\??:`)
)

// tsInterfaceFields reads one interface's property names and the line it opens
// on. It reads the declaration's shape, never its wording, which is all a
// contract is.
func tsInterfaceFields(body, typeName string) ([]string, int, bool) {
	for _, m := range tsInterfaceRe.FindAllStringSubmatchIndex(body, -1) {
		if body[m[2]:m[3]] != typeName {
			continue
		}
		fields, _, ok := strings.Cut(body[m[1]:], "\n}")
		if !ok {
			return nil, 0, false
		}
		var out []string
		for line := range strings.SplitSeq(fields, "\n") {
			if strings.HasPrefix(strings.TrimSpace(line), "//") {
				continue
			}
			if p := tsPropertyRe.FindStringSubmatch(line); p != nil {
				out = append(out, p[1])
			}
		}
		return out, strings.Count(body[:m[0]], "\n") + 1, true
	}
	return nil, 0, false
}
