package bot

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"reasonix/internal/control"
	"reasonix/internal/event"
	"reasonix/internal/sessioninbox"
)

// GatewayConfig 是 BotGateway 的配置。
type GatewayConfig struct {
	Model             string
	ToolApprovalMode  string
	MaxSteps          int
	QueueMode         string
	QueueCap          int
	QueueDrop         string
	PairingEnabled    bool
	PairingTTL        time.Duration
	PairingMaxPending int
	// IgnoreSelfMessages drops messages that are clearly sent by this bot. It
	// uses configured SelfUserIDs plus recently returned outbound message IDs.
	IgnoreSelfMessages bool
	SelfUserIDs        map[Platform][]string
	ControlEnabled     bool
	ControlAddr        string
	ControlToken       string
	// ApprovalTimeout bounds how long a tool-approval/ask prompt blocks a bot
	// session waiting for a remote user's reply. Zero falls back to
	// defaultBotApprovalTimeout so an abandoned prompt can't wedge the bot forever
	// (#4626, #4402). A negative value disables the timeout (wait indefinitely).
	ApprovalTimeout    time.Duration
	WorkspaceRoot      string
	Channels           map[Platform]ChannelConfig
	ConnectionChannels map[string]ChannelConfig
	Routes             []RouteConfig
	ConnectionAccess   map[string]AccessConfig
	Allowlist          AllowlistConfig
	Enabled            map[Platform]bool
	Debounce           time.Duration
	// OnInbound observes every allowlisted inbound message before dispatch.
	//
	// Reentrancy contract for all GatewayConfig callbacks (OnInbound,
	// OnSessionReady, OnToolApprovalModeChange): they run synchronously on
	// gateway-owned dispatch/turn goroutines; OnSessionReady can also run on a
	// controller recovery/autosave goroutine. Stop drains all of those paths
	// before returning. A callback must therefore never call Stop, nor block
	// until a goroutine that does so completes — Stop would wait on the very
	// goroutine running the callback, a guaranteed deadlock. Hosts that want to
	// shut the gateway down in reaction to a callback must trigger the shutdown
	// asynchronously.
	OnInbound func(InboundMessage)
	// OnSessionReady notifies the host after the bot has created, reused, or
	// recovered the controller for an inbound remote. Hosts may persist the
	// concrete session ID or keep the remote as a read-only channel.
	OnSessionReady func(InboundMessage, string) error
	// OnToolApprovalModeChange persists a remote IM request such as /yolo on.
	// The gateway updates the live session and in-memory defaults first; this
	// callback lets desktop save the chosen connection mode to user config.
	OnToolApprovalModeChange func(InboundMessage, string) error
	// Desktop, when the gateway is embedded in the desktop app, gives bot
	// chats a god view over desktop sessions (/desktop commands): global
	// status, event subscriptions, and remote approvals for any live desktop
	// session. Nil when the gateway runs standalone (reasonix bot start).
	Desktop DesktopBridge
}

// ChannelConfig overrides gateway defaults for one IM channel.
type ChannelConfig struct {
	Model            string
	ToolApprovalMode string
	WorkspaceRoot    string
	SessionMappings  []SessionMapping
}

// SessionMapping is the runtime subset of a saved bot connection mapping used
// to route a remote chat/user/thread back to its intended workspace.
type SessionMapping struct {
	RemoteID      string
	SessionID     string
	SessionSource string
	ChatType      string
	UserID        string
	ThreadID      string
	Scope         string
	WorkspaceRoot string
	UpdatedAt     string
}

// RouteConfig applies per-remote overrides. Empty match fields are wildcards;
// the first matching route wins.
type RouteConfig struct {
	ConnectionID string
	Platform     Platform
	ChatType     ChatType
	ChatID       string
	UserID       string
	ThreadID     string
	Channel      ChannelConfig
}

// AdapterBinding attaches an adapter instance to one saved bot connection.
// Feishu and Lark share PlatformFeishu, so ID/Domain keep their sessions,
// replies, and per-connection settings separated at runtime.
type AdapterBinding struct {
	ID       string
	Domain   string
	Platform Platform
	Adapter  Adapter
}

// AllowlistConfig 控制哪些用户/群可以使用 bot。
type AllowlistConfig struct {
	Enabled   bool
	AllowAll  bool
	Users     map[Platform][]string
	Approvers map[Platform][]string
	Admins    map[Platform][]string
	Groups    map[Platform][]string
}

// AccessConfig controls who may use one concrete bot connection.
type AccessConfig struct {
	Enabled        bool
	AllowAll       bool
	PairingEnabled bool
	Users          []string
	Groups         []string
	Approvers      []string
	Admins         []string
}

// AdapterHealthSnapshot describes the gateway's current view of one adapter.
type AdapterHealthSnapshot struct {
	ID            string    `json:"id"`
	Platform      Platform  `json:"platform"`
	Domain        string    `json:"domain,omitempty"`
	Name          string    `json:"name,omitempty"`
	Status        string    `json:"status"`
	StartedAt     time.Time `json:"started_at,omitempty"`
	LastMessageAt time.Time `json:"last_message_at,omitempty"`
	LastSendAt    time.Time `json:"last_send_at,omitempty"`
	LastErrorAt   time.Time `json:"last_error_at,omitempty"`
	LastError     string    `json:"last_error,omitempty"`
	Messages      int64     `json:"messages"`
	Sends         int64     `json:"sends"`
	SendErrors    int64     `json:"send_errors"`
	Closed        bool      `json:"closed"`
}

// BotGateway 是 reasonix bot 消息网关，管理 Controller 生命周期、session 并发、
// 事件渲染和平台适配器。
type BotGateway struct {
	cfg      GatewayConfig
	adapters []AdapterBinding
	sessions *SessionManager
	startErr []error

	lifecycleMu sync.Mutex
	started     bool
	stopped     bool
	runCancel   context.CancelFunc
	startDone   chan struct{}
	stopDone    chan struct{}
	gatewayWG   sync.WaitGroup
	turnWG      sync.WaitGroup

	mu                      sync.Mutex
	controllers             map[string]*sessionState // session key -> active state
	pendingReactionCleanups map[string][]func()
	allowlist               map[Platform]map[string]bool
	groupAllowlist          map[Platform]map[string]bool
	selfUserIDs             map[Platform]map[string]bool
	outboundMessageIDs      map[string]time.Time
	adapterHealth           map[string]*AdapterHealthSnapshot
	controlServer           *controlHTTPServer
	sessionOverrides        map[string]sessionRuntimeOverride

	logger *slog.Logger
}

// botController is the slice of the controller's driving port the gateway needs:
// session lifecycle, turn execution, and approval/ask handling. The bot never
// touches goals, checkpoints, or memory, so it depends on those sub-ports only —
// not the concrete *control.Controller and its ~99 methods.
type botController interface {
	control.Lifecycle
	control.TurnControl
	control.Approvals
}

type sessionState struct {
	lifecycleMu      sync.Mutex
	retired          bool
	ctrl             botController
	sink             *sessionEventSink
	leases           *control.SessionLeaseKeeper
	platform         Platform
	connectionID     string
	model            string
	workspaceRoot    string
	toolApprovalMode string
	sessionPath      string
	// mappingDegraded records that this state intentionally runs on a fresh
	// session because its session_mappings target could not be used at build
	// time. It keeps later messages (whose profile re-resolves the mapping)
	// from tearing the state down every turn; convergence back onto the
	// mapped file happens on the next gateway restart.
	mappingDegraded  bool
	cancel           context.CancelFunc
	pendingAsks      map[string][]event.AskQuestion
	pendingApprovals map[string]event.Approval
	lastApprovalID   string
	lastAskID        string
	createdAt        time.Time
	lastActive       time.Time
}

var errBotSessionRetired = errors.New("bot session retired during recovery")

type sessionRuntimeProfile struct {
	model            string
	workspaceRoot    string
	toolApprovalMode string
	sessionPath      string
	// sessionPathOptional marks sessionPath as a persisted session_mappings
	// binding rather than an explicit /attach: when the mapped file cannot be
	// loaded or leased, the session degrades to a fresh path instead of
	// dropping the message (#6917).
	sessionPathOptional bool
}

type sessionRuntimeOverride struct {
	channel     ChannelConfig
	sessionPath string
	label       string
}

type sessionEventSink struct {
	mu     sync.RWMutex
	target event.Sink
}

type pendingReactionAdapter interface {
	AddPendingReaction(ctx context.Context, messageID string) (func(), error)
}

const outboundEchoTTL = 10 * time.Minute

func (s *sessionEventSink) setTarget(target event.Sink) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.target = target
}

func (s *sessionEventSink) Emit(e event.Event) {
	s.mu.RLock()
	target := s.target
	s.mu.RUnlock()
	if target != nil {
		target.Emit(e)
	}
}

// NewGateway 创建一个新的 BotGateway。
func NewGateway(cfg GatewayConfig, adapters map[Platform]Adapter, logger *slog.Logger) *BotGateway {
	bindings := make([]AdapterBinding, 0, len(adapters))
	for plat, adapter := range adapters {
		bindings = append(bindings, AdapterBinding{ID: string(plat), Platform: plat, Adapter: adapter})
	}
	return NewGatewayWithAdapterBindings(cfg, bindings, logger)
}

// NewGatewayWithAdapterBindings creates a gateway with one or more adapter
// instances per platform.
func NewGatewayWithAdapterBindings(cfg GatewayConfig, adapters []AdapterBinding, logger *slog.Logger) *BotGateway {
	if logger == nil {
		logger = slog.Default()
	}
	if cfg.Debounce <= 0 {
		cfg.Debounce = 1500 * time.Millisecond
	}
	cfg.QueueMode = NormalizeQueueMode(cfg.QueueMode)
	if cfg.QueueCap <= 0 {
		cfg.QueueCap = DefaultQueueCap
	}
	cfg.QueueDrop = NormalizeQueueDrop(cfg.QueueDrop)
	if cfg.PairingTTL <= 0 {
		cfg.PairingTTL = defaultPairingTTL
	}
	if cfg.PairingMaxPending <= 0 {
		cfg.PairingMaxPending = defaultPairingMaxPending
	}
	gw := &BotGateway{
		cfg:                     cfg,
		adapters:                normalizeAdapterBindings(adapters),
		sessions:                NewSessionManager(cfg.Debounce),
		controllers:             make(map[string]*sessionState),
		pendingReactionCleanups: make(map[string][]func()),
		allowlist:               make(map[Platform]map[string]bool),
		groupAllowlist:          make(map[Platform]map[string]bool),
		selfUserIDs:             make(map[Platform]map[string]bool),
		outboundMessageIDs:      make(map[string]time.Time),
		adapterHealth:           make(map[string]*AdapterHealthSnapshot),
		sessionOverrides:        make(map[string]sessionRuntimeOverride),
		logger:                  logger.With("component", "bot_gateway"),
	}
	gw.buildAllowlist()
	gw.buildSelfUserIDs()
	for _, binding := range gw.adapters {
		gw.setAdapterConfigured(binding)
	}
	return gw
}

func normalizeAdapterBindings(adapters []AdapterBinding) []AdapterBinding {
	out := make([]AdapterBinding, 0, len(adapters))
	for _, binding := range adapters {
		if binding.Adapter == nil {
			continue
		}
		if binding.Platform == "" {
			binding.Platform = binding.Adapter.Platform()
		}
		if strings.TrimSpace(binding.ID) == "" {
			binding.ID = string(binding.Platform)
		}
		binding.ID = strings.TrimSpace(binding.ID)
		binding.Domain = strings.TrimSpace(binding.Domain)
		out = append(out, binding)
	}
	return out
}

func (gw *BotGateway) buildAllowlist() {
	for _, plat := range []Platform{PlatformQQ, PlatformFeishu, PlatformWeixin} {
		gw.allowlist[plat] = make(map[string]bool)
		if !gw.cfg.Allowlist.Enabled {
			continue
		}
		addAllowlistUsers(gw.allowlist[plat], gw.cfg.Allowlist.Users[plat])
		addAllowlistUsers(gw.allowlist[plat], gw.cfg.Allowlist.Admins[plat])
		addAllowlistUsers(gw.allowlist[plat], gw.cfg.Allowlist.Approvers[plat])
		gw.groupAllowlist[plat] = make(map[string]bool)
		for _, gid := range gw.cfg.Allowlist.Groups[plat] {
			gw.groupAllowlist[plat][gid] = true
		}
	}
}

func addAllowlistUsers(dst map[string]bool, users []string) {
	for _, uid := range users {
		uid = strings.TrimSpace(uid)
		if uid != "" {
			dst[uid] = true
		}
	}
}

func (gw *BotGateway) buildSelfUserIDs() {
	for _, plat := range []Platform{PlatformQQ, PlatformFeishu, PlatformWeixin} {
		gw.selfUserIDs[plat] = stringSet(gw.cfg.SelfUserIDs[plat])
	}
}

// Start 启动所有已启用的平台适配器并开始处理消息。
func (gw *BotGateway) Start(ctx context.Context) (err error) {
	gw.lifecycleMu.Lock()
	if gw.stopped {
		gw.lifecycleMu.Unlock()
		return errors.New("bot gateway already stopped")
	}
	if gw.started {
		gw.lifecycleMu.Unlock()
		return errors.New("bot gateway already started")
	}
	gw.started = true
	runCtx, cancel := context.WithCancel(ctx)
	gw.runCancel = cancel
	startDone := make(chan struct{})
	gw.startDone = startDone
	gw.lifecycleMu.Unlock()
	defer func() {
		if err != nil {
			cancel()
		}
		gw.lifecycleMu.Lock()
		if err != nil {
			gw.runCancel = nil
		}
		close(startDone)
		gw.lifecycleMu.Unlock()
	}()

	started := make([]AdapterBinding, 0, len(gw.adapters))
	var startErr []error
	for _, binding := range gw.adapters {
		if !gw.cfg.Enabled[binding.Platform] {
			gw.logger.Info("platform disabled, skipping", "platform", binding.Platform, "connection", binding.ID)
			gw.markAdapterDisabled(binding)
			continue
		}
		gw.logger.Info("starting adapter", "platform", binding.Platform, "connection", binding.ID, "domain", binding.Domain)
		if err := binding.Adapter.Start(runCtx); err != nil {
			wrapped := fmt.Errorf("start adapter %s: %w", binding.ID, err)
			startErr = append(startErr, wrapped)
			gw.markAdapterStartFailed(binding, err)
			gw.logger.Warn("adapter start failed", "platform", binding.Platform, "connection", binding.ID, "domain", binding.Domain, "err", err)
			continue
		}
		gw.markAdapterStarted(binding)
		started = append(started, binding)
	}
	// SendToAdapter reads gw.adapters under gw.mu; publish the started set under
	// the same lock.
	gw.mu.Lock()
	gw.adapters = started
	gw.startErr = startErr
	gw.mu.Unlock()
	if len(started) == 0 && len(startErr) > 0 {
		return errors.Join(startErr...)
	}
	if err := gw.startControlServer(runCtx); err != nil {
		for _, binding := range started {
			_ = binding.Adapter.Stop()
		}
		return err
	}

	// 合并所有适配器的消息通道
	for _, binding := range gw.adapters {
		gw.gatewayWG.Go(func() {
			gw.dispatchLoop(runCtx, binding)
		})
	}

	return nil
}

// Stop 停止所有适配器并关闭所有 session。它会等待 dispatch 与 turn goroutine
// 全部退出，所以绝不能在 GatewayConfig 回调里同步调用（见 OnInbound 的
// reentrancy contract），否则 Stop 会等待正在运行该回调的 goroutine 自己。
func (gw *BotGateway) Stop() {
	gw.lifecycleMu.Lock()
	if gw.stopped {
		stopDone := gw.stopDone
		gw.lifecycleMu.Unlock()
		if stopDone != nil {
			<-stopDone
		}
		return
	}
	gw.stopped = true
	stopDone := make(chan struct{})
	gw.stopDone = stopDone
	cancel := gw.runCancel
	gw.runCancel = nil
	startDone := gw.startDone
	gw.lifecycleMu.Unlock()
	defer close(stopDone)

	if cancel != nil {
		cancel()
	}
	if startDone != nil {
		<-startDone
	}

	// Cancel sessions that already exist before waiting for dispatch to drain.
	// A dispatch already inside handleMessage may still publish a late session,
	// so closeSessions is repeated after gatewayWG and turnWG reach zero.
	gw.closeSessions()
	for _, binding := range gw.adapters {
		if err := binding.Adapter.Stop(); err != nil {
			gw.logger.Warn("error stopping adapter", "platform", binding.Platform, "connection", binding.ID, "err", err)
		}
		gw.markAdapterClosed(binding)
	}
	gw.stopControlServer()
	gw.gatewayWG.Wait()
	gw.closeSessions()
	gw.turnWG.Wait()
	gw.closeSessions()
}

func (gw *BotGateway) dispatchLoop(ctx context.Context, binding AdapterBinding) {
	for {
		select {
		case <-ctx.Done():
			gw.markAdapterClosed(binding)
			return
		case msg, ok := <-binding.Adapter.Messages():
			if !ok {
				gw.markAdapterClosed(binding)
				return
			}
			gw.markAdapterMessage(binding)
			gw.handleMessage(ctx, binding, msg)
		}
	}
}

func (gw *BotGateway) handleMessage(ctx context.Context, binding AdapterBinding, msg InboundMessage) {
	msg.Platform = binding.Platform
	if msg.ConnectionID == "" {
		msg.ConnectionID = binding.ID
	}
	if msg.Domain == "" {
		msg.Domain = binding.Domain
	}
	if gw.isSelfMessage(msg) {
		gw.logger.Debug("bot ignored self message", "platform", binding.Platform, "connection", msg.ConnectionID, "chat", hashID(msg.ChatID), "message", hashID(msg.MessageID), "user", hashID(msg.UserID))
		return
	}
	src := msg.Session()
	key := BuildSessionKey(src)
	logFields := []any{
		"platform", binding.Platform,
		"connection", msg.ConnectionID,
		"domain", msg.Domain,
		"chat_type", msg.ChatType,
		"chat", hashID(msg.ChatID),
		"user", hashID(msg.UserID),
		"operator", hashID(msg.OperatorID),
		"thread", hashID(msg.ThreadID),
		"message", hashID(msg.MessageID),
		"text_chars", len([]rune(msg.Text)),
		"session", key[:8],
	}
	gw.logger.Info("bot inbound message", logFields...)

	// allowlist 检查
	if !gw.checkAllowlist(binding.Platform, msg) {
		gw.logger.Info("user not in allowlist", "platform", binding.Platform, "connection", msg.ConnectionID, "user", hashID(msg.UserID))
		if gw.offerPairing(ctx, binding.Adapter, msg) {
			return
		}
		_ = gw.sendText(ctx, binding.Adapter, msg, "抱歉，您没有使用此 bot 的权限。")
		return
	}
	if gw.cfg.OnInbound != nil {
		gw.cfg.OnInbound(msg)
	}

	if normalized, ok := gw.normalizeApprovalShortcut(key, msg.Text); ok {
		msg.Text = normalized
	} else if normalized, ok := gw.normalizeAskShortcut(key, msg.Text); ok {
		msg.Text = normalized
	} else if _, ok := decisionShortcutCommand(msg.Text); ok && gw.sessions.IsActive(key) {
		_ = gw.sendText(ctx, binding.Adapter, msg, "没有找到可匹配的待处理操作。请重新触发一次操作后回复编号，或按消息中的 ID 使用 /approve、/deny 或 /answer。")
		return
	}

	// 斜杠命令处理
	if IsSlashBypass(msg.Text) {
		gw.logger.Info("bot slash command", logFields...)
		gw.handleSlashCommand(ctx, binding.Adapter, key, msg)
		return
	}

	// 已接管桌面会话的聊天：普通消息直接驱动那个桌面会话，不进 bot 自己的
	// 会话机器（斜杠命令仍走上面的分支，/desktop release 永远可达）。
	if gw.divertToDesktopTakeover(ctx, binding.Adapter, msg) {
		gw.logger.Info("bot message diverted to desktop takeover", logFields...)
		return
	}

	cleanup := gw.addPendingReaction(ctx, binding.Platform, binding.Adapter, msg)

	queueMode := gw.queueMode(key, msg)
	warnDeprecatedQueueDrop(gw.cfg.QueueDrop)
	if gw.sessions.IsActive(key) {
		// Busy session: durable inbox is the authority (not SessionManager.pending).
		if IsSlashBypass(msg.Text) {
			// Slash commands still acquire through the session lock below.
		} else {
			switch queueMode {
			case QueueModeSteer:
				if rec, ok := gw.steerActiveSessionDurable(ctx, binding.Adapter, key, msg); ok {
					gw.logger.Info("bot message steered into active turn", "session", key[:8], "item", rec.ItemID)
					if cleanup != nil {
						cleanup()
					}
					_ = gw.sendText(ctx, binding.Adapter, msg, formatQueuedReceipt(rec)+"（已并入当前任务）")
					return
				}
			case QueueModeInterrupt:
				gw.cancelActiveSession(key)
				runReactionCleanups(gw.takeReactionCleanups(key))
				rec, err := gw.interruptActiveSessionDurable(ctx, binding.Adapter, key, msg)
				gw.storeReactionCleanup(key, cleanup)
				if err != nil {
					gw.logger.Warn("bot interrupt enqueue failed", "session", key[:8], "err", err)
					_ = gw.sendText(ctx, binding.Adapter, msg, "排队失败："+err.Error())
					return
				}
				gw.logger.Info("bot active turn interrupted; newest message durable-queued", "session", key[:8], "item", rec.ItemID)
				_ = gw.sendText(ctx, binding.Adapter, msg, "已停止当前任务。"+formatQueuedReceipt(rec))
				return
			case QueueModeCollect:
				if rec, err := gw.collectActiveSessionDurable(ctx, binding.Adapter, key, msg); err == nil {
					gw.storeReactionCleanup(key, cleanup)
					_ = gw.sendText(ctx, binding.Adapter, msg, formatQueuedReceipt(rec))
					return
				} else if errors.Is(err, sessioninbox.ErrCapacityItems) || errors.Is(err, sessioninbox.ErrCapacityBytes) || errors.Is(err, sessioninbox.ErrItemTooLarge) {
					if cleanup != nil {
						cleanup()
					}
					_ = gw.sendText(ctx, binding.Adapter, msg, "当前会话排队已满，请稍后再发，或使用 /queue pause 后清理。")
					return
				}
			default: // followup
				if rec, err := gw.followupActiveSessionDurable(ctx, binding.Adapter, key, msg); err == nil {
					gw.storeReactionCleanup(key, cleanup)
					_ = gw.sendText(ctx, binding.Adapter, msg, formatQueuedReceipt(rec))
					return
				} else if errors.Is(err, sessioninbox.ErrCapacityItems) || errors.Is(err, sessioninbox.ErrCapacityBytes) || errors.Is(err, sessioninbox.ErrItemTooLarge) {
					if cleanup != nil {
						cleanup()
					}
					_ = gw.sendText(ctx, binding.Adapter, msg, "当前会话排队已满，请稍后再发。")
					return
				}
			}
		}
	}

	// session 并发控制 — only the active-turn lock remains here; bodies live in inbox.
	result := gw.sessions.TryAcquireWithQueue(key, msg, QueueOptions{
		Mode: QueueModeFollowup, // never drop_old; capacity enforced by inbox
		Cap:  sessioninbox.DefaultMaxItems,
		Drop: QueueDropNew,
	})
	if result.Rejected {
		gw.logger.Warn("bot queue rejected message", "session", key[:8], "pending", result.Pending, "mode", result.Mode)
		if cleanup != nil {
			cleanup()
		}
		_ = gw.sendText(ctx, binding.Adapter, msg, "当前会话排队已满，请稍后再发，或使用 /queue 管理队列。")
		return
	}
	gw.dispatchQueueResult(ctx, binding.Adapter, key, msg, cleanup, result)
}

func (gw *BotGateway) queueMode(key string, msg InboundMessage) string {
	return gw.sessions.QueueMode(key, gw.cfg.QueueMode)
}

func (gw *BotGateway) sessionAPI(key string) control.GatewayAPI {
	gw.mu.Lock()
	state, ok := gw.controllers[key]
	gw.mu.Unlock()
	if !ok || state == nil || state.ctrl == nil {
		return nil
	}
	if api, ok := state.ctrl.(control.GatewayAPI); ok {
		return api
	}
	return nil
}

func (gw *BotGateway) steerActiveSessionDurable(ctx context.Context, adapter Adapter, key string, msg InboundMessage) (sessioninbox.InboxReceipt, bool) {
	text := strings.TrimSpace(msg.Text)
	if text == "" && len(msg.MediaURLs) == 0 && len(msg.Media) == 0 {
		return sessioninbox.InboxReceipt{}, false
	}
	gw.mu.Lock()
	state, ok := gw.controllers[key]
	gw.mu.Unlock()
	if !ok || state.ctrl == nil {
		return sessioninbox.InboxReceipt{}, false
	}
	msg = gw.prepareDurableInboxMessage(ctx, adapter, msg, state)
	text = msg.Text
	if strings.TrimSpace(text) == "" {
		return sessioninbox.InboxReceipt{}, false
	}
	msg.Text = text
	api, ok := state.ctrl.(control.GatewayAPI)
	if !ok {
		// Legacy fallback.
		if steerer, ok := state.ctrl.(interface{ TrySteer(string) bool }); ok && steerer.TrySteer(text) {
			return sessioninbox.InboxReceipt{Disposition: sessioninbox.DispositionSteerAccepted}, true
		}
		return sessioninbox.InboxReceipt{}, false
	}
	rec, err := enqueueViaInbox(api, msg, sessioninbox.IntentSteer)
	if err != nil {
		return sessioninbox.InboxReceipt{}, false
	}
	return rec, true
}

func (gw *BotGateway) cancelActiveSession(key string) {
	// state.cancel is rewritten under gw.mu on every turn (runTurn), so copy it
	// inside the lock and invoke it outside.
	var cancel context.CancelFunc
	gw.mu.Lock()
	state, ok := gw.controllers[key]
	if ok && state != nil {
		cancel = state.cancel
	}
	gw.mu.Unlock()
	if !ok || state == nil {
		return
	}
	if cancel != nil {
		cancel()
		return
	}
	if state.ctrl != nil {
		state.ctrl.Cancel()
	}
}

func (gw *BotGateway) connectionAccess(msg InboundMessage) (AccessConfig, bool) {
	if gw.cfg.ConnectionAccess == nil {
		return AccessConfig{}, false
	}
	id := strings.TrimSpace(msg.ConnectionID)
	if id == "" {
		return AccessConfig{}, false
	}
	access, ok := gw.cfg.ConnectionAccess[id]
	if !ok {
		return AccessConfig{}, false
	}
	if !accessConfigActive(access) {
		return AccessConfig{}, false
	}
	return access, true
}

func accessConfigActive(access AccessConfig) bool {
	return access.Enabled ||
		access.AllowAll ||
		access.PairingEnabled ||
		len(access.Users) > 0 ||
		len(access.Groups) > 0 ||
		len(access.Approvers) > 0 ||
		len(access.Admins) > 0
}

func (gw *BotGateway) checkAllowlist(plat Platform, msg InboundMessage) bool {
	if access, ok := gw.connectionAccess(msg); ok {
		return checkConnectionAllowlist(access, msg)
	}
	if gw.cfg.Allowlist.AllowAll {
		return true
	}
	if !gw.cfg.Allowlist.Enabled {
		return false
	}
	actor := msg.UserID
	if msg.OperatorID != "" {
		actor = msg.OperatorID
	}
	if !gw.allowlist[plat][actor] {
		return false
	}
	groups := gw.groupAllowlist[plat]
	if chatUsesGroupAllowlist(msg.ChatType) && len(groups) > 0 && !groups[msg.ChatID] {
		return false
	}
	return true
}

func checkConnectionAllowlist(access AccessConfig, msg InboundMessage) bool {
	if access.AllowAll {
		return true
	}
	if !access.Enabled {
		return false
	}
	actor := msg.UserID
	if msg.OperatorID != "" {
		actor = msg.OperatorID
	}
	users := stringSet(append(append(append([]string{}, access.Users...), access.Admins...), access.Approvers...))
	groups := stringSet(access.Groups)
	actorAllowed := users[actor]
	groupAllowed := chatUsesGroupAllowlist(msg.ChatType) && groups[msg.ChatID]
	if len(users) == 0 && len(groups) == 0 {
		return false
	}
	return actorAllowed || groupAllowed
}

func (gw *BotGateway) requireCommandRole(ctx context.Context, adapter Adapter, msg InboundMessage, role string) bool {
	if gw.checkCommandRole(msg.Platform, msg, role) {
		return true
	}
	_ = gw.sendText(ctx, adapter, msg, "抱歉，你没有执行此 bot 命令的权限。")
	return false
}

func (gw *BotGateway) checkCommandRole(plat Platform, msg InboundMessage, role string) bool {
	actor := msg.UserID
	if msg.OperatorID != "" {
		actor = msg.OperatorID
	}
	if strings.TrimSpace(actor) == "" {
		return false
	}
	if access, ok := gw.connectionAccess(msg); ok {
		admins := stringSet(access.Admins)
		approvers := stringSet(access.Approvers)
		if len(admins) == 0 && len(approvers) == 0 {
			return true
		}
		if admins[actor] {
			return true
		}
		return role == "approver" && approvers[actor]
	}
	admins := stringSet(gw.cfg.Allowlist.Admins[plat])
	approvers := stringSet(gw.cfg.Allowlist.Approvers[plat])
	if len(admins) == 0 && len(approvers) == 0 {
		return true
	}
	if admins[actor] {
		return true
	}
	if role == "approver" && approvers[actor] {
		return true
	}
	return false
}

func stringSet(values []string) map[string]bool {
	out := make(map[string]bool, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			out[value] = true
		}
	}
	return out
}

func (gw *BotGateway) offerPairing(ctx context.Context, adapter Adapter, msg InboundMessage) bool {
	if access, ok := gw.connectionAccess(msg); ok {
		if !access.PairingEnabled {
			return false
		}
	} else if !gw.cfg.PairingEnabled {
		return false
	}
	req, created, err := CreateOrRefreshPairingRequest(msg, PairingConfig{
		Enabled:               true,
		RequestTTL:            gw.cfg.PairingTTL,
		MaxPendingPerPlatform: gw.cfg.PairingMaxPending,
	})
	if err != nil {
		gw.logger.Warn("bot pairing request failed", "platform", msg.Platform, "chat_type", msg.ChatType, "err", err)
		return false
	}
	prefix := "需要先完成配对。"
	if !created {
		prefix = "你已有待批准的配对请求。"
	}
	text := fmt.Sprintf("%s\n配对码: %s\n请在本机运行: reasonix bot pairing approve %s\n此码将在 %s 过期。",
		prefix, req.Code, req.Code, req.ExpiresAt.Local().Format("2006-01-02 15:04"))
	_ = gw.sendText(ctx, adapter, msg, text)
	return true
}

func chatUsesGroupAllowlist(chatType ChatType) bool {
	switch chatType {
	case ChatGroup, ChatGuild, ChatThread:
		return true
	default:
		return false
	}
}

func (gw *BotGateway) setToolApprovalModeForMessage(key string, msg InboundMessage, mode string) error {
	mode = normalizeBotToolApprovalMode(mode)
	var ctrl botController

	gw.mu.Lock()
	if state, ok := gw.controllers[key]; ok {
		ctrl = state.ctrl
	}
	gw.updateToolApprovalModeDefaultLocked(msg, mode)
	gw.mu.Unlock()

	if ctrl != nil {
		ctrl.SetToolApprovalMode(mode)
	}
	if gw.cfg.OnToolApprovalModeChange != nil {
		return gw.cfg.OnToolApprovalModeChange(msg, mode)
	}
	return nil
}

func (gw *BotGateway) updateToolApprovalModeDefaultLocked(msg InboundMessage, mode string) {
	if id := strings.TrimSpace(msg.ConnectionID); id != "" {
		if gw.cfg.ConnectionChannels == nil {
			gw.cfg.ConnectionChannels = make(map[string]ChannelConfig)
		}
		channel := gw.cfg.ConnectionChannels[id]
		channel.ToolApprovalMode = mode
		gw.cfg.ConnectionChannels[id] = channel
		return
	}
	if msg.Platform != "" {
		if gw.cfg.Channels == nil {
			gw.cfg.Channels = make(map[Platform]ChannelConfig)
		}
		channel := gw.cfg.Channels[msg.Platform]
		channel.ToolApprovalMode = mode
		gw.cfg.Channels[msg.Platform] = channel
		return
	}
	gw.cfg.ToolApprovalMode = mode
}

func (gw *BotGateway) currentToolApprovalMode(key string, msg InboundMessage) string {
	var ctrl botController
	gw.mu.Lock()
	if state, ok := gw.controllers[key]; ok {
		ctrl = state.ctrl
	}
	gw.mu.Unlock()
	if ctrl != nil {
		return ctrl.ToolApprovalMode()
	}
	_, _, mode := gw.sessionOptionsForMessage(msg)
	return mode
}

func (gw *BotGateway) runTurn(ctx context.Context, adapter Adapter, key string, msg InboundMessage, cleanup func()) {
	gw.runTurnItem(ctx, adapter, key, msg, "", cleanup)
}

func (gw *BotGateway) runTurnItem(ctx context.Context, adapter Adapter, key string, msg InboundMessage, inboxItemID string, cleanup func()) {
	gw.logger.Info("bot turn started", "platform", msg.Platform, "chat_type", msg.ChatType, "chat", hashID(msg.ChatID), "session", key[:8])
	defer gw.finishTurnItem(ctx, adapter, key, msg, cleanup)

	// 获取或创建 Controller
	state := gw.getOrCreateSession(ctx, key, msg)
	if state == nil || state.ctrl == nil {
		_ = gw.sendText(ctx, adapter, msg, "内部错误：无法创建会话。")
		return
	}
	gw.rememberSessionReady(msg, state.ctrl)

	// 构建输入文本：群聊中在消息前加上发送者名，并把 IM 媒体保存为 @附件引用。
	input := msg.Text
	if inboxItemID == "" {
		input = gw.inputTextWithMedia(ctx, adapter, msg, state)
	}
	if inboxItemID == "" && msg.ChatType == ChatGroup {
		userName := strings.TrimSpace(msg.UserName)
		if msg.ResolveUserName != nil {
			if resolved := strings.TrimSpace(msg.ResolveUserName(ctx)); resolved != "" {
				userName = resolved
			}
		}
		input = fmt.Sprintf("[%s] %s", userName, input)
	}

	// 发送"正在输入"状态
	_ = adapter.SendTyping(ctx, msg.ChatID)

	// 创建事件渲染 sink
	sink := newRenderSink(
		ctx,
		adapter,
		msg.ConnectionID,
		msg.Domain,
		msg.ChatID,
		msg.ChatType,
		msg.UserID,
		msg.MessageID,
		gw.logger,
		func(approval event.Approval) {
			gw.mu.Lock()
			if state.pendingApprovals == nil {
				state.pendingApprovals = make(map[string]event.Approval)
			}
			state.pendingApprovals[approval.ID] = approval
			state.lastApprovalID = approval.ID
			gw.mu.Unlock()
		},
		func(ask event.Ask) {
			gw.mu.Lock()
			if state.pendingAsks == nil {
				state.pendingAsks = make(map[string][]event.AskQuestion)
			}
			state.pendingAsks[ask.ID] = ask.Questions
			state.lastAskID = ask.ID
			gw.mu.Unlock()
		},
	)
	// Finish initializing the sink before publishing it as the live target: once
	// setTarget runs, other goroutines can reach this sink via state.sink.Emit.
	sink.ctrl = state.ctrl
	state.sink.setTarget(sink)
	defer state.sink.setTarget(nil)

	// 创建带取消的 context
	turnCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	gw.mu.Lock()
	live := gw.controllers[key] == state
	if live {
		state.cancel = cancel
	}
	state.lastActive = time.Now()
	gw.mu.Unlock()
	if !live {
		// The session was closed (gateway stop or runtime rebuild) after this
		// turn picked it up; a cancel published now would never be consumed, so
		// abort the turn instead of running it uncancellable.
		cancel()
	}

	// 运行一轮对话
	var err error
	if inboxItemID == "" {
		err = state.ctrl.RunTurn(turnCtx, input)
	} else if api, ok := state.ctrl.(interface {
		RunInboxTurn(context.Context, string) error
	}); ok {
		err = api.RunInboxTurn(turnCtx, inboxItemID)
	} else {
		err = fmt.Errorf("controller cannot run durable inbox item")
	}
	sink.Emit(event.Event{Kind: event.TurnDone, Err: err})
	if err != nil {
		gw.logger.Warn("turn error", "session", key[:8], "err", err)
		return
	}
	gw.logger.Info("bot turn completed", "platform", msg.Platform, "chat_type", msg.ChatType, "chat", hashID(msg.ChatID), "session", key[:8])
}

func (gw *BotGateway) inputTextWithMedia(ctx context.Context, adapter Adapter, msg InboundMessage, state *sessionState) string {
	input := msg.Text
	if len(msg.MediaURLs) == 0 && len(msg.Media) == 0 {
		return input
	}
	workspaceRoot := ""
	if state != nil && state.ctrl != nil {
		workspaceRoot = state.ctrl.WorkspaceRoot()
	}
	if strings.TrimSpace(workspaceRoot) == "" {
		_, workspaceRoot, _ = gw.sessionOptionsForMessage(msg)
	}
	refs, errs := saveInboundMedia(ctx, workspaceRoot, msg.MediaURLs)
	itemRefs, fallbacks, itemErrs := saveInboundMediaItems(ctx, workspaceRoot, msg.Media)
	refs = append(refs, itemRefs...)
	errs = append(errs, itemErrs...)
	if len(errs) > 0 {
		gw.logger.Warn("bot media attachment failed", "platform", msg.Platform, "chat", hashID(msg.ChatID), "errors", len(errs))
		_ = gw.sendText(ctx, adapter, msg, fmt.Sprintf("有 %d 个附件保存失败；我会先处理可用内容。", len(errs)))
	}
	return appendMediaRefs(appendMediaFallbacks(input, fallbacks), refs)
}

// defaultBotApprovalTimeout caps how long a bot session waits for a remote
// user's approval/ask reply before treating it as denied, so an abandoned
// prompt (or a dropped IM event) can't leave the session wedged forever
// (#4626, #4402). 30 minutes is generous for a human reply yet bounded.
const defaultBotApprovalTimeout = 30 * time.Minute

// approvalTimeout resolves the configured bot approval wait: zero uses the
// bounded default; a negative value opts out (wait indefinitely).
func (gw *BotGateway) approvalTimeout() time.Duration {
	switch {
	case gw.cfg.ApprovalTimeout < 0:
		return 0
	case gw.cfg.ApprovalTimeout == 0:
		return defaultBotApprovalTimeout
	default:
		return gw.cfg.ApprovalTimeout
	}
}

func normalizeBotToolApprovalMode(mode string) string {
	if value := normalizeOptionalBotToolApprovalMode(mode); value != "" {
		return value
	}
	return control.ToolApprovalAsk
}

func normalizeOptionalBotToolApprovalMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case control.ToolApprovalAsk:
		return control.ToolApprovalAsk
	case control.ToolApprovalAuto:
		return control.ToolApprovalAuto
	case control.ToolApprovalYolo, "full", "full-access", "bypass":
		return control.ToolApprovalYolo
	default:
		return ""
	}
}

// UpdateConnectionToolApprovalMode updates the in-memory tool approval mode for
// a single bot connection without restarting the gateway. Empty mode clears the
// connection override, so existing sessions inherit the current gateway default.
func (gw *BotGateway) UpdateConnectionToolApprovalMode(connID, mode string) {
	connID = strings.TrimSpace(connID)
	if connID == "" {
		return
	}
	mode = normalizeOptionalBotToolApprovalMode(mode)
	type controllerMode struct {
		ctrl botController
		mode string
	}
	var updates []controllerMode

	gw.mu.Lock()
	if gw.cfg.ConnectionChannels == nil {
		gw.cfg.ConnectionChannels = make(map[string]ChannelConfig)
	}
	ch := gw.cfg.ConnectionChannels[connID]
	ch.ToolApprovalMode = mode
	gw.cfg.ConnectionChannels[connID] = ch
	// Update every active session that belongs to this connection.
	for _, state := range gw.controllers {
		if state == nil || state.ctrl == nil || strings.TrimSpace(state.connectionID) != connID {
			continue
		}
		effectiveMode := mode
		if effectiveMode == "" {
			effectiveMode = normalizeBotToolApprovalMode(gw.cfg.ToolApprovalMode)
		}
		updates = append(updates, controllerMode{ctrl: state.ctrl, mode: effectiveMode})
	}
	gw.mu.Unlock()

	for _, update := range updates {
		update.ctrl.SetToolApprovalMode(update.mode)
	}
}
