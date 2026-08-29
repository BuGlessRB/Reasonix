package main

import (
	"fmt"
	"math/rand"

	"reasonix/internal/agent"
	"reasonix/internal/provider"
)

// Synthetic history on purpose: an answer that exists in the workspace is
// solvable by reading the workspace, which measures grep. Every answer is a
// placeholder here and a value only inside one run.

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

// plantKind is how a task's target enters the transcript.
const (
	plantAssistant = "assistant" // the model's own account: never index-addressed
	plantCall      = "call"      // a tool call and its result: always index-addressed
)

// contextTask is one recall question as a template. Nothing here is an answer.
type contextTask struct {
	ID         string
	Experiment string
	TargetKind string
	Prompt     string
	ProbeQuery string
	// Vars declare the placeholders. AnswerVars name the ones a correct answer
	// must contain, CueVar the one an index line carries.
	Vars       []varSpec
	AnswerVars []string
	CueVar     string
	CueTier    string
	// PlantAfterGen ages the target: mergeFoldIndex trims from its oldest end,
	// so what follows a cue decides which budget addresses it. Frozen, because
	// a run that searched for it would let the tested code move its goalposts.
	PlantAfterGen int
	// Plant is how the target enters, and the rest is what it says.
	Plant     string
	CallTool  string
	CallArgs  string
	CallID    string
	PlantBody string
}

// fixtureInstance is one task with values. Scoring binds to this, never to the
// corpus: the corpus has no answers to bind to.
type fixtureInstance struct {
	Task          contextTask
	Vars          fixtureVars
	Prompt        string
	ProbeQuery    string
	AnswerMarkers []string
	CueMarker     string
	body          string
	callArgs      string
}

// instantiateTask resolves one task against a generator.
func instantiateTask(t contextTask, rng *rand.Rand) (fixtureInstance, error) {
	vars := newVars(t.Vars, rng)
	inst := fixtureInstance{Task: t, Vars: vars}
	var err error
	for _, pair := range []struct {
		dst  *string
		text string
	}{
		{&inst.Prompt, t.Prompt},
		{&inst.ProbeQuery, t.ProbeQuery},
		{&inst.body, t.PlantBody},
		{&inst.callArgs, t.CallArgs},
	} {
		if *pair.dst, err = instantiate(pair.text, vars); err != nil {
			return fixtureInstance{}, fmt.Errorf("%s: %w", t.ID, err)
		}
	}
	for _, name := range t.AnswerVars {
		value, ok := vars[name]
		if !ok {
			return fixtureInstance{}, fmt.Errorf("%s: answer var %q is not declared", t.ID, name)
		}
		inst.AnswerMarkers = append(inst.AnswerMarkers, value)
	}
	if t.CueVar != "" {
		inst.CueMarker = vars[t.CueVar]
	}
	return inst, nil
}

// plant appends the instantiated target and returns its canonical position.
func (f fixtureInstance) plant(s *agent.Session) int {
	at := len(s.Messages)
	if f.Task.Plant == plantAssistant {
		s.Add(provider.Message{Role: provider.RoleAssistant, Content: f.body})
		return at
	}
	s.Add(provider.Message{Role: provider.RoleAssistant, ToolCalls: []provider.ToolCall{
		{ID: f.Task.CallID, Name: f.Task.CallTool, Arguments: f.callArgs},
	}})
	s.Add(provider.Message{Role: provider.RoleTool, ToolCallID: f.Task.CallID, Name: f.Task.CallTool, Content: f.body})
	return at
}

func code(name, word string) varSpec { return varSpec{Name: name, Kind: varCodename, Word: word} }

func num(name string, lo, hi int) varSpec {
	return varSpec{Name: name, Kind: varInt, Min: lo, Max: hi}
}

func searchTasks() []contextTask {
	return []contextTask{
		{
			ID: "r01-retry-boundary", Experiment: experimentSearch, TargetKind: "assistant_reasoning",
			Prompt:     "What ownership token did we settle on for the retry boundary, and under what tag did we file the reason for not moving retries above framing?",
			ProbeQuery: "retry boundary ownership token framing",
			Vars:       []varSpec{code("token", "cobalt"), code("reason", "stream")},
			AnswerVars: []string{"token", "reason"},
			Plant:      plantAssistant,
			PlantBody: "The retry boundary remains at ingress. The ownership token is {{token}}. " +
				"Moving retries above framing can duplicate a partially received stream, which we filed as {{reason}}.",
		},
		{
			ID: "r02-checkpoint-anchor", Experiment: experimentSearch, TargetKind: "assistant_reasoning",
			Prompt:     "What was the codename of the checkpoint anchor policy, and what tag did we give the reason for not anchoring against digest positions?",
			ProbeQuery: "checkpoint anchor policy digest positions",
			Vars:       []varSpec{code("policy", "northglass"), code("reason", "genlocal")},
			AnswerVars: []string{"policy", "reason"},
			Plant:      plantAssistant,
			PlantBody: "The checkpoint anchor policy is {{policy}}. Digest positions are generation-local — " +
				"filed as {{reason}} — so recovery must anchor against canonical state.",
		},
		{
			ID: "r03-recovery-cjk", Experiment: experimentSearch, TargetKind: "assistant_reasoning_cjk",
			Prompt:     "我们之前定的验证失败恢复策略代号叫什么？当时把“失败 digest 不能变成 baseline”这条登记在哪个标记下？",
			ProbeQuery: "恢复策略 baseline digest 验证失败",
			Vars:       []varSpec{code("policy", "songzhen"), code("rule", "trustbase")},
			AnswerVars: []string{"policy", "rule"},
			Plant:      plantAssistant,
			PlantBody: "恢复策略代号是 {{policy}}。验证失败以后必须保留旧基线，不能把新的 digest 晋升为 baseline，" +
				"因为失败证据不能成为后续比较的可信基准；这条规则登记为 {{rule}}。",
		},
		{
			ID: "r04-lease-fence", Experiment: experimentSearch, TargetKind: "assistant_reasoning",
			Prompt:     "那次 lease fence 检查报告的诊断标记是什么？expected 和 observed epoch 分别是多少？",
			ProbeQuery: "lease fence writer epoch mismatch",
			Vars: []varSpec{
				code("marker", "fence"), num("expected", 400, 900), num("observed", 100, 399),
			},
			AnswerVars: []string{"marker", "expected", "observed"},
			Plant:      plantAssistant,
			PlantBody: "The lease fence check reported {{marker}}: expected writer epoch {{expected}}, " +
				"observed {{observed}}. We left the fence in place.",
		},
		{
			ID: "r05-schema-guard", Experiment: experimentSearch, TargetKind: "assistant_reasoning",
			Prompt:     "当时 schema guard 报告的标记是什么，超预算多少 token？",
			ProbeQuery: "schema guard provider visible surface budget",
			Vars:       []varSpec{code("marker", "amber"), num("over", 120, 480)},
			AnswerVars: []string{"marker", "over"},
			Plant:      plantAssistant,
			PlantBody: "The schema guard reported {{marker}}: the provider-visible tool schema exceeded " +
				"the allowed surface by {{over}} tokens. We trimmed a description instead.",
		},
		{
			ID: "r06-shadow-probe", Experiment: experimentSearch, TargetKind: "assistant_reasoning",
			Prompt:     "我们那次 shadow probe 的 marker、p95 divergence 和 generation 分别是多少？",
			ProbeQuery: "shadow probe divergence generation marker",
			Vars: []varSpec{
				code("marker", "violet"),
				{Name: "divergence", Kind: varDecimal, Min: 110, Max: 890},
				num("generation", 14, 60),
			},
			AnswerVars: []string{"marker", "divergence", "generation"},
			Plant:      plantAssistant,
			PlantBody: "The shadow probe came back as marker={{marker}} with a p95 divergence of " +
				"{{divergence}} at generation={{generation}}, which we judged within tolerance.",
		},
	}
}

func indexTasks() []contextTask {
	return []contextTask{
		{
			ID: "i01-transport-fallback", Experiment: experimentIndex, TargetKind: "tool_input",
			Prompt:     "transport fallback probe 当时报告的 selector 是什么？",
			ProbeQuery: "transport fallback probe selector",
			Vars:       []varSpec{code("cue", "tfprobe"), code("selector", "fern")},
			AnswerVars: []string{"selector"}, CueVar: "cue", CueTier: tierQuarter,
			PlantAfterGen: 4,
			Plant:         plantCall, CallID: "tf1", CallTool: "bash",
			CallArgs:  `{"command":"./scripts/probe {{cue}}"}`,
			PlantBody: "fallback selector = {{selector}}",
		},
		{
			ID: "i02-scheduler-handoff", Experiment: experimentIndex, TargetKind: "tool_input",
			Prompt:     "scheduler handoff 当时记录的 grace 和 marker 是什么？",
			ProbeQuery: "scheduler handoff grace marker",
			Vars: []varSpec{
				code("cue", "schedhandoff"), code("marker", "opal"),
				{Name: "grace", Kind: varDuration, Min: 18, Max: 90},
			},
			AnswerVars: []string{"grace", "marker"}, CueVar: "cue", CueTier: tierQuarter,
			PlantAfterGen: 4,
			Plant:         plantCall, CallID: "sh1", CallTool: "read_file",
			CallArgs:  `{"path":"scratch/{{cue}}.txt"}`,
			PlantBody: "handoff grace = {{grace}}\nmarker = {{marker}}",
		},
		{
			ID: "i03-coalescing", Experiment: experimentIndex, TargetKind: "tool_input",
			Prompt:     "job completion 的 coalescing probe 最后记录的 window 和 marker 是多少？",
			ProbeQuery: "coalescing policy completion window",
			Vars: []varSpec{
				code("cue", "coalescing"), code("marker", "cedar"),
				{Name: "window", Kind: varDuration, Min: 40, Max: 140},
			},
			AnswerVars: []string{"window", "marker"}, CueVar: "cue", CueTier: tierHalf,
			PlantAfterGen: 2,
			Plant:         plantCall, CallID: "cp1", CallTool: "bash",
			CallArgs:  `{"command":"grep {{cue}} scratch/job-notes.log"}`,
			PlantBody: "completion coalesce window = {{window}}\nmarker = {{marker}}",
		},
		{
			ID: "i04-recovery-fence", Experiment: experimentIndex, TargetKind: "tool_input",
			Prompt:     "recovery fence 那次检查报告的 marker 和两个 epoch 是什么？",
			ProbeQuery: "recovery fence epoch check",
			Vars: []varSpec{
				code("cue", "recfence"), code("marker", "quartz"),
				num("expected", 400, 900), num("observed", 100, 399),
			},
			AnswerVars: []string{"marker", "expected", "observed"}, CueVar: "cue", CueTier: tierHalf,
			PlantAfterGen: 2,
			Plant:         plantCall, CallID: "rf1", CallTool: "bash",
			CallArgs:  `{"command":"./scripts/check {{cue}}"}`,
			PlantBody: "{{marker}}\nexpected epoch {{expected}}\nobserved epoch {{observed}}",
		},
		{
			ID: "i05-cache-probe", Experiment: experimentIndex, TargetKind: "tool_input",
			Prompt:     "cache probe 当时记录的 prefix salt 和 scope 是什么？",
			ProbeQuery: "cache probe prefix salt scope",
			Vars:       []varSpec{code("cue", "cacheprobe"), code("salt", "comet"), code("scope", "lineage")},
			AnswerVars: []string{"salt", "scope"}, CueVar: "cue", CueTier: tierDefault,
			PlantAfterGen: 0,
			Plant:         plantCall, CallID: "cb1", CallTool: "read_file",
			CallArgs:  `{"path":"scratch/{{cue}}.txt"}`,
			PlantBody: "prefix salt = {{salt}}\nscope = {{scope}}",
		},
		{
			ID: "i06-dispatch-handoff", Experiment: experimentIndex, TargetKind: "tool_input",
			Prompt:     "dispatch handoff probe 当时确认的 mode 和 marker 是什么？",
			ProbeQuery: "dispatch handoff probe mode",
			Vars:       []varSpec{code("cue", "dispatchhandoff"), code("mode", "trigger"), code("marker", "iris")},
			AnswerVars: []string{"mode", "marker"}, CueVar: "cue", CueTier: tierDefault,
			PlantAfterGen: 0,
			Plant:         plantCall, CallID: "dh1", CallTool: "bash",
			CallArgs:  `{"command":"./scripts/probe {{cue}}"}`,
			PlantBody: "handoff mode = {{mode}}\nmarker = {{marker}}",
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

// boundaryPair is the one comparison an index task is built for: the narrowest
// scale that still addresses its cue, and the next one down. Holding the
// question fixed and moving only the affordance is what four arm averages
// cannot do, since each of those mixes six different questions.
func boundaryPair(tier string) (cue, noCue string) {
	switch tier {
	case tierQuarter:
		return "quarter", "off"
	case tierHalf:
		return "half", "quarter"
	case tierDefault:
		return "default", "half"
	default:
		return "", ""
	}
}

func validateCorpus() error {
	seen := map[string]bool{}
	for _, t := range allTasks() {
		if seen[t.ID] {
			return fmt.Errorf("duplicate task id %q", t.ID)
		}
		seen[t.ID] = true
		if len(t.AnswerVars) == 0 || t.ProbeQuery == "" || t.Prompt == "" || t.PlantBody == "" {
			return fmt.Errorf("%s: a task needs a prompt, a probe query, a body and answer vars", t.ID)
		}
		if t.Experiment == experimentIndex {
			if t.CueVar == "" {
				return fmt.Errorf("%s: an index task needs a cue var", t.ID)
			}
			if v, _ := tierScales(t.CueTier); v == nil {
				return fmt.Errorf("%s: unknown cue tier %q", t.ID, t.CueTier)
			}
		}
		if _, err := instantiateTask(t, seededRand(t.ID, "validate")); err != nil {
			return err
		}
	}
	return nil
}
