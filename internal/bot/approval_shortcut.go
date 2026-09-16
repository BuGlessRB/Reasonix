package bot

// Answering a pending approval or ask by replying to its message: which reply
// counts as which answer, and which pending question a reply is answering.

import (
	"fmt"
	"reasonix/internal/event"
	"strconv"
	"strings"
)

func (gw *BotGateway) normalizeApprovalShortcut(key, text string) (string, bool) {
	approvalID := gw.currentPendingApprovalID(key)
	if approvalID == "" {
		return "", false
	}
	if gw.pendingApprovalIsRecovery(key, approvalID) {
		if command, ok := recoveryShortcutCommand(text, gw.pendingRecoveryCanGrantTask(key, approvalID)); ok {
			return command + " " + approvalID, true
		}
		return "", false
	}
	command, ok := approvalShortcutCommand(text)
	if !ok {
		return "", false
	}
	return command + " " + approvalID, true
}

func approvalShortcutCommand(text string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(text)) {
	case "1", "y", "yes", "ok", "同意", "批准", "允许", "允许一次":
		return "/approve", true
	case "2", "0", "n", "no", "deny", "拒绝":
		return "/deny", true
	default:
		return "", false
	}
}

func recoveryShortcutCommand(text string, canGrantTask bool) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(text)) {
	case "1", "y", "yes", "ok", "继续", "继续此变更", "continue":
		return "/recovery-continue", true
	case "2", "a", "同类", "本任务允许", "allow similar":
		if canGrantTask {
			return "/recovery-continue-task", true
		}
		return "/recovery-revise", true
	case "3":
		if canGrantTask {
			return "/recovery-revise", true
		}
		return "", false
	case "修改", "修改方案", "换个办法", "revise":
		return "/recovery-revise", true
	default:
		return "", false
	}
}

func (gw *BotGateway) pendingRecoveryCanGrantTask(key, id string) bool {
	gw.mu.Lock()
	defer gw.mu.Unlock()
	state, ok := gw.controllers[key]
	if !ok || state.pendingApprovals == nil {
		return false
	}
	a, ok := state.pendingApprovals[id]
	return ok && a.Recovery != nil && a.Recovery.CanGrantTask
}

func (gw *BotGateway) pendingApprovalIsRecovery(key, id string) bool {
	gw.mu.Lock()
	defer gw.mu.Unlock()
	state, ok := gw.controllers[key]
	if !ok || state.pendingApprovals == nil {
		return false
	}
	a, ok := state.pendingApprovals[id]
	if !ok {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(a.Kind), "recovery") || a.Recovery != nil
}

func decisionShortcutCommand(text string) (string, bool) {
	if command, ok := approvalShortcutCommand(text); ok {
		return command, true
	}
	if _, ok := askShortcutAnswer(text); ok {
		return "/answer", true
	}
	return "", false
}

func (gw *BotGateway) currentPendingApprovalID(key string) string {
	gw.mu.Lock()
	defer gw.mu.Unlock()
	state, ok := gw.controllers[key]
	if !ok || len(state.pendingApprovals) == 0 {
		return ""
	}
	if state.lastApprovalID != "" {
		if _, ok := state.pendingApprovals[state.lastApprovalID]; ok {
			return state.lastApprovalID
		}
	}
	for id := range state.pendingApprovals {
		return id
	}
	return ""
}

func (gw *BotGateway) forgetPendingApproval(key, id string) {
	gw.mu.Lock()
	defer gw.mu.Unlock()
	state, ok := gw.controllers[key]
	if !ok || state.pendingApprovals == nil {
		return
	}
	delete(state.pendingApprovals, id)
	if state.lastApprovalID == id {
		state.lastApprovalID = ""
		for nextID := range state.pendingApprovals {
			state.lastApprovalID = nextID
			break
		}
	}
}

func (gw *BotGateway) normalizeAskShortcut(key, text string) (string, bool) {
	raw := strings.TrimSpace(text)
	if raw == "" || strings.HasPrefix(raw, "/") {
		return "", false
	}
	askID := gw.currentPendingAskIDForReply(key)
	if askID == "" {
		return "", false
	}
	return "/answer " + askID + " " + raw, true
}

func askShortcutAnswer(text string) (string, bool) {
	raw := strings.TrimSpace(text)
	if raw == "" {
		return "", false
	}
	if strings.ContainsAny(raw, " \t\n;=") {
		return "", false
	}
	if _, err := strconv.Atoi(raw); err == nil {
		return raw, true
	}
	return "", false
}

func (gw *BotGateway) currentPendingAskIDForReply(key string) string {
	gw.mu.Lock()
	defer gw.mu.Unlock()
	state, ok := gw.controllers[key]
	if !ok || len(state.pendingAsks) == 0 {
		return ""
	}
	if state.lastAskID != "" {
		if _, ok := state.pendingAsks[state.lastAskID]; ok {
			return state.lastAskID
		}
	}
	if len(state.pendingAsks) != 1 {
		return ""
	}
	for id := range state.pendingAsks {
		return id
	}
	return ""
}

func parseAskAnswers(questions []event.AskQuestion, raw string) []event.AskAnswer {
	raw = strings.TrimSpace(raw)
	if len(questions) == 0 {
		return []event.AskAnswer{{Selected: []string{raw}}}
	}
	byID := make(map[string]*event.AskQuestion, len(questions))
	for i := range questions {
		q := &questions[i]
		byID[q.ID] = q
		byID[fmt.Sprintf("%d", i+1)] = q
	}
	answerMap := make(map[string][]string, len(questions))
	if strings.Contains(raw, "=") {
		for part := range strings.SplitSeq(raw, ";") {
			k, v, ok := strings.Cut(part, "=")
			if !ok {
				continue
			}
			q := byID[strings.TrimSpace(k)]
			if q == nil {
				continue
			}
			answerMap[q.ID] = normalizeAskSelection(*q, strings.TrimSpace(v))
		}
	} else if len(questions) == 1 {
		answerMap[questions[0].ID] = normalizeAskSelection(questions[0], raw)
	}
	out := make([]event.AskAnswer, 0, len(questions))
	for _, q := range questions {
		out = append(out, event.AskAnswer{QuestionID: q.ID, Selected: answerMap[q.ID]})
	}
	return out
}

func normalizeAskSelection(q event.AskQuestion, raw string) []string {
	parts := []string{raw}
	if q.Multi && strings.Contains(raw, ",") {
		parts = strings.Split(raw, ",")
	}
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if idx, err := strconv.Atoi(part); err == nil && idx >= 1 && idx <= len(q.Options) {
			out = append(out, q.Options[idx-1].Label)
			continue
		}
		out = append(out, part)
	}
	return out
}
