package agent

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"sync"
	"testing"

	"reasonix/internal/event"
	"reasonix/internal/provider"
	"reasonix/internal/tool"
)

func TestNormalizeFanoutItemsKeepsSubjectsAndRefusesProse(t *testing.T) {
	got, err := normalizeFanoutItems([]string{" a.py ", "", "b.py", "   "})
	if err != nil {
		t.Fatalf("plain subjects: %v", err)
	}
	if !slices.Equal(got, []string{"a.py", "b.py"}) {
		t.Errorf("normalized = %v, want the two real subjects trimmed", got)
	}
	if _, err := normalizeFanoutItems([]string{"a.py", "a.py"}); err == nil ||
		!strings.Contains(err.Error(), "twice") {
		t.Errorf("duplicate err = %v, want one naming the repeat", err)
	}
	if _, err := normalizeFanoutItems([]string{strings.Repeat("x", fanoutMaxItemBytes+1)}); err == nil ||
		!strings.Contains(err.Error(), "names one subject") {
		t.Errorf("prose err = %v, want one saying an entry names a subject", err)
	}
	over := make([]string, fanoutHardMaxItems+1)
	for i := range over {
		over[i] = string(rune('a'+i%26)) + string(rune('0'+i/26))
	}
	if _, err := normalizeFanoutItems(over); err == nil || !strings.Contains(err.Error(), "narrow the subjects") {
		t.Errorf("over-cap err = %v, want the host ceiling named", err)
	}
}

func TestSubmitItemsRefusesOutsideAFanoutSource(t *testing.T) {
	_, err := (&SubmitItemsTool{}).Execute(context.Background(), json.RawMessage(`{"items":["a"]}`))
	if err == nil || !strings.Contains(err.Error(), "maps over its result") {
		t.Errorf("err = %v, want a refusal explaining nothing maps over this run", err)
	}
}

func TestFanoutItemShapeRules(t *testing.T) {
	for name, tc := range map[string]struct {
		item    fleetTaskItem
		enabled bool
		want    string
	}{
		"for_each with adopt_ref": {fleetTaskItem{ForEach: "a", AdoptRef: "sa_x"}, true, "mutually exclusive"},
		"max_items alone":         {fleetTaskItem{Prompt: "go", MaxItems: 4}, true, "bounds nothing"},
		"writer map":              {fleetTaskItem{ForEach: "a", Prompt: "go"}, true, "must set read_only"},
		"read-only map is ok":     {fleetTaskItem{ForEach: "a", Prompt: "go", ReadOnly: true, MaxItems: 4}, true, ""},
		"plain item is ok":        {fleetTaskItem{Prompt: "go"}, true, ""},
		"mapped while off":        {fleetTaskItem{ForEach: "a", Prompt: "go", ReadOnly: true}, false, "not enabled in this build"},
		"plain item while off":    {fleetTaskItem{Prompt: "go"}, false, ""},
	} {
		err := validateFanoutItemShape(0, tc.item, tc.enabled)
		switch {
		case tc.want == "" && err != nil:
			t.Errorf("%s: unexpected err %v", name, err)
		case tc.want != "" && (err == nil || !strings.Contains(err.Error(), tc.want)):
			t.Errorf("%s: err = %v, want one mentioning %q", name, err, tc.want)
		}
	}
}

// for_each orders like depends_on, and naming the same task both ways is still
// one edge — two would leave the mapped node waiting for a second result that
// is never published.
func TestForEachIsOneOrderingEdge(t *testing.T) {
	plan, err := planFor(t,
		fleetTaskItem{ID: "scan"},
		fleetTaskItem{ID: "review", ForEach: "scan", DependsOn: []string{"scan"}},
	)
	if err != nil {
		t.Fatal(err)
	}
	if got := plan.deps[1]; len(got) != 1 || got[0] != 0 {
		t.Fatalf("deps = %v, want exactly one edge to scan", got)
	}
	if plan.fanoutSource[1] != 0 || plan.fanoutSource[0] != -1 {
		t.Fatalf("fanoutSource = %v, want only review mapped over scan", plan.fanoutSource)
	}
	if !plan.ordered(0, 1) {
		t.Error("a mapped node must be ordered against the node naming its subjects")
	}
	if _, err := planFor(t, fleetTaskItem{ID: "a", ForEach: "nope"}, fleetTaskItem{ID: "b"}); err == nil ||
		!strings.Contains(err.Error(), "for_each") {
		t.Errorf("err = %v, want an unknown for_each id refused by name", err)
	}
}

func TestFanoutSourceMustBeAbleToSubmit(t *testing.T) {
	plan, err := planFor(t,
		fleetTaskItem{ID: "scan"},
		fleetTaskItem{ID: "review", ForEach: "scan", ReadOnly: true},
		fleetTaskItem{ID: "second", ForEach: "review", ReadOnly: true},
	)
	if err != nil {
		t.Fatal(err)
	}
	items := []fleetTaskItem{{ID: "scan"}, {ID: "review", ForEach: "scan"}, {ID: "second", ForEach: "review"}}
	if err := validateFanoutSources(items, plan, map[int]adoptedItem{0: {}}); err == nil ||
		!strings.Contains(err.Error(), "adopted") {
		t.Errorf("err = %v, want an adopted source refused: nothing ran to submit subjects", err)
	}
	if err := validateFanoutSources(items, plan, nil); err == nil ||
		!strings.Contains(err.Error(), "aggregate") {
		t.Errorf("err = %v, want a mapped source refused: it answers with an aggregate", err)
	}
}

// The effect at its boundary: a node whose width nobody knew at preflight runs
// one child per submitted subject, and its dependent starts from the aggregate.
func TestFleetFanoutMapsOverSubmittedSubjects(t *testing.T) {
	prov := &fanoutScriptProvider{subjects: []string{"h01.py", "h07.py", "h12.py"}}
	fleet := newFanoutFleet(t, prov)
	ctx := withCallContext(context.Background(), "fleet-call", event.Discard, nil, false)

	out, err := fleet.Execute(ctx, json.RawMessage(`{"tasks":[
		{"id":"scan","prompt":"SCAN find the handlers that matter","read_only":true},
		{"id":"review","for_each":"scan","read_only":true,"prompt":"REVIEW this handler"},
		{"id":"report","depends_on":["review"],"prompt":"REPORT summarize the reviews","read_only":true}
	]}`))
	if err != nil {
		t.Fatalf("fleet: %v\n%s", err, out)
	}

	if got := prov.subjectsSeen(); !slices.Equal(got, []string{"h01.py", "h07.py", "h12.py"}) {
		t.Errorf("mapped children saw %v, want one run per submitted subject", got)
	}
	for _, want := range []string{"Mapped 3 subject(s)", "h01.py", "h07.py", "h12.py", "reviewed h07.py"} {
		if !strings.Contains(out, want) {
			t.Errorf("aggregate is missing %q:\n%s", want, out)
		}
	}
	report := prov.turnFor("REPORT")
	if report == "" {
		t.Fatal("the dependent of a mapped node never ran")
	}
	for _, want := range []string{"<upstream-results", "from review", "reviewed h01.py"} {
		if !strings.Contains(report, want) {
			t.Errorf("the dependent's opening turn is missing %q:\n%s", want, report)
		}
	}
}

func TestFleetFanoutFailsWhenTheSourceNeverSubmits(t *testing.T) {
	prov := &fanoutScriptProvider{subjects: []string{"a.py"}, skipSubmit: true}
	out, err := newFanoutFleet(t, prov).Execute(
		withCallContext(context.Background(), "fleet-call", event.Discard, nil, false),
		json.RawMessage(`{"tasks":[
			{"id":"scan","prompt":"SCAN find them","read_only":true},
			{"id":"review","for_each":"scan","read_only":true,"prompt":"REVIEW this one"}
		]}`))
	if err != nil {
		t.Fatalf("the fleet itself must complete and report the item: %v", err)
	}
	if !strings.Contains(out, "without calling submit_items") {
		t.Errorf("aggregate must name the reason the mapped node failed:\n%s", out)
	}
	if got := prov.subjectsSeen(); len(got) != 0 {
		t.Errorf("mapped children ran without subjects: %v", got)
	}
}

func TestFleetFanoutRefusesMoreSubjectsThanTheCeiling(t *testing.T) {
	prov := &fanoutScriptProvider{subjects: []string{"a.py", "b.py", "c.py"}}
	out, err := newFanoutFleet(t, prov).Execute(
		withCallContext(context.Background(), "fleet-call", event.Discard, nil, false),
		json.RawMessage(`{"tasks":[
			{"id":"scan","prompt":"SCAN find them","read_only":true},
			{"id":"review","for_each":"scan","read_only":true,"max_items":2,"prompt":"REVIEW this one"}
		]}`))
	if err != nil {
		t.Fatalf("fleet: %v", err)
	}
	if !strings.Contains(out, "ceiling of 2") {
		t.Errorf("aggregate must say the width ceiling stopped it:\n%s", out)
	}
	if got := prov.subjectsSeen(); len(got) != 0 {
		t.Errorf("subjects ran past the ceiling: %v", got)
	}
}

// Default off, and off means invisible: the field is not on the surface, the
// description does not advertise it, and a call that names it is refused by the
// setting rather than half-honoured. Mapping was measured at about twice the
// tokens of the single-task form for no gain in solved rate, so a capability
// the model can see is one it would reach for at that price.
func TestFanoutIsOffTheSurfaceUntilEnabled(t *testing.T) {
	off := &FleetTool{}
	if strings.Contains(string(off.Schema()), "for_each") {
		t.Error("for_each is on the default schema surface")
	}
	if strings.Contains(off.Description(), "for_each") {
		t.Error("the default description advertises a field the schema does not carry")
	}
	on := (&FleetTool{}).WithFanout(true)
	if !strings.Contains(string(on.Schema()), "for_each") || !strings.Contains(on.Description(), "for_each") {
		t.Error("enabling fan-out must put the field on both the schema and the description")
	}

	prov := &fanoutScriptProvider{subjects: []string{"a.py"}}
	fleet := newFanoutFleet(t, prov).WithFanout(false)
	_, err := fleet.Execute(withCallContext(context.Background(), "fleet-call", event.Discard, nil, false),
		json.RawMessage(`{"tasks":[
			{"id":"scan","prompt":"SCAN find them","read_only":true},
			{"id":"review","for_each":"scan","read_only":true,"prompt":"REVIEW this one"}
		]}`))
	if err == nil || !strings.Contains(err.Error(), "not enabled in this build") {
		t.Errorf("err = %v, want a refusal saying it is not enabled", err)
	}
	if got := prov.subjectsSeen(); len(got) != 0 {
		t.Errorf("a refused call started work: %v", got)
	}
}

func newFanoutFleet(t *testing.T, prov provider.Provider) *FleetTool {
	t.Helper()
	reg := tool.NewRegistry()
	reg.Add(fakeReadFileTool{})
	return NewFleetTool(NewTaskTool(prov, nil, reg, 20, 0, 0, 0, 0.0, "", "sys", nil, 0, "", "", nil).
		WithTranscripts(mustSubagentStore(t), t.TempDir(), "base", "high").
		WithScheduler(NewSubagentScheduler(6, 6))).WithFanout(true)
}

// fanoutScriptProvider answers by what it is asked rather than by call order,
// because the mapped children run concurrently and a sequential script cannot
// say which of them it is answering.
type fanoutScriptProvider struct {
	subjects   []string
	skipSubmit bool

	mu    sync.Mutex
	seen  []string
	turns map[string]string
}

func (p *fanoutScriptProvider) Name() string { return "fanout-script" }

func (p *fanoutScriptProvider) subjectsSeen() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := append([]string(nil), p.seen...)
	slices.Sort(out)
	return out
}

func (p *fanoutScriptProvider) turnFor(token string) string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.turns[token]
}

func (p *fanoutScriptProvider) record(token, content, subject string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.turns == nil {
		p.turns = map[string]string{}
	}
	if token != "" {
		if _, seen := p.turns[token]; !seen {
			p.turns[token] = content
		}
	}
	if subject != "" {
		p.seen = append(p.seen, subject)
	}
}

func (p *fanoutScriptProvider) Stream(_ context.Context, req provider.Request) (<-chan provider.Chunk, error) {
	last := lastUser(req)
	submitted := false
	for _, m := range req.Messages {
		if m.Role == provider.RoleTool && m.Name == "submit_items" {
			submitted = true
		}
	}

	ch := make(chan provider.Chunk, 3)
	const marker = "The subject for this run: "
	switch {
	case strings.Contains(last, "<fanout-contract>") && !submitted && !p.skipSubmit:
		args, _ := json.Marshal(map[string]any{"items": p.subjects})
		call := provider.ToolCall{ID: "call-submit", Name: "submit_items", Arguments: string(args)}
		ch <- provider.Chunk{Type: provider.ChunkToolCall, ToolCall: &call}
	case strings.Contains(last, marker):
		subject, _, _ := strings.Cut(last[strings.LastIndex(last, marker)+len(marker):], "\n")
		subject = strings.TrimSpace(subject)
		p.record("", "", subject)
		ch <- provider.Chunk{Type: provider.ChunkText, Text: "reviewed " + subject}
	default:
		token := ""
		for _, candidate := range []string{"REPORT", "SCAN"} {
			if strings.Contains(last, candidate) {
				token = candidate
				break
			}
		}
		p.record(token, last, "")
		ch <- provider.Chunk{Type: provider.ChunkText, Text: "done"}
	}
	ch <- provider.Chunk{Type: provider.ChunkDone}
	close(ch)
	return ch, nil
}
