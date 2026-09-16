package bot

// The slash commands a chat platform can send, and the text each answers with.

import (
	"context"
	"fmt"
	"os"
	"reasonix/internal/agent"
	"reasonix/internal/control"
	"reasonix/internal/event"
	"reasonix/internal/sessioninbox"
	"strings"
	"time"
)

func (gw *BotGateway) handleSlashCommand(ctx context.Context, adapter Adapter, key string, msg InboundMessage) {
	switch {
	case strings.HasPrefix(msg.Text, "/stop"):
		var cancel context.CancelFunc
		gw.mu.Lock()
		if state, ok := gw.controllers[key]; ok {
			cancel = state.cancel
		}
		gw.mu.Unlock()
		if cancel != nil {
			cancel()
		}
		gw.sessions.ForceRelease(key)
		_ = gw.sendText(ctx, adapter, msg, "已停止当前任务。")

	case strings.HasPrefix(msg.Text, "/new") || strings.HasPrefix(msg.Text, "/reset"):
		var cancel context.CancelFunc
		gw.mu.Lock()
		state, ok := gw.controllers[key]
		if ok {
			cancel = state.cancel
		}
		gw.mu.Unlock()
		if ok {
			if cancel != nil {
				cancel()
			}
			// NewSession refuses to rotate while a turn is running; the cancel
			// above is asynchronous, so give the turn a bounded window to
			// unwind before rotating.
			deadline := time.Now().Add(5 * time.Second)
			for state.ctrl.Running() && time.Now().Before(deadline) {
				time.Sleep(10 * time.Millisecond)
			}
			if err := state.ctrl.NewSession(); err != nil {
				gw.logger.Warn("new session failed", "err", err)
				gw.sessions.ForceRelease(key)
				_ = gw.sendText(ctx, adapter, msg, "新会话创建失败，请稍后重试。")
				return
			}
			if state.leases != nil {
				if err := rebindBotSessionWriteAuthority(state, state.ctrl.SessionPath()); err != nil {
					gw.logger.Warn("new session lease failed", "err", control.SessionInUseMessage(err))
					gw.unlinkAndCloseSessionState(key, state)
					gw.sessions.ForceRelease(key)
					_ = gw.sendText(ctx, adapter, msg, "新会话创建失败：无法取得写入权限。请关闭其他 Reasonix 窗口或进程后重试。")
					return
				}
			}
			// /new leaves an attached transcript and continues in the freshly
			// rotated path. Clear only the path pin while preserving any project
			// override, otherwise the next message would rebuild the old attached
			// transcript and silently undo the rotation.
			gw.mu.Lock()
			if gw.controllers[key] == state {
				state.sessionPath = ""
				if override, exists := gw.sessionOverrides[key]; exists && override.sessionPath != "" {
					override.sessionPath = ""
					gw.sessionOverrides[key] = override
				}
			}
			gw.mu.Unlock()
			gw.rememberSessionReady(msg, state.ctrl)
		}
		gw.sessions.ForceRelease(key)
		_ = gw.sendText(ctx, adapter, msg, "已开始新会话。")

	case strings.HasPrefix(msg.Text, "/approve"):
		if !gw.requireCommandRole(ctx, adapter, msg, "approver") {
			return
		}
		// 从消息中解析 approval ID
		parts := strings.Fields(msg.Text)
		if len(parts) < 2 {
			_ = gw.sendText(ctx, adapter, msg, "用法: /approve <id>")
			return
		}
		gw.mu.Lock()
		state, ok := gw.controllers[key]
		gw.mu.Unlock()
		if ok && state.ctrl != nil {
			// Recovery cards map allow → continue for older clients that only know Approve.
			if gw.pendingApprovalIsRecovery(key, parts[1]) {
				_ = state.ctrl.ResolveRecovery(parts[1], agent.RecoveryActionContinue, "")
			} else {
				state.ctrl.Approve(parts[1], true, false, false)
			}
			gw.forgetPendingApproval(key, parts[1])
			_ = gw.sendText(ctx, adapter, msg, "已批准。")
		} else {
			_ = gw.sendText(ctx, adapter, msg, "没有找到当前会话中的待审批操作，请重新触发一次操作。")
		}

	case strings.HasPrefix(msg.Text, "/deny"):
		if !gw.requireCommandRole(ctx, adapter, msg, "approver") {
			return
		}
		parts := strings.Fields(msg.Text)
		if len(parts) < 2 {
			_ = gw.sendText(ctx, adapter, msg, "用法: /deny <id>")
			return
		}
		gw.mu.Lock()
		state, ok := gw.controllers[key]
		gw.mu.Unlock()
		if ok && state.ctrl != nil {
			if gw.pendingApprovalIsRecovery(key, parts[1]) {
				_ = state.ctrl.ResolveRecovery(parts[1], agent.RecoveryActionRevise, "")
			} else {
				state.ctrl.Approve(parts[1], false, false, false)
			}
			gw.forgetPendingApproval(key, parts[1])
			_ = gw.sendText(ctx, adapter, msg, "已拒绝。")
		} else {
			_ = gw.sendText(ctx, adapter, msg, "没有找到当前会话中的待审批操作，请重新触发一次操作。")
		}

	case strings.HasPrefix(msg.Text, "/recovery-continue-task"):
		if !gw.requireCommandRole(ctx, adapter, msg, "approver") {
			return
		}
		parts := strings.Fields(msg.Text)
		if len(parts) < 2 {
			_ = gw.sendText(ctx, adapter, msg, "用法: /recovery-continue-task <id>")
			return
		}
		gw.mu.Lock()
		state, ok := gw.controllers[key]
		gw.mu.Unlock()
		if ok && state.ctrl != nil {
			if err := state.ctrl.ResolveRecovery(parts[1], agent.RecoveryActionContinueTask, ""); err != nil {
				_ = gw.sendText(ctx, adapter, msg, "确认失败: "+err.Error())
				return
			}
			gw.forgetPendingApproval(key, parts[1])
			_ = gw.sendText(ctx, adapter, msg, "已继续；本任务内同类操作将自动执行，范围扩大或风险升级仍会确认。")
		} else {
			_ = gw.sendText(ctx, adapter, msg, "没有找到当前会话中的待确认操作。")
		}

	case strings.HasPrefix(msg.Text, "/recovery-continue"):
		if !gw.requireCommandRole(ctx, adapter, msg, "approver") {
			return
		}
		parts := strings.Fields(msg.Text)
		if len(parts) < 2 {
			_ = gw.sendText(ctx, adapter, msg, "用法: /recovery-continue <id>")
			return
		}
		gw.mu.Lock()
		state, ok := gw.controllers[key]
		gw.mu.Unlock()
		if ok && state.ctrl != nil {
			if err := state.ctrl.ResolveRecovery(parts[1], agent.RecoveryActionContinue, ""); err != nil {
				_ = gw.sendText(ctx, adapter, msg, "确认失败: "+err.Error())
				return
			}
			gw.forgetPendingApproval(key, parts[1])
			_ = gw.sendText(ctx, adapter, msg, "已继续。")
		} else {
			_ = gw.sendText(ctx, adapter, msg, "没有找到当前会话中的待确认操作。")
		}

	case strings.HasPrefix(msg.Text, "/recovery-revise"):
		if !gw.requireCommandRole(ctx, adapter, msg, "approver") {
			return
		}
		parts := strings.Fields(msg.Text)
		if len(parts) < 2 {
			_ = gw.sendText(ctx, adapter, msg, "用法: /recovery-revise <id> [补充要求]")
			return
		}
		feedback := strings.TrimSpace(strings.Join(parts[2:], " "))
		gw.mu.Lock()
		state, ok := gw.controllers[key]
		gw.mu.Unlock()
		if ok && state.ctrl != nil {
			if err := state.ctrl.ResolveRecovery(parts[1], agent.RecoveryActionRevise, feedback); err != nil {
				_ = gw.sendText(ctx, adapter, msg, "修改方案失败: "+err.Error())
				return
			}
			gw.forgetPendingApproval(key, parts[1])
			_ = gw.sendText(ctx, adapter, msg, "已拒绝当前变更并注入修改要求。")
		} else {
			_ = gw.sendText(ctx, adapter, msg, "没有找到当前会话中的恢复检查点。")
		}

	case strings.HasPrefix(msg.Text, "/recovery-stop"):
		// Backward compatibility for cards rendered by an older client: reject
		// the proposed mutation but leave task cancellation to ordinary /stop.
		if !gw.requireCommandRole(ctx, adapter, msg, "approver") {
			return
		}
		parts := strings.Fields(msg.Text)
		if len(parts) < 2 {
			_ = gw.sendText(ctx, adapter, msg, "用法: /recovery-stop <id>")
			return
		}
		gw.mu.Lock()
		state, ok := gw.controllers[key]
		gw.mu.Unlock()
		if ok && state.ctrl != nil {
			if err := state.ctrl.ResolveRecovery(parts[1], agent.RecoveryActionRevise, "cancel this proposed action"); err != nil {
				_ = gw.sendText(ctx, adapter, msg, "取消变更失败: "+err.Error())
				return
			}
			gw.forgetPendingApproval(key, parts[1])
			_ = gw.sendText(ctx, adapter, msg, "已取消当前变更；如需停止整个任务，请使用 /stop。")
		} else {
			_ = gw.sendText(ctx, adapter, msg, "没有找到当前会话中的恢复检查点。")
		}

	case strings.HasPrefix(msg.Text, "/answer"):
		parts := strings.Fields(msg.Text)
		if len(parts) < 3 {
			_ = gw.sendText(ctx, adapter, msg, "用法: /answer <id> <选项或 q1=选项;q2=选项>")
			return
		}
		askID := parts[1]
		rawAnswer := strings.TrimSpace(strings.Join(parts[2:], " "))
		gw.mu.Lock()
		state, ok := gw.controllers[key]
		var questions []event.AskQuestion
		if ok {
			questions = state.pendingAsks[askID]
			delete(state.pendingAsks, askID)
			if state.lastAskID == askID {
				state.lastAskID = ""
				for nextID := range state.pendingAsks {
					state.lastAskID = nextID
					break
				}
			}
		}
		gw.mu.Unlock()
		if !ok || state.ctrl == nil {
			_ = gw.sendText(ctx, adapter, msg, "没有找到当前会话。")
			return
		}
		answers := parseAskAnswers(questions, rawAnswer)
		state.ctrl.AnswerQuestion(askID, answers)
		_ = gw.sendText(ctx, adapter, msg, "已提交回答。")

	case strings.HasPrefix(msg.Text, "/yolo") || strings.HasPrefix(msg.Text, "/mode"):
		if !gw.requireCommandRole(ctx, adapter, msg, "admin") {
			return
		}
		mode, statusOnly, ok := parseToolApprovalModeCommand(msg.Text)
		if !ok {
			_ = gw.sendText(ctx, adapter, msg, "用法: /yolo on|off|auto|status，或 /mode yolo|ask|auto")
			return
		}
		if statusOnly {
			_ = gw.sendText(ctx, adapter, msg, gw.toolApprovalModeStatusText(key, msg))
			return
		}
		persistErr := gw.setToolApprovalModeForMessage(key, msg, mode)
		text := toolApprovalModeChangedText(mode)
		if persistErr != nil {
			text += "\n当前会话已生效，但保存到设置失败：" + persistErr.Error()
		}
		_ = gw.sendText(ctx, adapter, msg, text)

	case strings.HasPrefix(msg.Text, "/queue"):
		if reply, handled, kick := gw.handleQueueInboxCommand(ctx, key, msg); handled {
			_ = gw.sendText(ctx, adapter, msg, reply)
			if kick {
				gw.kickInbox(ctx, adapter, key, msg)
			}
			return
		}
		mode, clear, statusOnly, ok := parseQueueCommand(msg.Text)
		if !ok {
			_ = gw.sendText(ctx, adapter, msg, "用法: /queue steer|followup|collect|interrupt|status|list|show|delete|move|pause|resume|retry|default")
			return
		}
		if statusOnly {
			_ = gw.sendText(ctx, adapter, msg, gw.queueStatusText(key, msg))
			return
		}
		if clear {
			gw.sessions.ClearQueueMode(key)
			_ = gw.sendText(ctx, adapter, msg, "已恢复默认队列模式："+queueModeLabel(gw.queueMode(key, msg))+"。")
			return
		}
		gw.sessions.SetQueueMode(key, mode)
		_ = gw.sendText(ctx, adapter, msg, "已切换队列模式："+queueModeLabel(mode)+"。")

	case slashCommandVerb(msg.Text) == "/projects":
		if !gw.requireCommandRole(ctx, adapter, msg, "admin") {
			return
		}
		query := strings.TrimSpace(strings.TrimPrefix(msg.Text, "/projects"))
		_ = gw.sendText(ctx, adapter, msg, formatBotProjects(gw.buildProjectIndex(), query, botProjectListLimit))

	case slashCommandVerb(msg.Text) == "/use":
		if !gw.requireCommandRole(ctx, adapter, msg, "admin") {
			return
		}
		_ = gw.sendText(ctx, adapter, msg, gw.handleUseProjectCommand(key, msg.Text))

	case slashCommandVerb(msg.Text) == "/sessions":
		if !gw.requireCommandRole(ctx, adapter, msg, "admin") {
			return
		}
		_ = gw.sendText(ctx, adapter, msg, gw.handleSessionsCommand(msg.Text))

	case slashCommandVerb(msg.Text) == "/attach":
		if !gw.requireCommandRole(ctx, adapter, msg, "admin") {
			return
		}
		_ = gw.sendText(ctx, adapter, msg, gw.handleAttachSessionCommand(key, msg.Text))

	case slashCommandVerb(msg.Text) == "/search":
		if !gw.requireCommandRole(ctx, adapter, msg, "admin") {
			return
		}
		_ = gw.sendText(ctx, adapter, msg, gw.handleProjectSearchCommand(ctx, msg.Text))

	case strings.HasPrefix(msg.Text, "/desktop"):
		// God view over the embedding desktop app: listing every live desktop
		// session and answering its approvals is strictly more power than the
		// per-session approver role, so gate on admin.
		if !gw.requireCommandRole(ctx, adapter, msg, "admin") {
			return
		}
		_ = gw.sendText(ctx, adapter, msg, gw.handleDesktopCommand(msg))

	case strings.HasPrefix(msg.Text, "/status"):
		active := gw.sessions.ActiveCount()
		pending := gw.sessions.PendingCount(key)
		gw.mu.Lock()
		sessions := len(gw.controllers)
		gw.mu.Unlock()
		mode := gw.currentToolApprovalMode(key, msg)
		_ = gw.sendText(ctx, adapter, msg, fmt.Sprintf("活跃任务数: %d\n保留会话数: %d\n工具审批模式: %s\n队列模式: %s\n当前会话排队: %d\n连接健康: %s", active, sessions, toolApprovalModeLabel(mode), queueModeLabel(gw.queueMode(key, msg)), pending, gw.adapterHealthSummaryText()))

	case strings.HasPrefix(msg.Text, "/help"):
		help := "可用命令:\n" +
			"/stop - 停止当前任务\n" +
			"/new - 开始新会话\n" +
			"/reset - 重置会话\n" +
			"/approve <id> - 批准操作\n" +
			"/deny <id> - 拒绝操作\n" +
			"/answer <id> <选项> - 回答 ask 问题\n" +
			"/yolo on|off|auto|status - 切换或查看工具审批模式\n" +
			"/mode yolo|ask|auto - 切换工具审批模式\n" +
			"/queue steer|followup|collect|interrupt|status - 切换或查看队列模式\n" +
			"/projects [关键词] - 查看可切换项目索引\n" +
			"/use project <id|名称> - 将当前远端会话切到某个项目\n" +
			"/sessions search <关键词> - 搜索可 attach 的历史会话\n" +
			"/attach session <id|关键词> - 绑定当前远端会话到已有历史会话\n" +
			"/search all <关键词> - 跨已索引项目检索文件内容\n" +
			"/desktop status|watch|approve|deny|answer - 桌面端上帝视角(需内嵌运行)\n" +
			"/status - 查看状态\n" +
			"/help - 显示帮助"
		_ = gw.sendText(ctx, adapter, msg, help)
	}
}

func (gw *BotGateway) kickInbox(ctx context.Context, adapter Adapter, key string, fallback InboundMessage) {
	if gw.sessions.IsActive(key) {
		return
	}
	next := gw.nextInboxTurn(key, fallback)
	if next == nil {
		return
	}
	if !gw.sessions.TryAcquireIdle(key) {
		return
	}
	gw.turnWG.Go(func() {
		gw.runTurnItem(ctx, adapter, key, next.msg, next.itemID, nil)
	})
}

func slashCommandVerb(text string) string {
	parts := strings.Fields(strings.TrimSpace(text))
	if len(parts) == 0 {
		return ""
	}
	return strings.ToLower(parts[0])
}

func (gw *BotGateway) handleUseProjectCommand(key, text string) string {
	selector := parseUseProjectSelector(text)
	if selector == "" {
		return "用法: /use project <项目 id|名称|路径>，或 /use project default 恢复默认路由。"
	}
	if isDefaultBotSelector(selector) {
		if !gw.setSessionRuntimeOverride(key, sessionRuntimeOverride{}, false) {
			return botRuntimeSwitchBusyText()
		}
		return "已恢复当前远端会话的默认项目路由。下一条消息会按 bot 配置重新选择 workspace。"
	}
	projects := gw.buildProjectIndex()
	project, matches := resolveBotProject(projects, selector)
	if project.Root == "" {
		if len(matches) > 0 {
			return "匹配到多个项目，请使用项目 id：\n" + formatBotProjects(matches, "", botProjectListLimit)
		}
		return "没有匹配的项目。可先用 /projects 查看当前索引。"
	}
	if !gw.setSessionRuntimeOverride(key, sessionRuntimeOverride{
		channel: ChannelConfig{WorkspaceRoot: project.Root},
		label:   "project:" + project.ID,
	}, true) {
		return botRuntimeSwitchBusyText()
	}
	return fmt.Sprintf("已将当前远端会话切到项目 %s %s。\n下一条消息将在 %s 中运行。", project.ID, project.Name, displayBotPath(project.Root))
}

func parseUseProjectSelector(text string) string {
	parts := strings.Fields(text)
	if len(parts) < 2 || strings.ToLower(parts[0]) != "/use" {
		return ""
	}
	if len(parts) >= 3 && strings.EqualFold(parts[1], "project") {
		return strings.TrimSpace(strings.Join(parts[2:], " "))
	}
	return strings.TrimSpace(strings.Join(parts[1:], " "))
}

func (gw *BotGateway) handleSessionsCommand(text string) string {
	query := parseSessionsQuery(text)
	projects := gw.buildProjectIndex()
	sessions := gw.buildSessionIndex(projects)
	return formatBotSessions(sessions, query, botSessionListLimit)
}

func parseSessionsQuery(text string) string {
	parts := strings.Fields(text)
	if len(parts) <= 1 {
		return ""
	}
	if strings.EqualFold(parts[1], "search") {
		return strings.TrimSpace(strings.Join(parts[2:], " "))
	}
	return strings.TrimSpace(strings.Join(parts[1:], " "))
}

func (gw *BotGateway) handleAttachSessionCommand(key, text string) string {
	selector := parseAttachSessionSelector(text)
	if selector == "" {
		return "用法: /attach session <会话 id|关键词|path:...>"
	}
	projects := gw.buildProjectIndex()
	sessions := gw.buildSessionIndex(projects)
	session, matches := resolveBotSession(sessions, selector)
	if session.ID == "" {
		if len(matches) > 0 {
			return "匹配到多个会话，请使用会话 id：\n" + formatBotSessions(matches, "", botSessionListLimit)
		}
		return "没有匹配的会话。可先用 /sessions search <关键词> 查看当前索引。"
	}
	if session.SessionPath == "" {
		return "这个会话没有可恢复的 path: transcript，暂时不能 attach。"
	}
	if info, err := os.Stat(session.SessionPath); err != nil || info.IsDir() {
		return "会话文件不可用或已被移动：" + displayBotPath(session.SessionPath)
	}
	workspaceRoot := session.WorkspaceRoot
	if workspaceRoot == "" {
		project := botProjectForPath(projects, session.SessionPath)
		workspaceRoot = project.Root
	}
	if !gw.setSessionRuntimeOverride(key, sessionRuntimeOverride{
		channel:     ChannelConfig{WorkspaceRoot: workspaceRoot},
		sessionPath: session.SessionPath,
		label:       "session:" + session.ID,
	}, true) {
		return botRuntimeSwitchBusyText()
	}
	projectName := firstNonEmptyString(session.ProjectName, botProjectName(workspaceRoot), "global")
	return fmt.Sprintf("已 attach 到会话 %s（%s）。\n下一条消息会从 %s 继续。", session.ID, projectName, displayBotPath(session.SessionPath))
}

func parseAttachSessionSelector(text string) string {
	parts := strings.Fields(text)
	if len(parts) < 3 || !strings.EqualFold(parts[0], "/attach") || !strings.EqualFold(parts[1], "session") {
		return ""
	}
	return strings.TrimSpace(strings.Join(parts[2:], " "))
}

func (gw *BotGateway) handleProjectSearchCommand(ctx context.Context, text string) string {
	parts := strings.Fields(text)
	if len(parts) < 3 || !strings.EqualFold(parts[1], "all") {
		return "用法: /search all <关键词>"
	}
	query := strings.TrimSpace(strings.Join(parts[2:], " "))
	searchCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	results, err := searchBotProjects(searchCtx, gw.buildProjectIndex(), query, botSearchListLimit)
	if err != nil {
		return "检索失败：" + err.Error()
	}
	return formatBotProjectSearchResults(results, botSearchListLimit)
}

func botRuntimeSwitchBusyText() string {
	return "当前会话仍有正在运行、等待确认或后台执行的任务。请先完成或停止这些任务，再切换项目或 attach 会话。"
}

func isDefaultBotSelector(selector string) bool {
	switch strings.ToLower(strings.TrimSpace(selector)) {
	case "default", "reset", "inherit", "global", "none", "默认", "重置":
		return true
	default:
		return false
	}
}

func parseQueueCommand(text string) (mode string, clear bool, statusOnly bool, ok bool) {
	parts := strings.Fields(text)
	if len(parts) == 0 || strings.ToLower(strings.TrimSpace(parts[0])) != "/queue" {
		return "", false, false, false
	}
	if len(parts) == 1 {
		return "", false, true, true
	}
	switch strings.ToLower(strings.TrimSpace(parts[1])) {
	case "status", "state", "show", "状态", "查看":
		return "", false, true, true
	case "default", "reset", "inherit", "默认", "重置":
		return "", true, false, true
	default:
		if normalized := NormalizeOptionalQueueMode(parts[1]); normalized != "" {
			return normalized, false, false, true
		}
		return "", false, false, false
	}
}

func (gw *BotGateway) queueStatusText(key string, msg InboundMessage) string {
	inboxN := 0
	paused := false
	if api := gw.sessionAPI(key); api != nil {
		snap := api.InboxSnapshot()
		inboxN = len(snap.Items)
		paused = snap.Paused
	}
	return fmt.Sprintf("当前队列模式：%s\n持久化 Inbox: %d%s\n全局上限: %d\n溢出策略: 拒绝新消息（queue_drop 已弃用）\n用法：/queue steer|followup|collect|interrupt|status|list|show|delete|move|pause|resume|retry|default",
		queueModeLabel(gw.queueMode(key, msg)),
		inboxN,
		map[bool]string{true: " (paused)", false: ""}[paused],
		sessioninbox.DefaultMaxItems,
	)
}

func queueModeLabel(mode string) string {
	switch NormalizeQueueMode(mode) {
	case QueueModeFollowup:
		return "逐条跟进"
	case QueueModeCollect:
		return "合并收集"
	case QueueModeInterrupt:
		return "打断重跑"
	default:
		return "即时补充"
	}
}

func parseToolApprovalModeCommand(text string) (mode string, statusOnly bool, ok bool) {
	parts := strings.Fields(text)
	if len(parts) == 0 {
		return "", false, false
	}
	cmd := strings.ToLower(strings.TrimSpace(parts[0]))
	switch cmd {
	case "/yolo":
		if len(parts) == 1 {
			return control.ToolApprovalYolo, false, true
		}
		return parseToolApprovalModeArg(parts[1])
	case "/mode":
		if len(parts) == 1 {
			return "", true, true
		}
		return parseToolApprovalModeArg(parts[1])
	default:
		return "", false, false
	}
}

func parseToolApprovalModeArg(arg string) (mode string, statusOnly bool, ok bool) {
	switch strings.ToLower(strings.TrimSpace(arg)) {
	case "status", "state", "show", "状态", "查看":
		return "", true, true
	case "on", "enable", "enabled", "true", "1", "yolo", "full", "full-access", "bypass", "开启", "打开":
		return control.ToolApprovalYolo, false, true
	case "off", "disable", "disabled", "false", "0", "ask", "询问", "关闭":
		return control.ToolApprovalAsk, false, true
	case "auto", "自动":
		return control.ToolApprovalAuto, false, true
	default:
		return "", false, false
	}
}

func (gw *BotGateway) toolApprovalModeStatusText(key string, msg InboundMessage) string {
	mode := gw.currentToolApprovalMode(key, msg)
	return fmt.Sprintf("当前工具审批模式：%s\n用法：/yolo on|off|auto|status，或 /mode yolo|ask|auto", toolApprovalModeLabel(mode))
}

func toolApprovalModeChangedText(mode string) string {
	switch normalizeBotToolApprovalMode(mode) {
	case control.ToolApprovalYolo:
		return "已开启 YOLO：普通工具审批将自动放行；Ask 问题和计划批准仍会等待确认。"
	case control.ToolApprovalAuto:
		return "已切换为自动模式：策略允许的工具会自动放行，仍保留需要询问或拒绝的规则。"
	default:
		return "已切回询问模式：工具执行前会请求确认。"
	}
}

func toolApprovalModeLabel(mode string) string {
	switch normalizeBotToolApprovalMode(mode) {
	case control.ToolApprovalYolo:
		return "YOLO"
	case control.ToolApprovalAuto:
		return "自动"
	default:
		return "询问"
	}
}
