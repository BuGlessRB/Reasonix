package tui

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"reasonix/internal/base/i18n"
	"reasonix/internal/contract/pricing"
	"reasonix/internal/frontend/termrender"
)

const footerIndent = "  "

func footerLabel(s string) string { return termrender.ThemeFg(termrender.ActiveTheme().Subtle, s) }

func footerValue(s string) string { return termrender.ThemeFg(termrender.ActiveTheme().Muted, s) }

func footerMetric(label, value string) string {
	if value == "" {
		return ""
	}
	return footerLabel(label) + " " + value
}

// statusBlock is the footer under the composer: interaction state with the
// model on the right, a quiet rule, then the session's telemetry.
func (m *model) statusBlock() []string {
	width := max(m.width-1, 1)
	first := layoutSides(footerIndent+m.modeTag()+" · "+m.stateText(), m.modelGroup(), width)
	rows := strings.Split(first, "\n")
	if tel := packGroups(m.telemetry(), width); len(tel) > 0 {
		rows = append(rows, footerIndent+termrender.ThemeFg(termrender.ActiveTheme().Border, strings.Repeat("─", max(width-len(footerIndent), 1))))
		rows = append(rows, tel...)
	}
	return rows
}

func (m *model) modeTag() string {
	s := m.status
	switch {
	case m.shell:
		return termrender.Badge(tagShellColor, tagLight, "Shell")
	case s.ToolApprovalMode == "yolo":
		return termrender.Badge(tagYoloColor, tagLight, "YOLO")
	case s.Plan:
		return termrender.Badge(tagPlanColor, tagLight, "Plan")
	case s.ToolApprovalMode == "auto":
		return termrender.Badge(tagAskColor, tagDark, "Auto")
	case s.ToolApprovalMode == "dontAsk":
		return termrender.Badge(tagAskColor, tagDark, "Read only")
	}
	return termrender.Badge(tagAskColor, tagDark, "Ask")
}

func (m *model) stateText() string {
	open := m.tr.OpenPrompt()
	switch {
	case open != nil && open.Kind == ItemAsk:
		return footerLabel(i18n.M.ChatStatusQuestion)
	case open != nil && open.Approval.Kind == "plan":
		return footerLabel(i18n.M.ChatStatusPlanApproval)
	case open != nil:
		return footerLabel(i18n.M.ChatStatusToolApproval)
	case !m.quitArmedAt.IsZero() && time.Since(m.quitArmedAt) < quitArmWindow:
		return i18n.M.CtrlCQuitHint
	case m.shell:
		return i18n.M.ShellModeHint
	case m.tr.Running:
		return footerLabel(i18n.M.ChatStatusCycleHintCompact)
	}
	return footerValue(i18n.M.ChatStatusIdle) + " · " + footerLabel(i18n.M.ChatStatusCycleHintCompact)
}

func (m *model) modelGroup() string {
	s := m.status
	label := s.Label
	if label == "" {
		label = modelName(s.ModelRef)
	}
	var fields []string
	if label != "" {
		fields = append(fields, footerMetric(i18n.M.ChatStatusModelLabel, termrender.ThemeFg(termrender.ActiveTheme().Info, label)))
	}
	switch s.Effort {
	case "", "auto":
		fields = append(fields, footerMetric(i18n.M.ChatStatusEffortLabel, footerValue("auto")))
	default:
		fields = append(fields, footerMetric(i18n.M.ChatStatusEffortLabel, termrender.ThemeFg(termrender.ActiveTheme().Info, termrender.Bold(s.Effort))))
	}
	return strings.Join(fields, "   ")
}

func (m *model) telemetry() []string {
	s := m.status
	var out []string
	if body, rate, ok := cacheStatus(s); ok {
		out = append(out, footerMetric(i18n.M.ChatStatusCacheLabel, termrender.ThemeFg(cacheColor(rate), body)))
	}
	out = append(out, contextGroups(s.Used, s.Window, m.compaction)...)
	if m.balance != "" {
		out = append(out, footerMetric(i18n.M.ChatStatusBalanceLabel, footerValue(m.balance)))
	}
	if cost := quoteText(s.SessionCostQuote); cost != "" {
		out = append(out, footerMetric(i18n.M.ChatStatusCostLabel, footerValue(cost)))
	}
	return out
}

func cacheStatus(s Status) (string, float64, bool) {
	var parts []string
	rate := 0.0
	if u := s.LastUsage; u != nil && u.CacheHitTokens+u.CacheMissTokens > 0 {
		rate = pct(u.CacheHitTokens, u.CacheHitTokens+u.CacheMissTokens)
		parts = append(parts, fmt.Sprintf(i18n.M.ChatStatusCacheNowFmt, fmt.Sprintf("%.2f%%", rate)))
	}
	if total := s.CacheHit + s.CacheMiss; total > 0 {
		rate = pct(s.CacheHit, total)
		parts = append(parts, fmt.Sprintf(i18n.M.ChatStatusCacheAvgFmt, fmt.Sprintf("%.2f%%", rate)))
	}
	return strings.Join(parts, " · "), rate, len(parts) > 0
}

func pct(n, of int) float64 { return float64(n) * 100 / float64(of) }

func cacheColor(rate float64) termrender.Color {
	t := termrender.ActiveTheme()
	switch {
	case rate >= 80:
		return t.Success
	case rate >= 50:
		return t.Info
	}
	return t.Warn
}

// contextGroups shows how full the window is and, where the session folds
// before the window ends, how much room is left before it does.
func contextGroups(used, window int, c Compaction) []string {
	if used == 0 || window == 0 {
		return nil
	}
	t := termrender.ActiveTheme()
	p := used * 100 / window
	threshold := 0
	switch {
	case c.Trigger > 0:
		threshold = c.Trigger * 100 / window
	case c.Ratio > 0 && c.Ratio < 1:
		threshold = int(c.Ratio * 100)
	}
	if threshold <= 0 || threshold >= 100 {
		color := t.Muted
		switch {
		case p >= 85:
			color = t.Danger
		case p >= 60:
			color = t.Warn
		}
		return []string{footerMetric(i18n.M.ChatStatusContextLabel, termrender.ThemeFg(color, fmt.Sprintf("%s / %s (%d%%)", shortTokens(used), shortTokens(window), p)))}
	}
	left := max(threshold-p, 0)
	ctxColor, foldColor := t.Muted, t.Muted
	switch {
	case p >= threshold:
		ctxColor, foldColor = t.Warn, t.Danger
	case left <= 10:
		ctxColor, foldColor = t.Warn, t.Warn
	}
	return []string{
		footerMetric(i18n.M.ChatStatusContextLabel, termrender.ThemeFg(ctxColor, fmt.Sprintf("%s (%d%%)", shortTokens(used), p))),
		footerMetric(i18n.M.ChatStatusCompactLabel, termrender.ThemeFg(foldColor, fmt.Sprintf("%d%%", left))),
	}
}

// quoteText is the session's spend, or "" where the kernel could not price
// it: an estimate never shows as a bare zero.
func quoteText(q *CostQuote) string {
	if q == nil || !q.CostComplete {
		return ""
	}
	money := q.Original
	if q.Selected != nil {
		money = *q.Selected
	}
	amount, err := strconv.ParseFloat(money.Amount, 64)
	if err != nil || amount <= 0 {
		return ""
	}
	return fmt.Sprintf("≈%s%.4f", pricing.CurrencySymbol(money.Currency), amount)
}

// layoutSides puts right against the right edge of left's row, or on a row of
// its own when the two do not fit side by side.
func layoutSides(left, right string, width int) string {
	if right == "" {
		return left
	}
	lw, rw := termrender.VisibleWidth(left), termrender.VisibleWidth(right)
	if lw+2+rw <= width {
		return left + strings.Repeat(" ", width-lw-rw) + right
	}
	return left + "\n" + footerIndent + right
}

// packGroups lays groups left to right, starting a new row only between them.
func packGroups(groups []string, width int) []string {
	var rows []string
	cur := ""
	for _, g := range groups {
		if g == "" {
			continue
		}
		next := footerIndent + g
		if cur != "" {
			next = cur + "  " + g
		}
		if cur != "" && termrender.VisibleWidth(next) > width {
			rows = append(rows, cur)
			next = footerIndent + g
		}
		cur = next
	}
	if cur != "" {
		rows = append(rows, cur)
	}
	return rows
}
