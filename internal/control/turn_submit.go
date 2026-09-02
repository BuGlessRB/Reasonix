package control

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"reasonix/internal/skill"
)

// Submit is the one-call entry for a simple frontend: it takes raw user input
// and does everything — slash-command dispatch, @-reference expansion, plan-mode
// composition — emitting all output as events. The HTTP/SSE server uses this so
// a browser client only POSTs the typed line.
//
// Slash commands route to the matching primitive: /compact, /new, and /clear
// run their session op and emit a Notice; /mcp__server__prompt and custom /commands
// resolve to a turn; an unknown slash emits a Notice. Anything else is a normal
// turn with its @-references resolved first.
func (c *Controller) Submit(input string) {
	c.submit(input, "", "")
}

// SubmitHTTP accepts input from the unauthenticated localhost HTTP frontend. It
// deliberately omits the trusted TUI-only "!cmd" shell shortcut and resolves file
// references only through the controller's workspace root.
func (c *Controller) SubmitHTTP(input string) {
	c.submitHTTP(input, "")
}

// SubmitHTTPFormat is SubmitHTTP with an optional structured-output format
// ("json_object") applied to the turn's completion requests. Empty format
// behaves exactly like SubmitHTTP. A format attached to a slash command,
// or other non-turn input is discarded; @reference turns preserve it because
// the format is bound to every submitted turn rather than a global slot.
func (c *Controller) SubmitHTTPFormat(input, format string) {
	// format 绑定到本次提交的 turn（随请求参数传递），不再写入 Controller
	// 全局一次性槽——评审 #7234 第 2 点：全局槽存在跨请求串用的逻辑竞态
	// （后提交的 JSON 请求先写槽，更早的普通请求先启动消费掉）。
	f := strings.TrimSpace(format)
	if f != "" && isNonTurnHTTPInput(input) {
		f = "" // 非 turn 输入（slash 命令/! 前缀）不携带 format
	}
	// @ 引用 turn（FileRefLine/SlashPathLineRef 等）同样绑定 format——
	// runRefTurnWithFormat 族 wrapper 注入 ctx（review fix7234and7168：
	// format 是每个被接纳 turn 的属性，统一架构）。
	c.submitHTTPWithFormat(input, "", f)
}

// SubmitDisplay runs input as a turn while remembering the user-facing display
// text for transcript replay when controller-side composition expands input.
func (c *Controller) SubmitDisplay(display, input string) {
	c.submit(input, display, "")
}

// SubmitDeliveryRecovery runs the same visible prompt path as SubmitDisplay but
// first authorizes the executor to retain the immediately preceding exhausted
// delivery ledger. The agent consumes that authorization once; if the card came
// from an older/reloaded session this safely degrades to an ordinary turn.
func (c *Controller) SubmitDeliveryRecovery(display, input string) {
	c.runGuarded(func(ctx context.Context) error {
		if c.executor != nil {
			c.executor.PrepareDeliveryRecovery()
		}
		return c.runTurnLoop(ctx, orchestratedTurn{input: input, raw: input, display: display})
	})
}

// SubmitInvocationDisplay executes composer-selected invocation entities
// independently of slash-command parsing. Plain string submit entry points keep
// their existing behavior for CLI, HTTP, and backward-compatible clients.
func (c *Controller) SubmitInvocationDisplay(display, input string, invocations []InvocationRequest) {
	c.submitInvocations(input, display, invocations)
}

func (c *Controller) submitInvocations(input, display string, requests []InvocationRequest) {
	if len(requests) == 0 {
		c.SubmitDisplay(display, input)
		return
	}
	prepared, err := c.prepareInvocationTurn(input, requests)
	if err != nil {
		c.notice(err.Error())
		return
	}
	c.runGuarded(func(ctx context.Context) error {
		return c.runPreparedInvocationTurn(ctx, prepared, input, input, display, nil)
	})
}

func (c *Controller) prepareInvocationTurn(input string, requests []InvocationRequest) (preparedInvocationTurn, error) {
	ordered := append([]InvocationRequest(nil), requests...)
	sort.SliceStable(ordered, func(i, j int) bool { return ordered[i].Offset < ordered[j].Offset })
	inline := make([]skill.Skill, 0, len(ordered))
	subagents := make([]skill.Skill, 0, len(ordered))
	for _, request := range ordered {
		sk, _, ok := c.resolveSkillInvocation("/" + strings.TrimSpace(request.Name))
		if !ok {
			return preparedInvocationTurn{}, fmt.Errorf("unknown invocation: /%s", strings.TrimSpace(request.Name))
		}
		kind := "skill"
		if sk.RunAs == skill.RunSubagent {
			kind = "subagent"
		}
		if strings.TrimSpace(request.Kind) != "" && request.Kind != kind {
			return preparedInvocationTurn{}, fmt.Errorf("invocation /%s is %s, not %s", sk.SlashName(), kind, request.Kind)
		}
		if sk.RunAs == skill.RunSubagent {
			subagents = append(subagents, sk)
		} else {
			inline = append(inline, sk)
		}
	}

	parts := make([]string, 0, len(inline)+1)
	for _, sk := range inline {
		parts = append(parts, c.skills.render(sk, ""))
	}
	if strings.TrimSpace(input) != "" {
		parts = append(parts, input)
	}
	composed := strings.Join(parts, "\n\n")
	if strings.TrimSpace(input) == "" {
		if len(subagents) > 0 {
			return preparedInvocationTurn{}, fmt.Errorf("subagent invocation requires a task")
		}
	}
	return preparedInvocationTurn{composed: composed, subagents: subagents}, nil
}

func (c *Controller) runPreparedInvocationTurn(
	ctx context.Context,
	prepared preparedInvocationTurn,
	input, raw, display string,
	frozenImages []string,
) error {
	if len(prepared.subagents) == 0 {
		return c.runTurnLoop(ctx, orchestratedTurn{
			input: prepared.composed, raw: raw, display: display, images: c.frozenTurnImages(frozenImages),
		})
	}
	runner := c.skillRunner
	if runner == nil {
		return fmt.Errorf("subagent skill runner is unavailable")
	}
	return newTurnOrchestrator(c).runSubagentSkillTurnsGoalLoop(
		ctx,
		prepared.subagents,
		prepared.composed,
		input,
		display,
		runner,
		c.PlanMode(),
	)
}

// SubmitEditedDisplay is SubmitDisplay for an inline-edited prompt. The model
// sees input; the saved user message also keeps the pre-edit prompt as local UI
// metadata so the edit survives session rewrites.
func (c *Controller) SubmitEditedDisplay(display, input, original string) {
	c.submit(input, display, original)
}

// SubmitUserTurn starts a normal model turn without interpreting shell or slash
// commands. It still resolves references, so callers can submit trusted
// user-authored prompt text without expanding the command surface.
func (c *Controller) SubmitUserTurn(display, input string) {
	c.runRefTurn(refTurn{input: input, display: display})
}

func (c *Controller) submit(input, display, editedOriginal string) {
	trimmed := strings.TrimSpace(input)
	if note, ok := MemoryQuickAddNote(trimmed); ok {
		c.rememberProjectNote(note)
		return
	}
	if note, ok := RememberCommandNote(trimmed); ok {
		c.rememberProjectNote(note)
		return
	}
	if c.applyGoalCommand(trimmed, display) {
		return
	}
	if strings.HasPrefix(trimmed, "!") {
		c.RunShell(trimmed[1:])
		return
	}
	c.submitCommandOrTurn(trimmed, input, display, false, editedOriginal, "")
}

func (c *Controller) submitHTTP(input, display string) {
	c.submitHTTPWithFormat(input, display, "")
}

func (c *Controller) submitHTTPWithFormat(input, display, format string) {
	trimmed := strings.TrimSpace(input)
	if note, ok := MemoryQuickAddNote(trimmed); ok {
		c.rememberProjectNote(note)
		return
	}
	if note, ok := RememberCommandNote(trimmed); ok {
		c.rememberProjectNote(note)
		return
	}
	if c.applyGoalCommand(trimmed, display) {
		return
	}
	if strings.HasPrefix(trimmed, "!") {
		c.notice("shell commands are unavailable from this frontend")
		return
	}
	c.submitCommandOrTurn(trimmed, input, display, true, "", format)
}

// refTurnBase is the shape every ref turn from one submitted line shares. An
// edited resubmit resolves against the whole workspace and rides the
// edited-goal loop, so it overrides scopedRefsOnly rather than combining with
// it.
func (c *Controller) refTurnBase(display, editedOriginal, format string, scopedRefsOnly bool) refTurn {
	base := refTurn{display: display, original: editedOriginal, format: format}
	if scopedRefsOnly && strings.TrimSpace(editedOriginal) == "" {
		base.resolve = c.ResolveScopedRefs
	}
	return base
}

// turnLoopRunner is the loop that same line runs in: an edited resubmit rides
// the edited-goal loop, everything else the plain one with its format bound.
func (c *Controller) turnLoopRunner(editedOriginal, format string) func(context.Context, string, string, string) error {
	if strings.TrimSpace(editedOriginal) != "" {
		return func(ctx context.Context, input, raw, display string) error {
			return c.runTurnLoop(ctx, orchestratedTurn{
				input: input, raw: raw, display: display, editedOriginal: editedOriginal,
			})
		}
	}
	return func(ctx context.Context, input, raw, display string) error {
		return c.runTurnLoop(c.withTurnFormat(ctx, format), orchestratedTurn{input: input, raw: raw, display: display})
	}
}

func (c *Controller) submitCommandOrTurnReady(trimmed, input, display string, scopedRefsOnly bool, editedOriginal, format string) {
	base := c.refTurnBase(display, editedOriginal, format, scopedRefsOnly)
	runRefTurn := func(input, display string) {
		r := base
		r.input, r.display = input, display
		c.runRefTurn(r)
	}
	runRefTurnWithRefs := func(input, refLine, display string) {
		r := base
		r.input, r.refLine, r.display = input, refLine, display
		c.runRefTurn(r)
	}
	runTurnLoop := c.turnLoopRunner(editedOriginal, format)
	switch {
	case trimmed == "/compact" || strings.HasPrefix(trimmed, "/compact "):
		go c.compactAndReport(strings.TrimSpace(strings.TrimPrefix(trimmed, "/compact")))
	case trimmed == "/context":
		c.noticeDetail(c.ContextReport())
	case trimmed == "/new":
		c.runSessionVerb(c.NewSession, "new session", "new session failed: ")
	case trimmed == "/clear":
		c.runSessionVerb(c.ClearSession, "context cleared", "clear context failed: ")
	case strings.HasPrefix(trimmed, "/mcp__"):
		c.runGuarded(func(ctx context.Context) error {
			sent, found, err := c.MCPPrompt(ctx, trimmed)
			if err != nil {
				return err
			}
			if !found {
				c.notice("unknown command: " + trimmed)
				return nil
			}
			return runTurnLoop(ctx, sent, sent, display)
		})
	case SlashCodeCommentLine(trimmed):
		// Slash-prefixed code comments are prompt text, not slash commands.
		runRefTurn(input, display)
	case strings.HasPrefix(trimmed, "/"):
		if ref, ok := FileRefLine(trimmed); ok {
			runRefTurn(ref, display)
			return
		}
		if ref, ok := SlashPathLineRef(trimmed, c.workspaceRoot); ok {
			runRefTurnWithRefs(input, ref, display)
			return
		}
		if SlashPathLikeLine(trimmed) {
			runRefTurn(input, display)
			return
		}
		// Management verbs (/model /memory /skills /hooks /mcp) emit a Notice, so
		// Submit-based frontends (desktop, HTTP) get them with no extra wiring.
		// The chat TUI handles these itself with richer output.
		fields := strings.Fields(trimmed)
		switch fields[0] {
		case "/tree":
			c.notice(c.BranchTreeText())
			return
		case "/branch":
			name := strings.TrimSpace(strings.TrimPrefix(trimmed, fields[0]))
			if _, err := c.Branch(name); err != nil {
				c.notice(err.Error())
			}
			return
		case "/switch":
			ref := strings.TrimSpace(strings.TrimPrefix(trimmed, fields[0]))
			if _, err := c.SwitchBranch(ref); err != nil {
				c.notice(err.Error())
			}
			return
		case "/rewind":
			args := strings.TrimSpace(strings.TrimPrefix(trimmed, fields[0]))
			turn, scope, err := parseRewind(args, c.Checkpoints())
			if err != nil {
				c.notice("usage: /rewind [turn] [code|conversation|both]")
				return
			}
			if err := c.Rewind(turn, scope); err != nil {
				c.notice(err.Error())
			}
			return
		case "/plan-exec":
			c.applyPlanExec(trimmed, display)
			return
		case "/prometheus":
			c.applyPrometheus(trimmed, display)
			return
		}
		if c.managementNotice(trimmed) {
			return
		}
		if IsBuiltinDocsSlash(fields[0], c.Commands(), c.SlashSkills()) {
			query := strings.TrimSpace(strings.TrimPrefix(trimmed, fields[0]))
			if query == "" {
				text, err := DocsCommandOverviewFor(fields[0])
				if err != nil {
					c.notice("docs: " + err.Error())
				} else {
					c.notice(text)
				}
				return
			}
			c.runGuarded(func(ctx context.Context) error {
				sent, err := docsCommandPrompt(ctx, query)
				if err != nil {
					return fmt.Errorf("docs: %w", err)
				}
				return runTurnLoop(ctx, sent, sent, display)
			})
			return
		}
		// A custom command wins over a skill of the same name; both resolve to a
		// turn. Built-ins and their explicit Reasonix namespace are handled above.
		if sent, ok := c.CustomCommand(trimmed); ok {
			c.runGuarded(func(ctx context.Context) error {
				return runTurnLoop(ctx, sent, sent, display)
			})
			return
		}
		if sk, task, ok := c.resolveSkillInvocation(trimmed); ok {
			if sk.RunAs == skill.RunSubagent {
				if strings.TrimSpace(task) == "" {
					c.notice("usage: /" + sk.Name + " <task>")
					return
				}
				c.runSubagentSkillSlash(sk, task, trimmed, display)
				return
			}
			sent := c.skills.render(sk, task)
			c.runGuarded(func(ctx context.Context) error {
				return runTurnLoop(ctx, sent, sent, display)
			})
			return
		}
		// Unknown slash input is prose more often than a typo ("/etc/hosts
		// looks wrong", pasted paths, half-remembered commands) — send it as a
		// regular message instead of dead-ending the submission, with a notice
		// so real typos are still visible (#5756).
		c.notice("unknown command: " + trimmed + " — sent as a regular message")
		runRefTurn(input, display)
	default:
		runRefTurn(input, display)
	}
}
