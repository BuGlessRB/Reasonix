package evidence

import (
	"path"
	"strings"

	"reasonix/internal/shellsafe"
)

// OutcomeSample decomposes one tool round's receipts by outcome: information
// gathered (Exploration), verification attempts run, and verification-command
// state transitions (Objective fail→pass, Regression pass→fail). Counts are
// unit-weighted; policy weighting is an offline concern.
type OutcomeSample struct {
	Round        int
	Exploration  int
	Verification int
	Objective    int
	Regression   int
	Churn        int
	// LegacyGain is the live novelty scorer's verdict on the same receipts, so
	// offline analysis can compare the two policies without replaying.
	LegacyGain int
	// Discriminating counts observations able to falsify the working
	// hypothesis: verification commands, or commands exercising a mutated
	// file — deliberately broader than delivery verification (repro scripts).
	Discriminating int
	// DebtAge counts consecutive rounds carrying an unverified mutation with
	// no discriminating observation; 0 while no verification debt is open.
	DebtAge int
	// BlindMutations counts mutations since the last discriminating observation.
	BlindMutations int
	// Stall counts this round's checks that ran, failed, and had already
	// failed: the third transition beside Objective and Regression, and the one
	// the live scorer prices as motion instead of as standing still.
	Stall int
	// StallAge counts consecutive rounds one check has stayed failed, and
	// StallMutations the change that landed against it without moving it.
	StallAge       int
	StallMutations int
}

// stall is the check a turn is currently stuck on: which one, how long it has
// stayed failed, and how much change landed against it in the meantime. The
// three share one lifetime — a state transition ends all of them at once.
type stall struct {
	identity  string
	age       int
	mutations int
}

// OutcomeTracker is the shadow counterpart of ProgressTracker: same per-round
// receipts, scored by outcome instead of novelty. It never influences guard
// behavior — samples exist only for trajectory recording and offline analysis.
type OutcomeTracker struct {
	legacy       *ProgressTracker
	round        int
	readPaths    map[string]bool
	commands     map[string]bool
	failures     map[string]bool
	actions      map[string]bool
	verifySeen   map[string]bool
	verifyPass   map[string]bool
	mutatedBases map[string]bool
	debt         bool
	debtAge      int
	blind        int
	stall        stall
	// failedCheck is the verification identity that failed this round and
	// refailed says it had already failed, handed from scoreCommand to the
	// round tail where the streak is kept.
	failedCheck string
	refailed    bool
}

func NewOutcomeTracker() *OutcomeTracker {
	return &OutcomeTracker{
		legacy:       NewProgressTracker(),
		readPaths:    map[string]bool{},
		commands:     map[string]bool{},
		failures:     map[string]bool{},
		actions:      map[string]bool{},
		verifySeen:   map[string]bool{},
		verifyPass:   map[string]bool{},
		mutatedBases: map[string]bool{},
	}
}

// ScoreRound folds one round's receipts into the tracker and returns the
// round's outcome decomposition.
func (t *OutcomeTracker) ScoreRound(receipts []Receipt) OutcomeSample {
	if t == nil {
		return OutcomeSample{}
	}
	t.round++
	s := OutcomeSample{Round: t.round}
	t.failedCheck, t.refailed = "", false
	for _, r := range receipts {
		t.scoreReceipt(r, &s)
	}
	s.LegacyGain = t.legacy.ScoreRound(receipts)
	// Verification debt: a discriminating observation settles it; otherwise a
	// mutation opens it and every silent round ages it, mutation round included.
	if s.Discriminating > 0 {
		t.debt, t.debtAge, t.blind = false, 0, 0
	} else {
		if s.Churn > 0 {
			t.debt = true
			t.blind += s.Churn
		}
		if t.debt {
			t.debtAge++
		}
	}
	s.DebtAge = t.debtAge
	s.BlindMutations = t.blind
	t.trackStall(&s)
	return s
}

// trackStall follows the one check a turn is stuck on. A transition in either
// direction ends the streak because the check moved; a different check failing
// again starts a new one; everything in between is change that landed without
// moving anything.
func (t *OutcomeTracker) trackStall(s *OutcomeSample) {
	switch {
	case s.Objective > 0 || s.Regression > 0:
		t.stall = stall{}
	case t.failedCheck != "" && t.failedCheck != t.stall.identity:
		// Opened by the first failure, not the repeat: the change made in
		// between is the change that failed to move it, and it is already made
		// by the time the repeat proves so.
		t.stall = stall{identity: t.failedCheck}
	}
	if t.stall.identity == "" {
		return
	}
	if t.refailed {
		t.stall.age++
	}
	t.stall.mutations += s.Churn
	s.StallAge, s.StallMutations = t.stall.age, t.stall.mutations
}

// noteMutatedPaths remembers mutated file basenames so a later command that
// mentions one (running a repro script, a targeted test file) reads as a
// discriminating observation even when it is not delivery verification.
func (t *OutcomeTracker) noteMutatedPaths(paths []string) {
	for _, p := range paths {
		if base := path.Base(strings.ReplaceAll(p, "\\", "/")); len(base) >= 3 {
			t.mutatedBases[base] = true
		}
	}
}

func (t *OutcomeTracker) commandExercisesMutation(command string) bool {
	// Inspecting a mutated file (cat/grep/head) cannot falsify anything; only
	// a command that can execute it discriminates.
	if _, _, readOnly := shellsafe.CommandIsReadOnly(command); readOnly {
		return false
	}
	for base := range t.mutatedBases {
		if strings.Contains(command, base) {
			return true
		}
	}
	return false
}

func (t *OutcomeTracker) scoreReceipt(r Receipt, s *OutcomeSample) {
	if command := strings.TrimSpace(r.Command); command != "" {
		t.scoreCommand(command, r, s)
		return
	}
	switch {
	case r.Success && (r.Mutation || r.Write):
		// A mutation is a state transition, not proof of progress: it counts
		// as churn until a verification transition vouches for it.
		s.Churn++
		t.noteMutatedPaths(r.Paths)
	case r.Success && (r.ToolName == "task" || r.ToolName == "parallel_tasks" || r.ToolName == "fleet"):
		// A delegation return is new information at best — never objective
		// progress on its own.
		s.Exploration++
	case r.Success && (r.StepProof || r.TodoStep != nil || len(r.Todos) > 0):
		// Bookkeeping moves no outcome dimension.
	case r.Success && r.Read && r.OutputBytes > 0 && len(r.Paths) > 0:
		for _, path := range r.Paths {
			if path == "" || t.readPaths[path] {
				continue
			}
			t.readPaths[path] = true
			s.Exploration++
		}
	case r.Success:
		sig := r.ToolName + "\x00" + string(r.Args)
		if !t.actions[sig] {
			t.actions[sig] = true
			s.Exploration++
		}
	}
}

func (t *OutcomeTracker) scoreCommand(command string, r Receipt, s *OutcomeSample) {
	if r.Success && (r.Mutation || r.Write) {
		s.Churn++
		t.noteMutatedPaths(r.Paths)
	}
	verify := IsDeliveryVerificationCommand(command)
	if verify || t.commandExercisesMutation(command) {
		s.Discriminating++
	}
	if verify {
		s.Verification++
		// The receipt's own exit status is the pipeline's, so `go test … | head`
		// reports success while the suite failed. The host's classification is
		// what read the failing stage.
		key, passed := VerificationIdentity(command), verificationPassed(r)
		seen, wasPass := t.verifySeen[key], t.verifyPass[key]
		t.verifySeen[key] = true
		t.verifyPass[key] = passed
		if seen && passed && !wasPass {
			s.Objective++
		}
		if seen && !passed && wasPass {
			s.Regression++
		}
		if !passed {
			t.failedCheck = key
			if seen && !wasPass {
				s.Stall++
				t.refailed = true
			}
		}
	}
	if r.Success {
		if !verify && !t.commands[command] {
			s.Exploration++
		}
		t.commands[command] = true
		return
	}
	if !t.failures[command] {
		t.failures[command] = true
		s.Exploration++
	}
}
