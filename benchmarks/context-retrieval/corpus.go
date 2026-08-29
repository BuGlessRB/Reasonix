package main

import (
	"fmt"

	"reasonix/internal/agent"
	"reasonix/internal/provider"
)

// Synthetic history on purpose: an answer that exists in the workspace is
// solvable by reading the workspace, which measures grep. Every answer here is
// a nonce appearing in one folded message and nowhere else.

// experiment names which question a task belongs to.
const (
	experimentSearch = "search" // does search reach what no index addresses?
	experimentIndex  = "index"  // what is an ambient address line worth?
)

// cueTier is the narrowest fold-index scale at which an index task's cue is
// still addressed. Below it the model has to find the call without a hint.
const (
	tierQuarter = "quarter" // visible at default, half, quarter
	tierHalf    = "half"    // visible at default, half
	tierDefault = "default" // visible at default only
)

// contextTask is one recall question and everything needed to plant it,
// verify it, and score it.
type contextTask struct {
	ID         string
	Experiment string
	TargetKind string
	Prompt     string
	// AnswerMarkers must all appear in the final answer.
	AnswerMarkers []string
	// ProbeQuery is never shown to the model. Preflight runs it host-side so a
	// miss in a real run means the model did not search or searched badly, not
	// that BM25 could never have won.
	ProbeQuery string
	// CueMarker is the text an index line carries for an index task; CueTier is
	// the narrowest scale that still shows it.
	CueMarker string
	CueTier   string
	// PlantAfterGen ages the target: mergeFoldIndex trims from its oldest end,
	// so what follows a cue decides which budget addresses it. Frozen, because
	// a run that searched for it would let the tested code move its goalposts.
	PlantAfterGen int
	// Plant appends the target to a session and returns its canonical position.
	Plant func(*agent.Session) int
}

func plantAssistant(text string) func(*agent.Session) int {
	return func(s *agent.Session) int {
		at := len(s.Messages)
		s.Add(provider.Message{Role: provider.RoleAssistant, Content: text})
		return at
	}
}

// plantCall plants a tool call and its result. The call is the address and the
// cue; the result is the answer. Splitting them is what makes an index task
// measure the index as a pointer rather than as an answer store.
func plantCall(id, tool, args, result string) func(*agent.Session) int {
	return func(s *agent.Session) int {
		at := len(s.Messages)
		s.Add(provider.Message{Role: provider.RoleAssistant, ToolCalls: []provider.ToolCall{
			{ID: id, Name: tool, Arguments: args},
		}})
		s.Add(provider.Message{Role: provider.RoleTool, ToolCallID: id, Name: tool, Content: result})
		return at
	}
}

func searchTasks() []contextTask {
	return []contextTask{
		{
			ID: "r01-retry-boundary", Experiment: experimentSearch, TargetKind: "assistant_reasoning",
			Prompt:        "What ownership token did we settle on for the retry boundary, and why did we reject moving retries above framing?",
			AnswerMarkers: []string{"cobalt-lark-17", "partial"},
			ProbeQuery:    "retry boundary ownership token framing",
			Plant: plantAssistant("The retry boundary remains at ingress. The ownership token is cobalt-lark-17. " +
				"Moving retries above framing can duplicate a partially received stream."),
		},
		{
			ID: "r02-checkpoint-anchor", Experiment: experimentSearch, TargetKind: "assistant_reasoning",
			Prompt:        "What was the codename of the checkpoint anchor policy, and why didn't we anchor against digest positions?",
			AnswerMarkers: []string{"northglass-42", "generation-local"},
			ProbeQuery:    "checkpoint anchor policy digest positions",
			Plant: plantAssistant("The checkpoint anchor policy is northglass-42. " +
				"Digest positions are generation-local, so recovery must anchor against canonical state."),
		},
		{
			ID: "r03-recovery-cjk", Experiment: experimentSearch, TargetKind: "assistant_reasoning_cjk",
			Prompt:        "我们之前定的验证失败恢复策略叫什么？为什么失败后的 digest 不能直接变成新 baseline？",
			AnswerMarkers: []string{"松针-27", "可信基准"},
			ProbeQuery:    "恢复策略 baseline digest 验证失败",
			Plant: plantAssistant("恢复策略代号是“松针-27”。验证失败以后必须保留旧基线，不能把新的 digest 晋升为 baseline，" +
				"因为失败证据不能成为后续比较的可信基准。"),
		},
		{
			// The assistant's own account, not the tool call: a call always earns
			// an index line, and an addressed target measures the index rather
			// than search. KeepErrors rules out a failing result likewise.
			ID: "r04-lease-fence", Experiment: experimentSearch, TargetKind: "assistant_reasoning",
			Prompt:        "那次 lease fence 检查报告的诊断标记是什么？expected 和 observed epoch 分别是多少？",
			AnswerMarkers: []string{"delta-91", "12", "9"},
			ProbeQuery:    "lease fence writer epoch mismatch",
			Plant: plantAssistant("The lease fence check reported LEASE_FENCE delta-91: " +
				"expected writer epoch 12, observed 9. We left the fence in place."),
		},
		{
			ID: "r05-schema-guard", Experiment: experimentSearch, TargetKind: "assistant_reasoning",
			Prompt:        "当时 schema guard 报告的标记是什么，超预算多少 token？",
			AnswerMarkers: []string{"amber-214", "214"},
			ProbeQuery:    "schema guard provider visible surface budget",
			Plant: plantAssistant("The schema guard reported SCHEMA_GUARD amber-214: the provider-visible " +
				"tool schema exceeded the allowed surface by 214 tokens. We trimmed a description instead."),
		},
		{
			ID: "r06-shadow-probe", Experiment: experimentSearch, TargetKind: "assistant_reasoning",
			Prompt:        "我们那次 shadow probe 的 marker、p95 divergence 和 generation 分别是多少？",
			AnswerMarkers: []string{"violet-63", "0.037", "12"},
			ProbeQuery:    "shadow probe divergence generation marker",
			Plant: plantAssistant("The shadow probe came back as marker=violet-63 with a p95 divergence " +
				"of 0.037 at generation=12, which we judged within tolerance."),
		},
	}
}

func indexTasks() []contextTask {
	return []contextTask{
		{
			ID: "i01-transport-fallback", Experiment: experimentIndex, TargetKind: "tool_input",
			Prompt:        "transport fallback probe 当时报告的 selector 是什么？",
			AnswerMarkers: []string{"fern-73"},
			ProbeQuery:    "transport fallback probe selector",
			CueMarker:     "transport-fallback", CueTier: tierQuarter,
			PlantAfterGen: 4,
			Plant: plantCall("tf1", "bash", `{"command":"./scripts/probe transport-fallback"}`,
				"fallback selector = fern-73"),
		},
		{
			ID: "i02-scheduler-handoff", Experiment: experimentIndex, TargetKind: "tool_input",
			Prompt:        "scheduler handoff 当时记录的 grace 和 marker 是什么？",
			AnswerMarkers: []string{"37ms", "opal-18"},
			ProbeQuery:    "scheduler handoff grace marker",
			CueMarker:     "scheduler-handoff", CueTier: tierQuarter,
			PlantAfterGen: 4,
			Plant: plantCall("sh1", "read_file", `{"path":"scratch/scheduler-handoff.txt"}`,
				"handoff grace = 37ms\nmarker = opal-18"),
		},
		{
			ID: "i03-coalescing", Experiment: experimentIndex, TargetKind: "tool_input",
			Prompt:        "job completion 的 coalescing probe 最后记录的 window 和 marker 是多少？",
			AnswerMarkers: []string{"75ms", "cedar-52"},
			ProbeQuery:    "coalescing policy completion window",
			CueMarker:     "coalescing policy", CueTier: tierHalf,
			PlantAfterGen: 2,
			Plant: plantCall("cp1", "bash", `{"command":"grep \"coalescing policy\" scratch/job-notes.log"}`,
				"completion coalesce window = 75ms\nmarker = cedar-52"),
		},
		{
			// Reported, not failed: a failing command is the digest's obligation
			// (coverageDemands) and the host backstop's, so it never reaches the
			// fold index and could not measure an index budget at all.
			ID: "i04-recovery-fence", Experiment: experimentIndex, TargetKind: "tool_input",
			Prompt:        "recovery fence 那次检查报告的 marker 和两个 epoch 是什么？",
			AnswerMarkers: []string{"quartz-44", "11", "9"},
			ProbeQuery:    "recovery fence epoch check",
			CueMarker:     "recovery-fence", CueTier: tierHalf,
			PlantAfterGen: 2,
			Plant: plantCall("rf1", "bash", `{"command":"./scripts/check recovery-fence"}`,
				"RECOVERY_FENCE quartz-44\nexpected epoch 11\nobserved epoch 9"),
		},
		{
			ID: "i05-cache-probe", Experiment: experimentIndex, TargetKind: "tool_input",
			Prompt:        "cache probe 当时记录的 prefix salt 和 scope 是什么？",
			AnswerMarkers: []string{"comet-42", "per-lineage"},
			ProbeQuery:    "cache probe prefix salt scope",
			CueMarker:     "cache-probe", CueTier: tierDefault,
			PlantAfterGen: 0,
			Plant: plantCall("cb1", "read_file", `{"path":"scratch/cache-probe.txt"}`,
				"prefix salt = comet-42\nscope = per-lineage"),
		},
		{
			ID: "i06-dispatch-handoff", Experiment: experimentIndex, TargetKind: "tool_input",
			Prompt:        "dispatch handoff probe 当时确认的 mode 和 marker 是什么？",
			AnswerMarkers: []string{"level-triggered", "iris-31"},
			ProbeQuery:    "dispatch handoff probe mode",
			CueMarker:     "dispatch-handoff", CueTier: tierDefault,
			PlantAfterGen: 0,
			Plant: plantCall("dh1", "bash", `{"command":"./scripts/probe dispatch-handoff"}`,
				"handoff mode = level-triggered\nmarker = iris-31"),
		},
	}
}

func allTasks() []contextTask {
	return append(searchTasks(), indexTasks()...)
}

func taskByID(id string) (contextTask, bool) {
	for _, t := range allTasks() {
		if t.ID == id {
			return t, true
		}
	}
	return contextTask{}, false
}

// tierScales is the fixture's contract: which arms must address a cue and
// which must not. A policy change that moves a cue across a boundary fails
// preflight rather than turning two arms into one experiment.
func tierScales(tier string) (visible, hidden []string) {
	switch tier {
	case tierQuarter:
		return []string{"default", "half", "quarter"}, []string{"off"}
	case tierHalf:
		return []string{"default", "half"}, []string{"quarter", "off"}
	case tierDefault:
		return []string{"default"}, []string{"half", "quarter", "off"}
	default:
		return nil, nil
	}
}

func validateCorpus() error {
	seen := map[string]bool{}
	for _, t := range allTasks() {
		if seen[t.ID] {
			return fmt.Errorf("duplicate task id %q", t.ID)
		}
		seen[t.ID] = true
		if len(t.AnswerMarkers) == 0 || t.ProbeQuery == "" || t.Prompt == "" {
			return fmt.Errorf("%s: a task needs a prompt, answer markers and a probe query", t.ID)
		}
		if t.Experiment == experimentIndex {
			if t.CueMarker == "" {
				return fmt.Errorf("%s: an index task needs a cue marker", t.ID)
			}
			if v, _ := tierScales(t.CueTier); v == nil {
				return fmt.Errorf("%s: unknown cue tier %q", t.ID, t.CueTier)
			}
		}
	}
	return nil
}
