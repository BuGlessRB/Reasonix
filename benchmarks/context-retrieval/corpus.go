package main

import (
	"fmt"
	"math/rand"
	"reasonix/internal/state/sessionstore"

	"reasonix/internal/contract/provider"
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

// role places an index task in the efficiency substrate or the stopping corpus.
const (
	roleEfficiency = "efficiency"
	roleStopping   = "stopping"
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
	// Role is roleEfficiency or roleStopping; it decides the batch a task enters.
	Role string
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
func (f fixtureInstance) plant(s *sessionstore.Session) int {
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

// efficiencyTasks is the substrate a study measures on. Every task in it
// qualifies under the corpus bar; the stopping corpus is deliberately excluded,
// because a task carrying post-sufficient stopping behaviour swamps the effect
// an efficiency batch exists to measure.
func efficiencyTasks() []contextTask {
	var out []contextTask
	for _, t := range indexTasks() {
		if t.Role == roleEfficiency {
			out = append(out, t)
		}
	}
	return out
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

// efficiencyIndexTasks is what a study measures on: two tasks per cue tier.
func efficiencyIndexTasks() []contextTask {
	out := indexQuarterTasks()
	out = append(out, indexHalfTasks()...)
	out = append(out, indexDefaultTasks()...)
	return out
}

// indexQuarterTasks holds the quarter-tier literals.
func indexQuarterTasks() []contextTask {
	return []contextTask{
		{
			ID: "i09-lease-epoch", Experiment: experimentIndex, TargetKind: "tool_input", Role: roleEfficiency,
			Prompt:     "lease epoch 检查报告的值是什么？",
			ProbeQuery: "lease epoch value",
			Vars:       []varSpec{code("cue", "leaseepoch"), code("epoch", "carbon")},
			AnswerVars: []string{"epoch"}, CueVar: "cue", CueTier: tierQuarter,
			PlantAfterGen: 4,
			Plant:         plantCall, CallID: "le1", CallTool: "bash",
			CallArgs:  `{"command":"./scripts/lease {{cue}}"}`,
			PlantBody: "lease epoch = {{epoch}}",
		},
		{
			ID: "i10-shard-seal", Experiment: experimentIndex, TargetKind: "tool_input", Role: roleEfficiency,
			Prompt:     "shard seal 记录的 token 是什么？",
			ProbeQuery: "shard seal token",
			Vars:       []varSpec{code("cue", "shardseal"), code("token", "amber")},
			AnswerVars: []string{"token"}, CueVar: "cue", CueTier: tierQuarter,
			PlantAfterGen: 4,
			Plant:         plantCall, CallID: "ss1", CallTool: "bash",
			CallArgs:  `{"command":"./scripts/shard {{cue}}"}`,
			PlantBody: "shard seal token = {{token}}",
		},
		{
			ID: "i15-ingest-lag", Experiment: experimentIndex, TargetKind: "tool_input", Role: roleEfficiency,
			Prompt:     "ingest lag 检查报告的值是多少？",
			ProbeQuery: "ingest lag value",
			Vars:       []varSpec{code("cue", "ingestlag"), {Name: "lag", Kind: varDuration, Min: 12, Max: 240}},
			AnswerVars: []string{"lag"}, CueVar: "cue", CueTier: tierQuarter,
			PlantAfterGen: 4,
			Plant:         plantCall, CallID: "il1", CallTool: "bash",
			CallArgs:  `{"command":"./scripts/ingest {{cue}}"}`,
			PlantBody: "ingest lag = {{lag}}",
		},
		{
			ID: "i16-commit-fence", Experiment: experimentIndex, TargetKind: "tool_input", Role: roleEfficiency,
			Prompt:     "commit fence 记录的 token 是什么？",
			ProbeQuery: "commit fence token",
			Vars:       []varSpec{code("cue", "commitfence"), code("token", "cobalt")},
			AnswerVars: []string{"token"}, CueVar: "cue", CueTier: tierQuarter,
			PlantAfterGen: 4,
			Plant:         plantCall, CallID: "cf2", CallTool: "bash",
			CallArgs:  `{"command":"./scripts/commit {{cue}}"}`,
			PlantBody: "commit fence token = {{token}}",
		},
		{
			ID: "i21-ingest-lag-recall", Experiment: experimentIndex, TargetKind: "tool_input", Role: roleEfficiency,
			Prompt:     "早前会话里执行过 ./scripts/ingest，它打印出的 ingest lag 是多少？",
			ProbeQuery: "ingest lag value",
			Vars:       []varSpec{code("cue", "ingestlag"), {Name: "lag", Kind: varDuration, Min: 12, Max: 240}},
			AnswerVars: []string{"lag"}, CueVar: "cue", CueTier: tierQuarter,
			PlantAfterGen: 4,
			Plant:         plantCall, CallID: "il1", CallTool: "bash",
			CallArgs:  `{"command":"./scripts/ingest {{cue}}"}`,
			PlantBody: "ingest lag = {{lag}}",
		},
		{
			ID: "i23-routelag-verbatim", Experiment: experimentIndex, TargetKind: "tool_input", Role: roleEfficiency,
			Prompt:     "早前会话里执行 ./scripts/routelag 时，输出里那一行原文是什么？",
			ProbeQuery: "routelag output line",
			Vars:       []varSpec{code("cue", "routelag"), {Name: "lag", Kind: varDuration, Min: 15, Max: 300}},
			AnswerVars: []string{"lag"}, CueVar: "cue", CueTier: tierQuarter,
			PlantAfterGen: 4,
			Plant:         plantCall, CallID: "rl7", CallTool: "bash",
			CallArgs:  `{"command":"./scripts/routelag {{cue}}"}`,
			PlantBody: "routelag lag = {{lag}}",
		}, {
			ID: "i31-leaseaudit", Experiment: experimentIndex, TargetKind: "tool_input", Role: roleEfficiency,
			Prompt:     "早前会话里执行过 ./scripts/leaseaudit，它打印出的 lease epoch 是多少？",
			ProbeQuery: "lease epoch epoch",
			Vars:       []varSpec{code("cue", "leaseaudit"), code("epoch", "cobalt")},
			AnswerVars: []string{"epoch"}, CueVar: "cue", CueTier: tierQuarter,
			PlantAfterGen: 4,
			Plant:         plantCall, CallID: "p31", CallTool: "bash",
			CallArgs:  `{"command":"./scripts/leaseaudit {{cue}}"}`,
			PlantBody: "epoch = {{epoch}}",
		},
		{
			ID: "i32-shardlease", Experiment: experimentIndex, TargetKind: "tool_input", Role: roleEfficiency,
			Prompt:     "早前会话里执行过 ./scripts/shardlease，它打印出的 lease epoch 是多少？",
			ProbeQuery: "lease epoch epoch",
			Vars:       []varSpec{code("cue", "shardlease"), code("epoch", "cobalt")},
			AnswerVars: []string{"epoch"}, CueVar: "cue", CueTier: tierQuarter,
			PlantAfterGen: 4,
			Plant:         plantCall, CallID: "p32", CallTool: "bash",
			CallArgs:  `{"command":"./scripts/shardlease {{cue}}"}`,
			PlantBody: "epoch = {{epoch}}",
		},
		{
			ID: "i33-quorumseal", Experiment: experimentIndex, TargetKind: "tool_input", Role: roleEfficiency,
			Prompt:     "早前会话里执行过 ./scripts/quorumseal，它打印出的 lease epoch 是多少？",
			ProbeQuery: "lease epoch epoch",
			Vars:       []varSpec{code("cue", "quorumseal"), code("epoch", "cobalt")},
			AnswerVars: []string{"epoch"}, CueVar: "cue", CueTier: tierQuarter,
			PlantAfterGen: 4,
			Plant:         plantCall, CallID: "p33", CallTool: "bash",
			CallArgs:  `{"command":"./scripts/quorumseal {{cue}}"}`,
			PlantBody: "epoch = {{epoch}}",
		},
		{
			ID: "i34-epochguard", Experiment: experimentIndex, TargetKind: "tool_input", Role: roleEfficiency,
			Prompt:     "早前会话里执行过 ./scripts/epochguard，它打印出的 lease epoch 是多少？",
			ProbeQuery: "lease epoch epoch",
			Vars:       []varSpec{code("cue", "epochguard"), code("epoch", "cobalt")},
			AnswerVars: []string{"epoch"}, CueVar: "cue", CueTier: tierQuarter,
			PlantAfterGen: 4,
			Plant:         plantCall, CallID: "p34", CallTool: "bash",
			CallArgs:  `{"command":"./scripts/epochguard {{cue}}"}`,
			PlantBody: "epoch = {{epoch}}",
		},
		{
			ID: "i35-flushtick", Experiment: experimentIndex, TargetKind: "tool_input", Role: roleEfficiency,
			Prompt:     "早前会话里执行过 ./scripts/flushtick，它打印出的 lease epoch 是多少？",
			ProbeQuery: "lease epoch epoch",
			Vars:       []varSpec{code("cue", "flushtick"), code("epoch", "cobalt")},
			AnswerVars: []string{"epoch"}, CueVar: "cue", CueTier: tierQuarter,
			PlantAfterGen: 4,
			Plant:         plantCall, CallID: "p35", CallTool: "bash",
			CallArgs:  `{"command":"./scripts/flushtick {{cue}}"}`,
			PlantBody: "epoch = {{epoch}}",
		},
		{
			ID: "i36-replicaslot", Experiment: experimentIndex, TargetKind: "tool_input", Role: roleEfficiency,
			Prompt:     "早前会话里执行过 ./scripts/replicaslot，它打印出的 lease epoch 是多少？",
			ProbeQuery: "lease epoch epoch",
			Vars:       []varSpec{code("cue", "replicaslot"), code("epoch", "cobalt")},
			AnswerVars: []string{"epoch"}, CueVar: "cue", CueTier: tierQuarter,
			PlantAfterGen: 4,
			Plant:         plantCall, CallID: "p36", CallTool: "bash",
			CallArgs:  `{"command":"./scripts/replicaslot {{cue}}"}`,
			PlantBody: "epoch = {{epoch}}",
		},
		{
			ID: "i37-dispatchlag", Experiment: experimentIndex, TargetKind: "tool_input", Role: roleEfficiency,
			Prompt:     "早前会话里执行过 ./scripts/dispatchlag，它打印出的 lease epoch 是多少？",
			ProbeQuery: "lease epoch epoch",
			Vars:       []varSpec{code("cue", "dispatchlag"), code("epoch", "cobalt")},
			AnswerVars: []string{"epoch"}, CueVar: "cue", CueTier: tierQuarter,
			PlantAfterGen: 4,
			Plant:         plantCall, CallID: "p37", CallTool: "bash",
			CallArgs:  `{"command":"./scripts/dispatchlag {{cue}}"}`,
			PlantBody: "epoch = {{epoch}}",
		},
		{
			ID: "i38-recoverytag", Experiment: experimentIndex, TargetKind: "tool_input", Role: roleEfficiency,
			Prompt:     "早前会话里执行过 ./scripts/recoverytag，它打印出的 lease epoch 是多少？",
			ProbeQuery: "lease epoch epoch",
			Vars:       []varSpec{code("cue", "recoverytag"), code("epoch", "cobalt")},
			AnswerVars: []string{"epoch"}, CueVar: "cue", CueTier: tierQuarter,
			PlantAfterGen: 4,
			Plant:         plantCall, CallID: "p38", CallTool: "bash",
			CallArgs:  `{"command":"./scripts/recoverytag {{cue}}"}`,
			PlantBody: "epoch = {{epoch}}",
		},
	}
}

// indexHalfTasks holds the half-tier literals.
func indexHalfTasks() []contextTask {
	return []contextTask{
		{
			ID: "i07-stream-latch", Experiment: experimentIndex, TargetKind: "tool_input", Role: roleEfficiency,
			Prompt:     "stream latch 检查当时记录的 hold 是什么？",
			ProbeQuery: "stream latch hold",
			Vars: []varSpec{
				code("cue", "streamlatch"),
				{Name: "hold", Kind: varDuration, Min: 24, Max: 160},
			},
			AnswerVars: []string{"hold"}, CueVar: "cue", CueTier: tierHalf,
			PlantAfterGen: 2,
			Plant:         plantCall, CallID: "sl1", CallTool: "bash",
			CallArgs:  `{"command":"./scripts/stream {{cue}}"}`,
			PlantBody: "stream latch hold = {{hold}}",
		},
		{
			ID: "i08-quota-refill", Experiment: experimentIndex, TargetKind: "tool_input", Role: roleEfficiency,
			Prompt:     "quota refill probe 当时确认的 bucket 和 refill 是什么？",
			ProbeQuery: "quota refill bucket",
			Vars: []varSpec{
				code("cue", "quotarefill"), code("bucket", "sable"),
				num("refill", 30, 480),
			},
			AnswerVars: []string{"bucket", "refill"}, CueVar: "cue", CueTier: tierHalf,
			PlantAfterGen: 2,
			Plant:         plantCall, CallID: "qr1", CallTool: "bash",
			CallArgs:  `{"command":"./scripts/quota {{cue}}"}`,
			PlantBody: "refill bucket = {{bucket}}\nrefill units = {{refill}}",
		},
		{
			ID: "i17-route-budget", Experiment: experimentIndex, TargetKind: "tool_input", Role: roleEfficiency,
			Prompt:     "route budget 记录的是多少？",
			ProbeQuery: "route budget number",
			Vars:       []varSpec{code("cue", "routebudget"), num("budget", 1200, 9800)},
			AnswerVars: []string{"budget"}, CueVar: "cue", CueTier: tierHalf,
			PlantAfterGen: 2,
			Plant:         plantCall, CallID: "rb3", CallTool: "bash",
			CallArgs:  `{"command":"./scripts/route {{cue}}"}`,
			PlantBody: "route budget = {{budget}}",
		},
		{
			ID: "i18-salvage-epoch", Experiment: experimentIndex, TargetKind: "tool_input", Role: roleEfficiency,
			Prompt:     "salvage epoch 检查报告的值是什么？",
			ProbeQuery: "salvage epoch value",
			Vars:       []varSpec{code("cue", "salvageepoch"), code("epoch", "flint")},
			AnswerVars: []string{"epoch"}, CueVar: "cue", CueTier: tierHalf,
			PlantAfterGen: 2,
			Plant:         plantCall, CallID: "se4", CallTool: "bash",
			CallArgs:  `{"command":"./scripts/salvage {{cue}}"}`,
			PlantBody: "salvage epoch = {{epoch}}",
		},
		{
			ID: "i20-salvage-epoch-recall", Experiment: experimentIndex, TargetKind: "tool_input", Role: roleEfficiency,
			Prompt:     "早前会话里执行过 ./scripts/salvage，它打印出的 salvage epoch 是多少？",
			ProbeQuery: "salvage epoch value",
			Vars:       []varSpec{code("cue", "salvageepoch"), code("epoch", "flint")},
			AnswerVars: []string{"epoch"}, CueVar: "cue", CueTier: tierHalf,
			PlantAfterGen: 2,
			Plant:         plantCall, CallID: "se4", CallTool: "bash",
			CallArgs:  `{"command":"./scripts/salvage {{cue}}"}`,
			PlantBody: "salvage epoch = {{epoch}}",
		},
		{
			ID: "i22-commitsync-verbatim", Experiment: experimentIndex, TargetKind: "tool_input", Role: roleEfficiency,
			Prompt:     "早前会话里执行 ./scripts/commitsync 时，输出里那一行原文是什么？",
			ProbeQuery: "commitsync output line",
			Vars:       []varSpec{code("cue", "commitsync"), code("rev", "amber")},
			AnswerVars: []string{"rev"}, CueVar: "cue", CueTier: tierHalf,
			PlantAfterGen: 2,
			Plant:         plantCall, CallID: "cs6", CallTool: "bash",
			CallArgs:  `{"command":"./scripts/commitsync {{cue}}"}`,
			PlantBody: "commitsync rev = {{rev}}",
		}, {
			ID: "i39-routequota", Experiment: experimentIndex, TargetKind: "tool_input", Role: roleEfficiency,
			Prompt:     "早前会话里执行过 ./scripts/routequota，它打印出的 route quota 是多少？",
			ProbeQuery: "route quota quota",
			Vars:       []varSpec{code("cue", "routequota"), num("quota", 1200, 9800)},
			AnswerVars: []string{"quota"}, CueVar: "cue", CueTier: tierHalf,
			PlantAfterGen: 2,
			Plant:         plantCall, CallID: "p39", CallTool: "bash",
			CallArgs:  `{"command":"./scripts/routequota {{cue}}"}`,
			PlantBody: "quota = {{quota}}",
		},
		{
			ID: "i40-tokenfence", Experiment: experimentIndex, TargetKind: "tool_input", Role: roleEfficiency,
			Prompt:     "早前会话里执行过 ./scripts/tokenfence，它打印出的 route quota 是多少？",
			ProbeQuery: "route quota quota",
			Vars:       []varSpec{code("cue", "tokenfence"), num("quota", 1200, 9800)},
			AnswerVars: []string{"quota"}, CueVar: "cue", CueTier: tierHalf,
			PlantAfterGen: 2,
			Plant:         plantCall, CallID: "p40", CallTool: "bash",
			CallArgs:  `{"command":"./scripts/tokenfence {{cue}}"}`,
			PlantBody: "quota = {{quota}}",
		},
		{
			ID: "i41-ingestseal", Experiment: experimentIndex, TargetKind: "tool_input", Role: roleEfficiency,
			Prompt:     "早前会话里执行过 ./scripts/ingestseal，它打印出的 route quota 是多少？",
			ProbeQuery: "route quota quota",
			Vars:       []varSpec{code("cue", "ingestseal"), num("quota", 1200, 9800)},
			AnswerVars: []string{"quota"}, CueVar: "cue", CueTier: tierHalf,
			PlantAfterGen: 2,
			Plant:         plantCall, CallID: "p41", CallTool: "bash",
			CallArgs:  `{"command":"./scripts/ingestseal {{cue}}"}`,
			PlantBody: "quota = {{quota}}",
		},
		{
			ID: "i42-salvagehold", Experiment: experimentIndex, TargetKind: "tool_input", Role: roleEfficiency,
			Prompt:     "早前会话里执行过 ./scripts/salvagehold，它打印出的 route quota 是多少？",
			ProbeQuery: "route quota quota",
			Vars:       []varSpec{code("cue", "salvagehold"), num("quota", 1200, 9800)},
			AnswerVars: []string{"quota"}, CueVar: "cue", CueTier: tierHalf,
			PlantAfterGen: 2,
			Plant:         plantCall, CallID: "p42", CallTool: "bash",
			CallArgs:  `{"command":"./scripts/salvagehold {{cue}}"}`,
			PlantBody: "quota = {{quota}}",
		},
		{
			ID: "i43-compactlag", Experiment: experimentIndex, TargetKind: "tool_input", Role: roleEfficiency,
			Prompt:     "早前会话里执行过 ./scripts/compactlag，它打印出的 route quota 是多少？",
			ProbeQuery: "route quota quota",
			Vars:       []varSpec{code("cue", "compactlag"), num("quota", 1200, 9800)},
			AnswerVars: []string{"quota"}, CueVar: "cue", CueTier: tierHalf,
			PlantAfterGen: 2,
			Plant:         plantCall, CallID: "p43", CallTool: "bash",
			CallArgs:  `{"command":"./scripts/compactlag {{cue}}"}`,
			PlantBody: "quota = {{quota}}",
		},
		{
			ID: "i44-handoffttl", Experiment: experimentIndex, TargetKind: "tool_input", Role: roleEfficiency,
			Prompt:     "早前会话里执行过 ./scripts/handoffttl，它打印出的 route quota 是多少？",
			ProbeQuery: "route quota quota",
			Vars:       []varSpec{code("cue", "handoffttl"), num("quota", 1200, 9800)},
			AnswerVars: []string{"quota"}, CueVar: "cue", CueTier: tierHalf,
			PlantAfterGen: 2,
			Plant:         plantCall, CallID: "p44", CallTool: "bash",
			CallArgs:  `{"command":"./scripts/handoffttl {{cue}}"}`,
			PlantBody: "quota = {{quota}}",
		},
		{
			ID: "i45-coalesceid", Experiment: experimentIndex, TargetKind: "tool_input", Role: roleEfficiency,
			Prompt:     "早前会话里执行过 ./scripts/coalesceid，它打印出的 route quota 是多少？",
			ProbeQuery: "route quota quota",
			Vars:       []varSpec{code("cue", "coalesceid"), num("quota", 1200, 9800)},
			AnswerVars: []string{"quota"}, CueVar: "cue", CueTier: tierHalf,
			PlantAfterGen: 2,
			Plant:         plantCall, CallID: "p45", CallTool: "bash",
			CallArgs:  `{"command":"./scripts/coalesceid {{cue}}"}`,
			PlantBody: "quota = {{quota}}",
		},
		{
			ID: "i46-probeepoch", Experiment: experimentIndex, TargetKind: "tool_input", Role: roleEfficiency,
			Prompt:     "早前会话里执行过 ./scripts/probeepoch，它打印出的 route quota 是多少？",
			ProbeQuery: "route quota quota",
			Vars:       []varSpec{code("cue", "probeepoch"), num("quota", 1200, 9800)},
			AnswerVars: []string{"quota"}, CueVar: "cue", CueTier: tierHalf,
			PlantAfterGen: 2,
			Plant:         plantCall, CallID: "p46", CallTool: "bash",
			CallArgs:  `{"command":"./scripts/probeepoch {{cue}}"}`,
			PlantBody: "quota = {{quota}}",
		},
	}
}

// indexDefaultTasks holds the default-tier literals.
func indexDefaultTasks() []contextTask {
	return []contextTask{
		{
			ID: "i13-replica-tag", Experiment: experimentIndex, TargetKind: "tool_input", Role: roleEfficiency,
			Prompt:     "replica tag probe 报告的是哪个 tag？",
			ProbeQuery: "replica tag name",
			Vars:       []varSpec{code("cue", "replicatag"), code("tag", "indigo")},
			AnswerVars: []string{"tag"}, CueVar: "cue", CueTier: tierDefault,
			PlantAfterGen: 0,
			Plant:         plantCall, CallID: "rt1", CallTool: "bash",
			CallArgs:  `{"command":"./scripts/replica {{cue}}"}`,
			PlantBody: "replica tag = {{tag}}",
		},
		{
			ID: "i14-tombstone-ttl", Experiment: experimentIndex, TargetKind: "tool_input", Role: roleEfficiency,
			Prompt:     "tombstone ttl 记录的是多少？",
			ProbeQuery: "tombstone ttl expiry",
			Vars: []varSpec{
				code("cue", "tombstonettl"),
				{Name: "ttl", Kind: varDuration, Min: 30, Max: 600},
			},
			AnswerVars: []string{"ttl"}, CueVar: "cue", CueTier: tierDefault,
			PlantAfterGen: 0,
			Plant:         plantCall, CallID: "tb1", CallTool: "bash",
			CallArgs:  `{"command":"./scripts/tombstone {{cue}}"}`,
			PlantBody: "tombstone ttl = {{ttl}}",
		},
		{
			ID: "i19-tenant-quota", Experiment: experimentIndex, TargetKind: "tool_input", Role: roleEfficiency,
			Prompt:     "tenant quota 当时记录的是多少？",
			ProbeQuery: "tenant quota amount",
			Vars:       []varSpec{code("cue", "tenantquota"), {Name: "quota", Kind: varDuration, Min: 30, Max: 600}},
			AnswerVars: []string{"quota"}, CueVar: "cue", CueTier: tierDefault,
			PlantAfterGen: 0,
			Plant:         plantCall, CallID: "tq5", CallTool: "bash",
			CallArgs:  `{"command":"./scripts/tenant {{cue}}"}`,
			PlantBody: "tenant quota = {{quota}}",
		}, {
			ID: "i47-tenantlease", Experiment: experimentIndex, TargetKind: "tool_input", Role: roleEfficiency,
			Prompt:     "早前会话里执行过 ./scripts/tenantlease，它打印出的 tenant lease 是多少？",
			ProbeQuery: "tenant lease lease",
			Vars:       []varSpec{code("cue", "tenantlease"), {Name: "lease", Kind: varDuration, Min: 15, Max: 300}},
			AnswerVars: []string{"lease"}, CueVar: "cue", CueTier: tierDefault,
			PlantAfterGen: 0,
			Plant:         plantCall, CallID: "p47", CallTool: "bash",
			CallArgs:  `{"command":"./scripts/tenantlease {{cue}}"}`,
			PlantBody: "lease = {{lease}}",
		},
		{
			ID: "i48-fencewindow", Experiment: experimentIndex, TargetKind: "tool_input", Role: roleEfficiency,
			Prompt:     "早前会话里执行过 ./scripts/fencewindow，它打印出的 tenant lease 是多少？",
			ProbeQuery: "tenant lease lease",
			Vars:       []varSpec{code("cue", "fencewindow"), {Name: "lease", Kind: varDuration, Min: 15, Max: 300}},
			AnswerVars: []string{"lease"}, CueVar: "cue", CueTier: tierDefault,
			PlantAfterGen: 0,
			Plant:         plantCall, CallID: "p48", CallTool: "bash",
			CallArgs:  `{"command":"./scripts/fencewindow {{cue}}"}`,
			PlantBody: "lease = {{lease}}",
		},
		{
			ID: "i49-quotaepoch", Experiment: experimentIndex, TargetKind: "tool_input", Role: roleEfficiency,
			Prompt:     "早前会话里执行过 ./scripts/quotaepoch，它打印出的 tenant lease 是多少？",
			ProbeQuery: "tenant lease lease",
			Vars:       []varSpec{code("cue", "quotaepoch"), {Name: "lease", Kind: varDuration, Min: 15, Max: 300}},
			AnswerVars: []string{"lease"}, CueVar: "cue", CueTier: tierDefault,
			PlantAfterGen: 0,
			Plant:         plantCall, CallID: "p49", CallTool: "bash",
			CallArgs:  `{"command":"./scripts/quotaepoch {{cue}}"}`,
			PlantBody: "lease = {{lease}}",
		},
		{
			ID: "i50-salvagedepth", Experiment: experimentIndex, TargetKind: "tool_input", Role: roleEfficiency,
			Prompt:     "早前会话里执行过 ./scripts/salvagedepth，它打印出的 tenant lease 是多少？",
			ProbeQuery: "tenant lease lease",
			Vars:       []varSpec{code("cue", "salvagedepth"), {Name: "lease", Kind: varDuration, Min: 15, Max: 300}},
			AnswerVars: []string{"lease"}, CueVar: "cue", CueTier: tierDefault,
			PlantAfterGen: 0,
			Plant:         plantCall, CallID: "p50", CallTool: "bash",
			CallArgs:  `{"command":"./scripts/salvagedepth {{cue}}"}`,
			PlantBody: "lease = {{lease}}",
		},
		{
			ID: "i51-routedelay", Experiment: experimentIndex, TargetKind: "tool_input", Role: roleEfficiency,
			Prompt:     "早前会话里执行过 ./scripts/routedelay，它打印出的 tenant lease 是多少？",
			ProbeQuery: "tenant lease lease",
			Vars:       []varSpec{code("cue", "routedelay"), {Name: "lease", Kind: varDuration, Min: 15, Max: 300}},
			AnswerVars: []string{"lease"}, CueVar: "cue", CueTier: tierDefault,
			PlantAfterGen: 0,
			Plant:         plantCall, CallID: "p51", CallTool: "bash",
			CallArgs:  `{"command":"./scripts/routedelay {{cue}}"}`,
			PlantBody: "lease = {{lease}}",
		},
		{
			ID: "i52-sealkey", Experiment: experimentIndex, TargetKind: "tool_input", Role: roleEfficiency,
			Prompt:     "早前会话里执行过 ./scripts/sealkey，它打印出的 tenant lease 是多少？",
			ProbeQuery: "tenant lease lease",
			Vars:       []varSpec{code("cue", "sealkey"), {Name: "lease", Kind: varDuration, Min: 15, Max: 300}},
			AnswerVars: []string{"lease"}, CueVar: "cue", CueTier: tierDefault,
			PlantAfterGen: 0,
			Plant:         plantCall, CallID: "p52", CallTool: "bash",
			CallArgs:  `{"command":"./scripts/sealkey {{cue}}"}`,
			PlantBody: "lease = {{lease}}",
		},
		{
			ID: "i53-lagbudget", Experiment: experimentIndex, TargetKind: "tool_input", Role: roleEfficiency,
			Prompt:     "早前会话里执行过 ./scripts/lagbudget，它打印出的 tenant lease 是多少？",
			ProbeQuery: "tenant lease lease",
			Vars:       []varSpec{code("cue", "lagbudget"), {Name: "lease", Kind: varDuration, Min: 15, Max: 300}},
			AnswerVars: []string{"lease"}, CueVar: "cue", CueTier: tierDefault,
			PlantAfterGen: 0,
			Plant:         plantCall, CallID: "p53", CallTool: "bash",
			CallArgs:  `{"command":"./scripts/lagbudget {{cue}}"}`,
			PlantBody: "lease = {{lease}}",
		},
		{
			ID: "i54-ttlmark", Experiment: experimentIndex, TargetKind: "tool_input", Role: roleEfficiency,
			Prompt:     "早前会话里执行过 ./scripts/ttlmark，它打印出的 tenant lease 是多少？",
			ProbeQuery: "tenant lease lease",
			Vars:       []varSpec{code("cue", "ttlmark"), {Name: "lease", Kind: varDuration, Min: 15, Max: 300}},
			AnswerVars: []string{"lease"}, CueVar: "cue", CueTier: tierDefault,
			PlantAfterGen: 0,
			Plant:         plantCall, CallID: "p54", CallTool: "bash",
			CallArgs:  `{"command":"./scripts/ttlmark {{cue}}"}`,
			PlantBody: "lease = {{lease}}",
		},
	}
}

// stoppingIndexTasks is the frozen study's six tasks, kept out of efficiency
// batches and measurable on their own with -task <id>.
func stoppingIndexTasks() []contextTask {
	return []contextTask{
		{
			ID: "i01-transport-fallback", Experiment: experimentIndex, TargetKind: "tool_input", Role: roleStopping,
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
			ID: "i02-scheduler-handoff", Experiment: experimentIndex, TargetKind: "tool_input", Role: roleStopping,
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
			ID: "i03-coalescing", Experiment: experimentIndex, TargetKind: "tool_input", Role: roleStopping,
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
			ID: "i04-recovery-fence", Experiment: experimentIndex, TargetKind: "tool_input", Role: roleStopping,
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
			ID: "i05-cache-probe", Experiment: experimentIndex, TargetKind: "tool_input", Role: roleStopping,
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
			ID: "i06-dispatch-handoff", Experiment: experimentIndex, TargetKind: "tool_input", Role: roleStopping,
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

// indexTasks is the whole index corpus, the efficiency substrate first.
func indexTasks() []contextTask {
	out := efficiencyIndexTasks()
	return append(out, stoppingIndexTasks()...)
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
			if t.Role != roleEfficiency && t.Role != roleStopping {
				return fmt.Errorf("%s: an index task needs a role (%s or %s)", t.ID, roleEfficiency, roleStopping)
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
