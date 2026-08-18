package agent

import (
	"fmt"
	"strings"

	"reasonix/internal/ablation"
	"reasonix/internal/event"
	"reasonix/internal/evidence"
	"reasonix/internal/instruction"
	"reasonix/internal/taskpolicy"
)

// Final readiness: whether a turn has earned the right to stop. It reads the
// evidence ledger, the delivery profile, and the approved plan's contract, and
// says what is missing rather than merely that something is.

type finalReadinessCheck struct {
	applies                   bool
	reason                    string
	missingProjectChecks      int
	incompleteTodos           int
	missingAcceptanceCriteria int
	missingVerification       int
	missingReview             int
	missingSignoff            int
	missingActionEvidence     int
	missingMutation           int
	missingCapabilities       int
}

func (c finalReadinessCheck) progressSignature() string {
	return fmt.Sprintf("%d/%d/%d/%d/%d/%d/%d/%d/%d/%d\x00%s",
		c.missingProjectChecks,
		c.incompleteTodos,
		c.missingAcceptanceCriteria,
		c.missingVerification,
		c.missingReview,
		c.missingSignoff,
		c.missingActionEvidence,
		c.missingMutation,
		c.missingCapabilities,
		boolInt(c.applies),
		c.reason,
	)
}

func (c finalReadinessCheck) missingIDs() []string {
	missing := make([]string, 0, 9)
	add := func(id string, count int) {
		if count > 0 {
			missing = append(missing, id)
		}
	}
	add("project_check", c.missingProjectChecks)
	add("todo", c.incompleteTodos)
	add("criteria", c.missingAcceptanceCriteria)
	add("verification", c.missingVerification)
	add("review", c.missingReview)
	add("signoff", c.missingSignoff)
	add("action", c.missingActionEvidence)
	add("mutation", c.missingMutation)
	add("capability", c.missingCapabilities)
	return missing
}

func (c finalReadinessCheck) audit(result evidence.ReadinessAuditResult, recovered bool) evidence.ReadinessAudit {
	return evidence.ReadinessAudit{
		Result:                    result,
		Recovered:                 recovered,
		MissingProjectChecks:      c.missingProjectChecks,
		IncompleteTodos:           c.incompleteTodos,
		CommandMismatchMissing:    c.missingProjectChecks,
		MissingAcceptanceCriteria: c.missingAcceptanceCriteria,
		MissingVerification:       c.missingVerification,
		MissingReview:             c.missingReview,
		MissingSignoff:            c.missingSignoff,
		MissingActionEvidence:     c.missingActionEvidence,
		MissingMutation:           c.missingMutation,
		MissingCapabilities:       c.missingCapabilities,
	}
}

// declaredChecksRanAfter reports whether the project's own declared checks all
// ran after the latest write. A project that names its checks has defined what
// verification means there, so the host's command classifier — which cannot
// tell `python3 test_x.py` from `python3 deploy.py` — does not get a second
// say. Projects that declare nothing keep the classifier as the only floor.
func (a *Agent) declaredChecksRanAfter(writer int) bool {
	if len(a.projectChecks) == 0 {
		return false
	}
	for _, check := range a.projectChecks {
		command := strings.TrimSpace(check.Command)
		if command == "" {
			continue
		}
		if !a.task.ledger.HasSuccessfulCommandAfter(command, writer) {
			return false
		}
	}
	return true
}

// unmetProjectChecks lists the declared checks that have not run since the
// latest write, phrased as the instruction that would settle each one.
func (a *Agent) unmetProjectChecks(writer int) []string {
	var gaps []string
	for _, check := range a.projectChecks {
		command := strings.TrimSpace(check.Command)
		if command == "" {
			continue
		}
		if !a.task.ledger.HasSuccessfulCommandAfter(command, writer) {
			gaps = append(gaps, fmt.Sprintf("run %q from %s after the latest write", command, finalReadinessCheckSource(check)))
		}
	}
	return gaps
}

func (a *Agent) finalReadinessCheckFor() finalReadinessCheck {
	if a.task.ledger == nil || a.ablation.Off(ablation.Evidence) {
		return finalReadinessCheck{}
	}
	var missing []string
	out := finalReadinessCheck{}
	// Planning returns a proposal; the controller owns approval and starts a
	// fresh execution turn, which is where delivery requirements belong. A
	// workflow boundary only — tool calls still take the usual permission path.
	if a.planMode.Load() {
		return out
	}
	{
		incomplete, hasTodos := a.task.ledger.IncompleteLatestTodos()
		if !hasTodos && a.task.ledger.HasAnySuccessfulReceipt() {
			incomplete, hasTodos = a.incompleteCanonicalTodos()
		}
		if hasTodos && len(incomplete) > 0 && a.task.ledger.HasSuccessfulTodoProgressReceipt() {
			out.applies = true
			out.incompleteTodos = len(incomplete)
			missing = append(missing, finalReadinessIncompleteTodos(incomplete))
		}
	}
	writer, hasWriter := a.mutationBaseline(false)
	deliveryMutation := false
	deliveryVerificationOnly := false
	checkpoint := a.task.checkpoint
	checkpointApplies := a.turn.deliveryScopeActive && checkpoint.ScopeID == a.task.scopeID
	if a.deliveryProfile {
		if mutation, ok := a.mutationBaseline(true); ok {
			writer, hasWriter = mutation, true
			deliveryMutation = true
		} else if checkpointApplies && checkpoint.PendingMutation {
			// The mutation happened before a controller rebuild/restart. Treat it as
			// the baseline so this run can satisfy verification/review/sign-off
			// without manufacturing another write.
			writer, hasWriter = -1, true
			deliveryMutation = true
		} else if checkpointApplies && checkpoint.MutationObserved {
			deliveryMutation = true
		}
		// What a turn owes is read off the ledger, never off the task text: one
		// that changed nothing owes nothing, one that did owes the verification,
		// review, and sign-off below.
		if !hasWriter && a.task.ledger.HasSuccessfulVerificationCommand() {
			writer, hasWriter = -1, true
			deliveryVerificationOnly = true
		}
		// Required/preferred capability gates apply before the no-writer fast
		// path below: a user-required Skill/MCP must not be skippable by
		// answering from ordinary reads alone.
		if msg := a.capabilityGateFailure(); msg != "" {
			out.applies = true
			out.missingCapabilities++
			missing = append(missing, msg)
		}
		if a.task.ledger.HasSuccessfulToolReceipt("remember") && !a.task.ledger.HasSuccessfulMutationOtherThan("remember") {
			// A turn whose only mutation was a memory write has nothing a test or
			// a diff could add. Any unrelated mutation falls through to the full
			// contract below.
			out.applies = true
			if len(missing) > 0 {
				out.reason = strings.Join(missing, "; ")
			}
			return out
		}
	}
	if !hasWriter {
		if len(missing) > 0 {
			out.reason = strings.Join(missing, "; ")
		}
		return out
	}
	// A turn declared blocked still owes the check that establishes the blocker;
	// what it cannot owe is a passing one, since the task being impossible is
	// exactly why the check does not pass. Nothing else about the turn is waived.
	verified, blockedWithCheck := a.postWriteVerification(writer)
	if !a.deliveryProfile && a.turn.policySet && a.turn.policy.Verification >= taskpolicy.VerifyTargeted &&
		toolPresent(a.svc.tools, "bash") && !blockedWithCheck && !a.checkEstablished(writer, verified) {
		out.applies = true
		out.missingVerification++
		missing = append(missing, a.verificationGap(writer))
	}
	hasProjectChecks := len(a.projectChecks) > 0
	hasTodoReceipt := a.task.ledger.HasSuccessfulTodoWrite()
	if !a.deliveryProfile && !hasProjectChecks && !hasTodoReceipt && len(missing) == 0 {
		return finalReadinessCheck{}
	}
	out.applies = true
	if a.deliveryProfile {
		a.emitTurnPhase(event.TurnPhaseVerifying)
		criteriaEstablished := a.turn.deliveryCriteriaEstablished || (checkpointApplies && checkpoint.CriteriaEstablished)
		if !criteriaEstablished {
			out.missingAcceptanceCriteria++
			missing = append(missing, "establish concrete acceptance criteria with todo_write before changing state")
		}
		hasCompleteStep := a.task.ledger.HasSuccessfulCompleteStepAfter(writer)
		if !hasCompleteStep {
			out.missingSignoff++
			missing = append(missing, "call complete_step after the latest mutation")
		}
		// A sign-off that cited a passing check but lacks the review is missing
		// the review, not the verification. Reporting both sends the model to
		// re-run and re-cite a check it already ran, cited, and watched pass.
		if !a.task.ledger.HasSuccessfulDeliverySignoffAfter(writer) && !blockedWithCheck &&
			!a.task.ledger.HasCitedVerificationAfter(writer) {
			out.missingVerification++
			missing = append(missing, "run relevant verification after the latest mutation and cite that successful command in complete_step")
		}
		if deliveryMutation && !a.task.ledger.HasSuccessfulReviewAfter(writer) {
			out.missingReview++
			missing = append(missing, "inspect the changed result after the latest mutation (read the touched file or run git diff/status)")
		}
		if msg := a.deliveryReviewGateFailure(); msg != "" {
			out.missingReview++
			missing = append(missing, msg)
		}
		// The capability gate already ran before the no-writer fast path above.
	}
	if !deliveryVerificationOnly {
		gaps := a.unmetProjectChecks(writer)
		out.missingProjectChecks += len(gaps)
		missing = append(missing, gaps...)
	}

	outstanding := a.outstandingPlanCriteria()
	out.missingAcceptanceCriteria += len(outstanding)
	missing = append(missing, outstanding...)
	if len(missing) == 0 {
		return out
	}
	out.reason = strings.Join(missing, "; ")
	return out
}

// checkEstablished reports that something after the latest write stands as its
// check: one the table recognised, a project's declared one, or one a
// completion named and the ledger corroborated.
func (a *Agent) checkEstablished(writer int, verified bool) bool {
	return verified ||
		a.declaredChecksRanAfter(writer) ||
		a.task.ledger.HasCorroboratedCitedCheckAfter(writer)
}

// postWriteVerification reads what the checks after the latest write establish.
// verified needs a passing check with none of them still standing failed — one
// check passing cannot answer for another that did not. blockedWithCheck is the
// declared-impossible case, which still owes the run that establishes it.
func (a *Agent) postWriteVerification(writer int) (verified, blockedWithCheck bool) {
	ledger := a.task.ledger
	verified = ledger.HasSuccessfulVerificationCommandAfter(writer) &&
		!ledger.HasFailedVerificationAfter(writer)
	blockedWithCheck = ledger.HasBlockedConclusionAfter(writer) &&
		ledger.HasVerificationCommandAfter(writer)
	return verified, blockedWithCheck
}

// verificationGap says what is actually missing. A check whose exit status
// belonged to a later stage of the same command did run, so asking for "a
// verification command" reads as false to the model that just ran one; what it
// needs is the command named and a shape whose status answers for the check.
func (a *Agent) verificationGap(writer int) string {
	const ask = "run a relevant verification command after the latest write for the current role setting"
	if unreadable, ok := a.task.ledger.LatestUnreadableVerificationAfter(writer); ok && strings.TrimSpace(unreadable.Command) != "" {
		return fmt.Sprintf("%s — %q ran, but its exit status is the last stage's, not the check's, "+
			"so it proves nothing either way; re-run the check on its own", ask, strings.TrimSpace(unreadable.Command))
	}
	// A check that ran and failed leaves two honest ways out, and a model told
	// only to "run a verification command" can see neither: it already ran one.
	if a.task.ledger.HasVerificationCommandAfter(writer) && toolPresent(a.svc.tools, "conclude_blocked") {
		return "the check you ran after the latest write did not pass: either make it pass, or — if it cannot pass as specified — call conclude_blocked with the evidence for why"
	}
	// Commands ran and the table read none as a check. Widening it would read a
	// deploy as one; what it lacks is in the turn, not the command, and a
	// citation carries it — so point there, not at what the model just did.
	if len(a.projectChecks) == 0 && toolPresent(a.svc.tools, "complete_step") {
		if ran, ok := a.task.ledger.LatestUnrecognizedCommandAfter(writer); ok {
			return fmt.Sprintf("%s — or, if %q was that check, sign the step off with complete_step citing it "+
				"(evidence kind=verification, command exactly as run): the host cannot tell a project's own "+
				"runner from any other command, but it can prove the one you name ran after the write and passed",
				ask, strings.TrimSpace(ran.Command))
		}
	}
	return ask
}

func finalReadinessCheckSource(check instruction.VerifyCheck) string {
	source := strings.TrimSpace(check.SourcePath)
	if source == "" {
		source = "project memory"
	}
	if check.Line > 0 {
		return fmt.Sprintf("%s:%d", source, check.Line)
	}
	return source
}

func finalReadinessIncompleteTodos(items []evidence.TodoStepMatch) string {
	parts := make([]string, 0, len(items))
	for _, item := range items {
		label := strings.TrimSpace(item.Content)
		if label == "" {
			label = fmt.Sprintf("todo %d", item.Index)
		}
		parts = append(parts, fmt.Sprintf("%s: %s", label, item.Status))
	}
	return "latest successful todo_write still has incomplete items: " + strings.Join(parts, ", ")
}

// PrepareReadinessContinuation is the same authorization for a continuation the
// host runs itself rather than one the user asked for. Without it the next run
// starts from an empty ledger, where a turn that owed verification owes
// nothing: the gap would read as closed because the record of it was dropped.
func (a *Agent) PrepareReadinessContinuation() bool {
	return a.prepareEvidenceContinuation()
}

func (a *Agent) prepareEvidenceContinuation() bool {
	if !a.pending.deliveryRecovery {
		return false
	}
	a.pending.preserveEvidence = true
	a.pending.deliveryRecovery = false
	return true
}
