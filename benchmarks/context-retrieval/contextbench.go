// contextbench.go — what the model did with the affordance, read off structure.
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"reasonix/internal/provider"
	"reasonix/internal/tokencount"
)

// Every judgement here reads a tool call's name and its JSON arguments, or a
// canonical position, and never the model's prose. "I will search the history"
// is not a search; a recall call with a query is.

// failure stages, in funnel order. Exactly one is reported per run.
const (
	stageReadMissed  = "NoSearch"           // never asked, and never read the target either
	stageSearchMiss  = "SearchMiss"         // searched, target never came back
	stageHitNotRead  = "HitNotRead"         // target was returned and never opened
	stageAnswerWrong = "ReadButAnswerWrong" // opened it and still answered wrong
	stageRecovered   = "Recovered"
	stageDirectRead  = "DirectRead"  // read the target from an index address, no search needed
	stageNoRetrieval = "NoRetrieval" // answered without touching recall at all
	toolRecall       = "recall"
)

// contextMetrics is one run's behaviour.
type contextMetrics struct {
	Task string `json:"task"`
	Arm  string `json:"arm"`

	SearchCalls int `json:"search_calls"`
	// SearchHits counts searches that returned anything; TargetSearchHits
	// counts searches that returned the target. The gap is query quality.
	SearchHits       int `json:"search_hits"`
	TargetSearchHits int `json:"target_search_hits"`
	FirstTargetRank  int `json:"first_target_rank"` // 0 = never returned

	ReadCalls    int  `json:"read_calls"`
	TargetRead   bool `json:"target_read"`
	ReadAfterHit bool `json:"read_after_hit"`
	// DirectRead is a read of the target with no search before it: the index
	// addressed it, which is the whole claim an index arm is testing.
	DirectRead bool `json:"direct_read"`

	FirstSearchRound int `json:"first_search_round"`
	FirstReadRound   int `json:"first_read_round"`
	RetrievalRounds  int `json:"retrieval_rounds"`

	// RecallReturnedTokens is what recall paged back in. The index saves
	// resident prefix; this is what is paid dynamically instead.
	RecallReturnedTokens int `json:"recall_returned_tokens"`

	AnswerRecovered bool     `json:"answer_recovered"`
	MissingMarkers  []string `json:"missing_markers,omitempty"`
	// FinalAnswer is kept bounded so a surprising run can be read back. A
	// metric nobody can audit is a claim, not a measurement.
	FinalAnswer  string `json:"final_answer,omitempty"`
	FailureStage string `json:"failure_stage"`

	// UnexpectedWorkTools are investigation tools used instead of recall. The
	// answer exists only in folded history, so reaching for the workspace is a
	// strategy change worth seeing even when it fails.
	UnexpectedWorkTools []string `json:"unexpected_work_tools,omitempty"`

	CueVisible bool `json:"cue_visible"`
}

// hitPosition matches the address recall prints for a search hit. Reading our
// own rendered format, not guessing at the model's words.
var hitPosition = regexp.MustCompile(`(?m)^#(\d+) `)

// scoreRun reads the messages one turn appended and reports what happened.
func scoreRun(msgs []provider.Message, t contextTask, target int, arm string, cueVisible bool) contextMetrics {
	m := contextMetrics{Task: t.ID, Arm: arm, CueVisible: cueVisible}
	calls := map[string]provider.ToolCall{}
	round := 0
	sawTargetHit := false

	for _, msg := range msgs {
		if len(msg.ToolCalls) > 0 {
			round++
		}
		for _, tc := range msg.ToolCalls {
			calls[tc.ID] = tc
			if tc.Name != toolRecall {
				if !containsString(m.UnexpectedWorkTools, tc.Name) {
					m.UnexpectedWorkTools = append(m.UnexpectedWorkTools, tc.Name)
				}
				continue
			}
			query, positions := recallArgs(tc.Arguments)
			switch {
			case query != "":
				m.SearchCalls++
				if m.FirstSearchRound == 0 {
					m.FirstSearchRound = round
				}
			case len(positions) > 0:
				m.ReadCalls++
				if m.FirstReadRound == 0 {
					m.FirstReadRound = round
				}
				// A read covers the target when it names its position. The
				// span recall returns also carries the tool results below it,
				// which is why the caller's own address is the one to match.
				if containsInt(positions, target) {
					m.TargetRead = true
					if sawTargetHit {
						m.ReadAfterHit = true
					} else if m.SearchCalls == 0 {
						m.DirectRead = true
					}
				}
			}
		}
		if msg.Role != provider.RoleTool {
			continue
		}
		call, ok := calls[msg.ToolCallID]
		if !ok || call.Name != toolRecall {
			continue
		}
		m.RecallReturnedTokens += tokencount.Text(msg.Content)
		query, _ := recallArgs(call.Arguments)
		if query == "" {
			continue
		}
		positions := searchHitPositions(msg.Content)
		if len(positions) > 0 {
			m.SearchHits++
		}
		if rank := indexOfInt(positions, target); rank > 0 {
			m.TargetSearchHits++
			sawTargetHit = true
			if m.FirstTargetRank == 0 {
				m.FirstTargetRank = rank
			}
		}
	}
	m.RetrievalRounds = m.SearchCalls + m.ReadCalls
	return m
}

// scoreAnswer grades the final text on the task's markers. Nonces, matched
// exactly: no judge model, and nothing a general answer could satisfy.
func (m *contextMetrics) scoreAnswer(final string, t contextTask) {
	lower := strings.ToLower(final)
	for _, marker := range t.AnswerMarkers {
		if !strings.Contains(lower, strings.ToLower(marker)) {
			m.MissingMarkers = append(m.MissingMarkers, marker)
		}
	}
	m.AnswerRecovered = len(m.MissingMarkers) == 0
	m.FailureStage = m.stage()
	if len(final) > 1200 {
		final = final[:1200] + "…"
	}
	m.FinalAnswer = final
}

// stage places the run in the funnel. One label, chosen at the first step that
// did not happen, so a table of stages reads as where the retrieval broke.
func (m contextMetrics) stage() string {
	switch {
	case m.AnswerRecovered && m.DirectRead:
		return stageDirectRead
	case m.AnswerRecovered:
		return stageRecovered
	case m.TargetRead:
		return stageAnswerWrong
	case m.TargetSearchHits > 0:
		return stageHitNotRead
	case m.SearchCalls > 0:
		return stageSearchMiss
	case m.ReadCalls > 0:
		return stageReadMissed
	default:
		return stageNoRetrieval
	}
}

// recallArgs pulls the two fields that say which operation a recall call is.
func recallArgs(args string) (query string, positions []int) {
	var parsed struct {
		Query     string `json:"query"`
		Positions []int  `json:"positions"`
	}
	if err := json.Unmarshal([]byte(args), &parsed); err != nil {
		return "", nil
	}
	return strings.TrimSpace(parsed.Query), parsed.Positions
}

// searchHitPositions reads the addresses out of a search result, in rank order.
func searchHitPositions(text string) []int {
	var out []int
	for _, match := range hitPosition.FindAllStringSubmatch(text, -1) {
		if n, err := strconv.Atoi(match[1]); err == nil {
			out = append(out, n)
		}
	}
	return out
}

func containsInt(list []int, want int) bool { return slices.Contains(list, want) }

// indexOfInt returns a 1-based rank, or 0 when absent.
func indexOfInt(list []int, want int) int {
	for i, v := range list {
		if v == want {
			return i + 1
		}
	}
	return 0
}

func containsString(list []string, want string) bool { return slices.Contains(list, want) }

// funnel summarizes one arm's runs the way the experiment is read.
type funnel struct {
	Arm                                       string
	Runs                                      int
	Searched, TargetHit, TargetRead, Answered int
	DirectReads                               int
	SearchCalls, ReadCalls, RecallTokens      int
	Escapes                                   int
	Stages                                    map[string]int
}

func summarize(arm string, runs []contextMetrics) funnel {
	f := funnel{Arm: arm, Runs: len(runs), Stages: map[string]int{}}
	for _, r := range runs {
		if r.SearchCalls > 0 {
			f.Searched++
		}
		if r.TargetSearchHits > 0 {
			f.TargetHit++
		}
		if r.TargetRead {
			f.TargetRead++
		}
		if r.AnswerRecovered {
			f.Answered++
		}
		if r.DirectRead {
			f.DirectReads++
		}
		if len(r.UnexpectedWorkTools) > 0 {
			f.Escapes++
		}
		f.SearchCalls += r.SearchCalls
		f.ReadCalls += r.ReadCalls
		f.RecallTokens += r.RecallReturnedTokens
		f.Stages[r.FailureStage]++
	}
	return f
}

func (f funnel) line() string {
	per := func(n int) float64 {
		if f.Runs == 0 {
			return 0
		}
		return float64(n) / float64(f.Runs)
	}
	return fmt.Sprintf("%-12s runs=%-3d answered=%d/%d  searched=%d  target-hit=%d  read=%d  direct=%d  search/task=%.2f  recall-tok/task=%.0f  escapes=%d",
		f.Arm, f.Runs, f.Answered, f.Runs, f.Searched, f.TargetHit, f.TargetRead, f.DirectReads,
		per(f.SearchCalls), per(f.RecallTokens), f.Escapes)
}

func writeJSONLine(w io.Writer, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(w, string(b))
	return err
}
