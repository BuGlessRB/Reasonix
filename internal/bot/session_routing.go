package bot

// Which session a message belongs to: the mapping from a chat to a workspace
// and runtime, the options a session is opened with, and what closing one does.

import (
	"context"
	"os"
	"reasonix/internal/agent"
	"reasonix/internal/boot"
	"reasonix/internal/config"
	"reasonix/internal/control"
	"reasonix/internal/event"
	"reasonix/internal/secrets"
	"reasonix/internal/surface"
	"strings"
	"time"
)

func (gw *BotGateway) closeSessions() {
	var states []*sessionState
	gw.mu.Lock()
	for key, state := range gw.controllers {
		states = append(states, state)
		delete(gw.controllers, key)
	}
	gw.mu.Unlock()
	for _, state := range states {
		gw.closeSessionState(state)
	}
}

// closeSessionState tears down a session state that has been unlinked from
// gw.controllers. runTurn publishes state.cancel under gw.mu on every turn —
// possibly after the state was already unlinked — so snapshot and clear the
// field inside the lock and invoke it outside (the same discipline as
// cancelActiveSession).
func (gw *BotGateway) closeSessionState(state *sessionState) {
	if state == nil {
		return
	}
	// Serialize retirement with recovery ownership handoffs. Stop unlinks
	// sessions before turn goroutines drain, so a recovery callback captured by
	// the controller can still arrive here. Marking the state retired under the
	// same lock prevents that callback from reacquiring a lease after teardown;
	// an already-running handoff completes before the lease is released below.
	state.lifecycleMu.Lock()
	if state.retired {
		state.lifecycleMu.Unlock()
		return
	}
	state.retired = true
	state.lifecycleMu.Unlock()

	gw.mu.Lock()
	cancel := state.cancel
	state.cancel = nil
	gw.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	if state.ctrl != nil {
		state.ctrl.Close()
	}
	if state.leases != nil {
		state.leases.Release()
	}
}

// unlinkAndCloseSessionState removes state from the live gateway before closing
// it. It is used when a controller has already rotated its transcript but the
// replacement lease could not be acquired: retaining that state would let the
// next message reuse a controller that no longer owns its active session path.
func (gw *BotGateway) unlinkAndCloseSessionState(key string, state *sessionState) {
	if state == nil {
		return
	}
	gw.mu.Lock()
	if gw.controllers[key] == state {
		delete(gw.controllers, key)
	}
	gw.mu.Unlock()
	gw.closeSessionState(state)
}

func (gw *BotGateway) setSessionRuntimeOverride(key string, override sessionRuntimeOverride, enabled bool) bool {
	return gw.sessions.runIfIdle(key, func() bool {
		var old *sessionState
		gw.mu.Lock()
		if state, ok := gw.controllers[key]; ok {
			if botSessionHasActiveWork(state) {
				gw.mu.Unlock()
				return false
			}
			old = state
			delete(gw.controllers, key)
		}
		if enabled {
			override.sessionPath = canonicalBotPath(override.sessionPath)
			override.channel.WorkspaceRoot = canonicalBotPath(override.channel.WorkspaceRoot)
			gw.sessionOverrides[key] = override
		} else {
			delete(gw.sessionOverrides, key)
		}
		gw.mu.Unlock()
		gw.closeSessionState(old)
		return true
	})
}

func botSessionHasActiveWork(state *sessionState) bool {
	if state == nil || state.ctrl == nil {
		return false
	}
	status, ok := safeBotControllerRuntimeStatus(state.ctrl)
	if !ok {
		return true
	}
	return status.Running || status.PendingPrompt || status.BackgroundJobs > 0
}

func safeBotControllerRuntimeStatus(ctrl botController) (status control.RuntimeStatus, ok bool) {
	if ctrl == nil {
		return control.RuntimeStatus{}, false
	}
	defer func() {
		if recover() != nil {
			status = control.RuntimeStatus{}
			ok = false
		}
	}()
	return ctrl.RuntimeStatus(), true
}

func (gw *BotGateway) sessionRuntimeOverrideForMessage(msg InboundMessage) (sessionRuntimeOverride, bool) {
	key := BuildSessionKey(msg.Session())
	gw.mu.Lock()
	defer gw.mu.Unlock()
	override, ok := gw.sessionOverrides[key]
	return override, ok
}

func (gw *BotGateway) getOrCreateSession(ctx context.Context, key string, msg InboundMessage) *sessionState {
	profile := gw.sessionProfileForMessage(msg)
	var stale *sessionState
	gw.mu.Lock()
	if state, ok := gw.controllers[key]; ok {
		if !sessionStateMatchesRuntime(state, profile) {
			if botSessionHasActiveWork(state) {
				gw.mu.Unlock()
				safeBotSetToolApprovalMode(state.ctrl, profile.toolApprovalMode)
				gw.logger.Warn("bot session runtime change deferred while work is active", "platform", msg.Platform, "chat_type", msg.ChatType, "chat", hashID(msg.ChatID), "session", key[:8])
				return state
			}
			delete(gw.controllers, key)
			stale = state
			gw.mu.Unlock()
			gw.closeSessionState(stale)
			gw.logger.Warn("bot session runtime changed; rebuilding", "platform", msg.Platform, "chat_type", msg.ChatType, "chat", hashID(msg.ChatID), "session", key[:8], "old_workspace_set", strings.TrimSpace(stale.workspaceRoot) != "", "new_workspace_set", profile.workspaceRoot != "", "old_model", stale.model, "new_model", profile.model)
		} else {
			updateSessionStateRuntime(state, msg, profile)
			gw.mu.Unlock()
			safeBotSetToolApprovalMode(state.ctrl, profile.toolApprovalMode)
			gw.logger.Info("bot session reused", "platform", msg.Platform, "chat_type", msg.ChatType, "chat", hashID(msg.ChatID), "session", key[:8])
			return state
		}
	} else {
		gw.mu.Unlock()
	}

	// Create the lease owner before the controller so automatic conflict
	// recovery can move ownership to the recovery branch before the controller
	// commits to writing it. Without this callback the bot kept guarding the
	// original path while continuing on an unleased recovery path.
	sessionSink := &sessionEventSink{}
	leases := control.NewSessionLeaseKeeper()
	state := &sessionState{
		sink:             sessionSink,
		leases:           leases,
		platform:         msg.Platform,
		connectionID:     strings.TrimSpace(msg.ConnectionID),
		model:            profile.model,
		workspaceRoot:    profile.workspaceRoot,
		toolApprovalMode: profile.toolApprovalMode,
		sessionPath:      profile.sessionPath,
		pendingAsks:      make(map[string][]event.AskQuestion),
		createdAt:        time.Now(),
		lastActive:       time.Now(),
	}
	gw.logger.Info("bot session creating", "platform", msg.Platform, "chat_type", msg.ChatType, "chat", hashID(msg.ChatID), "session", key[:8], "model", profile.model, "workspace_set", profile.workspaceRoot != "", "tool_approval_mode", profile.toolApprovalMode)
	ctrl, err := boot.Build(ctx, boot.Options{
		Model:              profile.model,
		MaxSteps:           gw.cfg.MaxSteps,
		MaxStepsKey:        "bot.max_steps",
		RequireKey:         true,
		Sink:               sessionSink,
		StatsSource:        surface.Bot,
		WorkspaceRoot:      profile.workspaceRoot,
		SessionDir:         botSessionDir(profile.workspaceRoot),
		ApprovalTimeout:    gw.approvalTimeout(),
		OnSessionRecovered: gw.botSessionRecoveredHandler(key, msg, state),
	})
	if err != nil {
		leases.Release()
		gw.logger.Error("build controller failed", "err", secrets.RedactError(err))
		return nil
	}
	state.ctrl = ctrl
	if profile.sessionPath != "" {
		// A mapped binding degrades to a fresh session on failure; only an
		// explicit /attach is allowed to hard-fail the message, because the
		// user named that exact session.
		degrade := func(reason string, err error) bool {
			if !profile.sessionPathOptional {
				return false
			}
			gw.logger.Warn("mapped bot session unavailable; starting fresh", "reason", reason, "session_path", profile.sessionPath, "err", err)
			profile.sessionPath = ""
			state.sessionPath = ""
			state.mappingDegraded = true
			return true
		}
		if err := leases.Rebind(profile.sessionPath); err != nil {
			if !degrade("lease held elsewhere", err) {
				ctrl.Close()
				leases.Release()
				gw.logger.Error("attached bot session is in use", "err", control.SessionInUseMessage(err))
				return nil
			}
		} else if loaded, err := agent.LoadSession(profile.sessionPath); err != nil {
			if !degrade("load failed", err) {
				ctrl.Close()
				leases.Release()
				if os.IsNotExist(err) {
					gw.logger.Error("attached bot session missing", "session_path", profile.sessionPath)
				} else {
					gw.logger.Error("attached bot session load failed", "session_path", profile.sessionPath, "err", err)
				}
				return nil
			}
		} else if err := ctrl.Resume(loaded, profile.sessionPath); err != nil {
			gw.logger.Error("attached session resume refused", "session_path", profile.sessionPath, "err", err)
		}
	}
	ctrl.EnableInteractiveApproval()
	ctrl.SetToolApprovalMode(profile.toolApprovalMode)
	ctrl.EnsureSessionPath()
	if err := rebindBotControllerWriteAuthority(leases, ctrl); err != nil {
		ctrl.Close()
		leases.Release()
		gw.logger.Error("bot session lease failed", "err", control.SessionInUseMessage(err))
		return nil
	}

	var replace *sessionState
	gw.mu.Lock()
	// Re-check under the lock: while we were off-lock in boot.Build, a second
	// message for the same key may have built and registered its own session.
	// Reuse it only when it still targets this message's runtime profile.
	if existing, ok := gw.controllers[key]; ok {
		if sessionStateMatchesRuntime(existing, profile) {
			updateSessionStateRuntime(existing, msg, profile)
			gw.mu.Unlock()
			ctrl.Close()
			leases.Release()
			safeBotSetToolApprovalMode(existing.ctrl, profile.toolApprovalMode)
			gw.logger.Info("bot session built concurrently; discarding duplicate", "platform", msg.Platform, "chat", hashID(msg.ChatID), "session", key[:8])
			return existing
		}
		delete(gw.controllers, key)
		replace = existing
	}
	gw.controllers[key] = state
	gw.mu.Unlock()
	gw.closeSessionState(replace)

	gw.logger.Info("bot session created", "platform", msg.Platform, "chat_type", msg.ChatType, "chat", hashID(msg.ChatID), "session", key[:8])
	return state
}

func updateSessionStateRuntime(state *sessionState, msg InboundMessage, profile sessionRuntimeProfile) {
	if state == nil {
		return
	}
	if state.connectionID == "" {
		state.connectionID = strings.TrimSpace(msg.ConnectionID)
	}
	if state.platform == "" {
		state.platform = msg.Platform
	}
	state.model = profile.model
	state.workspaceRoot = profile.workspaceRoot
	state.toolApprovalMode = profile.toolApprovalMode
	state.sessionPath = profile.sessionPath
	state.lastActive = time.Now()
}

func (gw *BotGateway) sessionProfileForMessage(msg InboundMessage) sessionRuntimeProfile {
	model, workspaceRoot, toolApprovalMode := gw.sessionOptionsForMessage(msg)
	var sessionPath string
	sessionPathOptional := false
	if override, ok := gw.sessionRuntimeOverrideForMessage(msg); ok {
		sessionPath = override.sessionPath
	}
	// A persisted session_mappings binding is the durable chat→session link
	// the desktop writes into the connection config. Without consuming it
	// here, every gateway restart or runtime rebuild opened a brand-new
	// session file for the chat and the configured binding was display-only
	// (#6917, #6934).
	if sessionPath == "" {
		if mapped := gw.sessionMappingPathForMessage(msg); mapped != "" {
			sessionPath = mapped
			sessionPathOptional = true
		}
	}
	return sessionRuntimeProfile{
		model:               strings.TrimSpace(model),
		workspaceRoot:       strings.TrimSpace(workspaceRoot),
		toolApprovalMode:    normalizeBotToolApprovalMode(toolApprovalMode),
		sessionPath:         canonicalBotPath(sessionPath),
		sessionPathOptional: sessionPathOptional,
	}
}

// sessionMappingPathForMessage resolves the persisted session_mappings entry
// for a message to an existing session file. Only bindings that resolve to a
// present, readable file participate — a moved or deleted target quietly
// degrades to normal session creation rather than blocking the chat.
func (gw *BotGateway) sessionMappingPathForMessage(msg InboundMessage) string {
	gw.mu.Lock()
	var mappings []SessionMapping
	if msg.ConnectionID != "" {
		if channel, ok := gw.cfg.ConnectionChannels[msg.ConnectionID]; ok {
			mappings = channel.SessionMappings
		}
	}
	if len(mappings) == 0 {
		if channel, ok := gw.cfg.Channels[msg.Platform]; ok {
			mappings = channel.SessionMappings
		}
	}
	gw.mu.Unlock()
	mapping, ok := matchingSessionMapping(mappings, msg)
	if !ok {
		return ""
	}
	path := botSessionPathFromTarget(mapping.SessionID)
	if path == "" {
		path = botSessionPathFromTarget(mapping.SessionSource)
	}
	if path == "" {
		return ""
	}
	if info, err := os.Stat(path); err != nil || info.IsDir() {
		return ""
	}
	return path
}

func sessionStateMatchesRuntime(state *sessionState, profile sessionRuntimeProfile) bool {
	if state == nil || state.ctrl == nil {
		return false
	}
	if stateModel := strings.TrimSpace(state.model); stateModel != "" && profile.model != "" && stateModel != profile.model {
		return false
	}
	stateRoot := strings.TrimSpace(state.workspaceRoot)
	wantRoot := strings.TrimSpace(profile.workspaceRoot)
	if stateRoot == "" {
		root, ok := safeBotControllerWorkspaceRoot(state.ctrl)
		if ok {
			stateRoot = strings.TrimSpace(root)
		} else if wantRoot != "" {
			return false
		}
	}
	if stateRoot != wantRoot {
		return false
	}
	// A state that already degraded off its mapped session keeps running on
	// its fresh path even though the profile re-resolves the mapping each
	// message; rebuilding here would spawn a new session per message while the
	// mapped file stays unavailable.
	if profile.sessionPathOptional && state.mappingDegraded {
		return true
	}
	if canonicalBotPath(state.sessionPath) != canonicalBotPath(profile.sessionPath) {
		return false
	}
	if profile.sessionPath != "" && canonicalBotPath(state.ctrl.SessionPath()) != canonicalBotPath(profile.sessionPath) {
		return false
	}
	return true
}

func safeBotControllerWorkspaceRoot(ctrl botController) (root string, ok bool) {
	if ctrl == nil {
		return "", false
	}
	defer func() {
		if recover() != nil {
			root = ""
			ok = false
		}
	}()
	return ctrl.WorkspaceRoot(), true
}

func safeBotSetToolApprovalMode(ctrl botController, mode string) {
	if ctrl == nil {
		return
	}
	defer func() {
		_ = recover()
	}()
	ctrl.SetToolApprovalMode(mode)
}

func botSessionDir(workspaceRoot string) string {
	if strings.TrimSpace(workspaceRoot) == "" {
		return config.SessionDir()
	}
	if dir := config.ProjectSessionDir(workspaceRoot); dir != "" {
		return dir
	}
	return config.SessionDir()
}

func (gw *BotGateway) rememberSessionReady(msg InboundMessage, ctrl botController) {
	if gw.cfg.OnSessionReady == nil || ctrl == nil {
		return
	}
	gw.rememberSessionPath(msg, ctrl.SessionPath())
}

func (gw *BotGateway) rememberSessionPath(msg InboundMessage, sessionPath string) {
	if gw.cfg.OnSessionReady == nil {
		return
	}
	sessionID := botSessionTarget(sessionPath)
	if sessionID == "" {
		return
	}
	if err := gw.cfg.OnSessionReady(msg, sessionID); err != nil {
		gw.logger.Warn("remember bot session failed", "platform", msg.Platform, "connection", msg.ConnectionID, "err", err)
	}
}

// botSessionRecoveredHandler keeps the controller path, its writer lease, and
// the remote-to-session mapping on the same recovery generation. The lease
// handoff runs first and is failure-atomic: if the recovery path is already
// owned, the controller stays on the original path and the old lease remains
// held. Mapping updates are limited to this exact sessionState so a late
// callback from a retired controller cannot overwrite its replacement.
func (gw *BotGateway) botSessionRecoveredHandler(key string, msg InboundMessage, state *sessionState) func(control.SessionRecoveryInfo) error {
	return func(info control.SessionRecoveryInfo) error {
		if state == nil || state.leases == nil {
			return nil
		}
		// Keep the lease handoff and mapping publication atomic with respect to
		// state retirement. In particular, never let a callback that outlives
		// Stop reacquire a lease after closeSessionState has released it.
		state.lifecycleMu.Lock()
		defer state.lifecycleMu.Unlock()
		if state.retired {
			return errBotSessionRetired
		}
		if err := state.leases.HandleSessionRecovered(info); err != nil {
			return err
		}

		originalPath := canonicalBotPath(info.OriginalPath)
		recoveryPath := canonicalBotPath(info.RecoveryPath)
		live := false
		gw.mu.Lock()
		if gw.controllers[key] == state {
			live = true
			if canonicalBotPath(state.sessionPath) == originalPath {
				state.sessionPath = recoveryPath
			}
			if override, ok := gw.sessionOverrides[key]; ok && canonicalBotPath(override.sessionPath) == originalPath {
				override.sessionPath = recoveryPath
				gw.sessionOverrides[key] = override
			}
		}
		gw.mu.Unlock()

		if live {
			gw.rememberSessionPath(msg, recoveryPath)
		}
		return nil
	}
}

func botSessionTarget(sessionPath string) string {
	sessionPath = strings.TrimSpace(sessionPath)
	if sessionPath == "" {
		return ""
	}
	return "path:" + sessionPath
}

func (gw *BotGateway) sessionOptionsForMessage(msg InboundMessage) (model string, workspaceRoot string, toolApprovalMode string) {
	// cfg.ToolApprovalMode / Channels / ConnectionChannels are rewritten under
	// gw.mu at runtime (/yolo, UpdateConnectionToolApprovalMode), so snapshot them
	// under a short lock and resolve outside it — applyRuntimeOverrideOptions
	// takes gw.mu itself. Copying the ChannelConfig value is enough: writers
	// replace whole map entries and never mutate SessionMappings in place.
	gw.mu.Lock()
	model = gw.cfg.Model
	workspaceRoot = gw.cfg.WorkspaceRoot
	toolApprovalMode = normalizeBotToolApprovalMode(gw.cfg.ToolApprovalMode)
	var connChannel ChannelConfig
	connOK := false
	if msg.ConnectionID != "" {
		connChannel, connOK = gw.cfg.ConnectionChannels[msg.ConnectionID]
	}
	platChannel, platOK := gw.cfg.Channels[msg.Platform]
	gw.mu.Unlock()

	var mappings []SessionMapping
	if connOK {
		applyBotChannelOptions(connChannel, &model, &workspaceRoot, &toolApprovalMode)
		mappings = connChannel.SessionMappings
		if mapping, ok := matchingSessionMapping(mappings, msg); ok {
			workspaceRoot = workspaceRootForSessionMapping(mapping, workspaceRoot)
		}
		model, workspaceRoot, toolApprovalMode = gw.applyRouteOptions(msg, model, workspaceRoot, toolApprovalMode)
		model, workspaceRoot, toolApprovalMode = gw.applyRuntimeOverrideOptions(msg, model, workspaceRoot, toolApprovalMode)
		return model, workspaceRoot, toolApprovalMode
	}
	if platOK {
		applyBotChannelOptions(platChannel, &model, &workspaceRoot, &toolApprovalMode)
		mappings = platChannel.SessionMappings
	}
	if mapping, ok := matchingSessionMapping(mappings, msg); ok {
		workspaceRoot = workspaceRootForSessionMapping(mapping, workspaceRoot)
	}
	model, workspaceRoot, toolApprovalMode = gw.applyRouteOptions(msg, model, workspaceRoot, toolApprovalMode)
	model, workspaceRoot, toolApprovalMode = gw.applyRuntimeOverrideOptions(msg, model, workspaceRoot, toolApprovalMode)
	return model, workspaceRoot, toolApprovalMode
}

func (gw *BotGateway) applyRuntimeOverrideOptions(msg InboundMessage, model, workspaceRoot, toolApprovalMode string) (string, string, string) {
	if override, ok := gw.sessionRuntimeOverrideForMessage(msg); ok {
		applyBotChannelOptions(override.channel, &model, &workspaceRoot, &toolApprovalMode)
	}
	return model, workspaceRoot, toolApprovalMode
}

func (gw *BotGateway) applyRouteOptions(msg InboundMessage, model, workspaceRoot, toolApprovalMode string) (string, string, string) {
	for _, route := range gw.cfg.Routes {
		if routeMatchesMessage(route, msg) {
			applyBotChannelOptions(route.Channel, &model, &workspaceRoot, &toolApprovalMode)
			break
		}
	}
	return model, workspaceRoot, toolApprovalMode
}

func applyBotChannelOptions(channel ChannelConfig, model *string, workspaceRoot *string, toolApprovalMode *string) {
	if value := strings.TrimSpace(channel.Model); value != "" {
		*model = value
	}
	if value := strings.TrimSpace(channel.WorkspaceRoot); value != "" {
		*workspaceRoot = value
	}
	if value := normalizeOptionalBotToolApprovalMode(channel.ToolApprovalMode); value != "" {
		*toolApprovalMode = value
	}
}

func matchingSessionMapping(mappings []SessionMapping, msg InboundMessage) (SessionMapping, bool) {
	for i := range mappings {
		if sessionMappingMatches(mappings[i], msg) {
			return mappings[i], true
		}
	}
	return SessionMapping{}, false
}

func sessionMappingMatches(mapping SessionMapping, msg InboundMessage) bool {
	if strings.TrimSpace(mapping.RemoteID) != strings.TrimSpace(msg.ChatID) {
		return false
	}
	chatType, userID, threadID := sessionMappingIdentity(msg)
	mappingChatType := strings.TrimSpace(mapping.ChatType)
	if mappingChatType == "" {
		return chatType == ""
	}
	if mappingChatType != chatType {
		return false
	}
	if strings.TrimSpace(mapping.UserID) != userID {
		return false
	}
	return strings.TrimSpace(mapping.ThreadID) == threadID
}

func sessionMappingIdentity(msg InboundMessage) (chatType string, userID string, threadID string) {
	switch msg.ChatType {
	case ChatGroup, ChatGuild:
		chatType = string(msg.ChatType)
		userID = strings.TrimSpace(msg.UserID)
	case ChatThread:
		chatType = string(msg.ChatType)
		threadID = strings.TrimSpace(msg.ThreadID)
		if threadID == "" {
			threadID = strings.TrimSpace(msg.ChatID)
		}
	}
	return chatType, userID, threadID
}

func workspaceRootForSessionMapping(mapping SessionMapping, fallback string) string {
	if root := strings.TrimSpace(mapping.WorkspaceRoot); root != "" {
		return root
	}
	if strings.EqualFold(strings.TrimSpace(mapping.Scope), "global") {
		return ""
	}
	return fallback
}

func routeMatchesMessage(route RouteConfig, msg InboundMessage) bool {
	if value := strings.TrimSpace(route.ConnectionID); value != "" && value != strings.TrimSpace(msg.ConnectionID) {
		return false
	}
	if route.Platform != "" && route.Platform != msg.Platform {
		return false
	}
	if route.ChatType != "" && route.ChatType != msg.ChatType {
		return false
	}
	if value := strings.TrimSpace(route.ChatID); value != "" && value != strings.TrimSpace(msg.ChatID) {
		return false
	}
	if value := strings.TrimSpace(route.UserID); value != "" && value != strings.TrimSpace(msg.UserID) {
		return false
	}
	if value := strings.TrimSpace(route.ThreadID); value != "" && value != strings.TrimSpace(msg.ThreadID) {
		return false
	}
	return true
}
