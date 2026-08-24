package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const (
	specEnglish = "docs/SPEC.md"
	specChinese = "docs/SPEC.zh-CN.md"
)

// lockedSpecSections are the sections both SPECs carry in full. The Chinese
// companion abridges the rest on purpose, so parity is declared here rather
// than inferred from the tree — the same reason the host's sensitive paths
// are. Adding a section commits every later change to it to both files.
var lockedSpecSections = []string{"3.11", "3.13", "3.14", "3.15", "3.16"}

var specHeadingRe = regexp.MustCompile(`^#{2,4}\s+(\d+(?:\.\d+)*)\s`)

// specBlock is one blank-line-separated run, or one fenced region. Size is
// carried only where a translation cannot legitimately change it: a table row
// and a fenced line survive translation, a wrapped sentence does not.
type specBlock struct {
	kind  string
	lines int
}

type specSection struct {
	line   int
	blocks []specBlock
}

// checkSpecParity holds the two SPECs to the same structure wherever they claim
// to say the same thing. It reads headings and block shape only — comparing
// wording would fire on every legitimate translation choice, and is a judgement
// this tool cannot read from structure.
func checkSpecParity(root string) []Finding {
	en, enOrder, err := readSpecSections(filepath.Join(root, specEnglish))
	if err != nil {
		return nil
	}
	zh, zhOrder, err := readSpecSections(filepath.Join(root, specChinese))
	if err != nil {
		return nil
	}
	return specParityFindings(lockedSpecSections, en, enOrder, zh, zhOrder)
}

func specParityFindings(locked []string, en map[string]specSection, enOrder []string, zh map[string]specSection, zhOrder []string) []Finding {
	var out []Finding
	if msg := specHeadingDiff(enOrder, zhOrder); msg != "" {
		out = append(out, Finding{specChinese, 1, ruleSpecParity, msg, 1})
	}
	for _, num := range locked {
		src, srcOK := en[num]
		dst, dstOK := zh[num]
		switch {
		case !srcOK:
			out = append(out, Finding{specEnglish, 1, ruleSpecParity, specMissing(num), 1})
		case !dstOK:
			out = append(out, Finding{specChinese, 1, ruleSpecParity, specMissing(num), 1})
		default:
			if msg := specShapeDiff(num, src.blocks, dst.blocks); msg != "" {
				out = append(out, Finding{specChinese, dst.line, ruleSpecParity, msg, 1})
			}
		}
	}
	return out
}

func specMissing(num string) string {
	return fmt.Sprintf("§%s is locked for translation parity but is missing here", num)
}

func specHeadingDiff(en, zh []string) string {
	for i := range max(len(en), len(zh)) {
		switch {
		case i >= len(zh):
			return fmt.Sprintf("§%s has a heading in %s and none here", en[i], specEnglish)
		case i >= len(en):
			return fmt.Sprintf("§%s has a heading here and none in %s", zh[i], specEnglish)
		case en[i] != zh[i]:
			return fmt.Sprintf("heading %d is §%s here and §%s in %s", i+1, zh[i], en[i], specEnglish)
		}
	}
	return ""
}

func specShapeDiff(num string, en, zh []specBlock) string {
	if len(en) != len(zh) {
		return fmt.Sprintf("§%s is locked for translation parity: %d blocks here, %d in %s", num, len(zh), len(en), specEnglish)
	}
	for i := range en {
		if en[i].kind != zh[i].kind {
			return fmt.Sprintf("§%s block %d is %s here and %s in %s", num, i+1, zh[i].kind, en[i].kind, specEnglish)
		}
		if en[i].lines != zh[i].lines {
			return fmt.Sprintf("§%s %s block %d has %d lines here and %d in %s", num, zh[i].kind, i+1, zh[i].lines, en[i].lines, specEnglish)
		}
	}
	return ""
}

func readSpecSections(path string) (map[string]specSection, []string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, err
	}
	sections, order := parseSpecSections(data)
	return sections, order, nil
}

func parseSpecSections(data []byte) (map[string]specSection, []string) {
	sections := map[string]specSection{}
	var order []string
	var num string
	var body []string
	line := 0
	flush := func() {
		if num != "" {
			sections[num] = specSection{line: line, blocks: specBlocks(body)}
		}
	}
	for i, raw := range splitLines(data) {
		m := specHeadingRe.FindStringSubmatch(raw)
		if m == nil {
			if num != "" {
				body = append(body, raw)
			}
			continue
		}
		flush()
		num, line, body = m[1], i+1, nil
		order = append(order, num)
	}
	flush()
	return sections, order
}

func specBlocks(body []string) []specBlock {
	var out []specBlock
	var cur []string
	fenced := false
	flush := func() {
		if len(cur) > 0 {
			out = append(out, classifySpecBlock(cur))
			cur = nil
		}
	}
	for _, raw := range body {
		fence := strings.HasPrefix(raw, "```")
		switch {
		case fence && !fenced:
			flush()
			cur = append(cur, raw)
			fenced = true
		case fence:
			cur = append(cur, raw)
			flush()
			fenced = false
		case fenced:
			cur = append(cur, raw)
		case strings.TrimSpace(raw) == "":
			flush()
		default:
			cur = append(cur, raw)
		}
	}
	flush()
	return out
}

func classifySpecBlock(lines []string) specBlock {
	switch {
	case strings.HasPrefix(lines[0], "```"):
		return specBlock{kind: "code", lines: len(lines)}
	case strings.HasPrefix(lines[0], "|"):
		return specBlock{kind: "table", lines: len(lines)}
	case specIsList(lines):
		return specBlock{kind: "list"}
	}
	return specBlock{kind: "prose"}
}

// specIsList reads a continuation line by its indent, which is what separates a
// wrapped bullet from the paragraph that follows a list.
func specIsList(lines []string) bool {
	for _, raw := range lines {
		if strings.HasPrefix(raw, "  ") {
			continue
		}
		trimmed := strings.TrimSpace(raw)
		if strings.HasPrefix(trimmed, "-") || strings.HasPrefix(trimmed, "*") || strings.HasPrefix(trimmed, "1.") {
			continue
		}
		return false
	}
	return true
}
