package tui

import (
	"context"
	"time"

	tea "charm.land/bubbletea/v2"

	"reasonix/internal/base/i18n"
)

// yoloConfirmWindow is how long a first Ctrl+Y waits for the second press
// that confirms YOLO the one time it is asked.
const yoloConfirmWindow = 5 * time.Second

// cycleMode steps through 1.x's order — read only, ask, auto, YOLO, plan —
// and back to read only. YOLO joins the cycle once it was confirmed, which
// Ctrl+Y asks for; before that auto steps straight to plan.
func (m *model) cycleMode() tea.Cmd {
	s := &m.status
	switch {
	case s.Plan:
		return m.setPosture(false, "readOnly")
	case s.ToolApprovalMode == "readOnly":
		return m.setPosture(false, "ask")
	case s.ToolApprovalMode == "auto" && m.opts.YoloConfirmed:
		m.yoloRestore = "auto"
		return m.setPosture(false, "yolo")
	case s.ToolApprovalMode == "auto":
		return m.setPosture(true, "auto")
	case s.ToolApprovalMode == "yolo":
		m.yoloRestore = ""
		return m.setPosture(true, "ask")
	default:
		return m.setPosture(false, "auto")
	}
}

// toggleYolo enters YOLO, remembering the posture it left, and a second press
// goes back to that posture. The first time ever it only says what YOLO
// skips; a second press inside the window confirms it for good.
func (m *model) toggleYolo() tea.Cmd {
	if m.status.ToolApprovalMode == "yolo" {
		restore := m.yoloRestore
		if restore == "" || restore == "yolo" {
			restore = "ask"
		}
		m.yoloRestore = ""
		return m.setPosture(m.status.Plan, restore)
	}
	if !m.opts.YoloConfirmed {
		if m.yoloArmedAt.IsZero() || time.Since(m.yoloArmedAt) > yoloConfirmWindow {
			m.yoloArmedAt = time.Now()
			m.tr.AddNotice("warn", i18n.M.YoloConfirmHint)
			return m.commit()
		}
		m.yoloArmedAt = time.Time{}
		m.opts.YoloConfirmed = true
		if m.opts.ConfirmYolo != nil {
			if err := m.opts.ConfirmYolo(); err != nil {
				m.tr.AddNotice("warn", "YOLO: "+err.Error())
			}
		}
	}
	m.yoloRestore = m.status.ToolApprovalMode
	return m.setPosture(m.status.Plan, "yolo")
}

// setPosture moves the kernel to plan and mode together, plan first so a turn
// submitted right after never sees the new mode under the old workflow.
func (m *model) setPosture(plan bool, mode string) tea.Cmd {
	s := &m.status
	planChanged := s.Plan != plan
	s.Plan, s.ToolApprovalMode = plan, mode
	step := func(ctx context.Context) error {
		if planChanged {
			if err := m.client.SetPlan(ctx, plan); err != nil {
				return err
			}
		}
		return m.client.SetApprovalMode(ctx, mode)
	}
	return tea.Sequence(m.call("mode", step), m.fetchStatus())
}
