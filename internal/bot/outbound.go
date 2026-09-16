package bot

// Sending back out: what was sent (so the gateway does not answer itself), and
// the reactions that have to be cleaned up afterwards.

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

func (gw *BotGateway) storeReactionCleanup(key string, cleanup func()) {
	if cleanup == nil {
		return
	}
	gw.mu.Lock()
	defer gw.mu.Unlock()
	gw.pendingReactionCleanups[key] = append(gw.pendingReactionCleanups[key], cleanup)
}

func (gw *BotGateway) flushReactionCleanups(key string, cleanup func()) {
	stored := gw.takeReactionCleanups(key)
	runReactionCleanups(stored)
	if cleanup != nil {
		cleanup()
	}
}

func (gw *BotGateway) takeReactionCleanups(key string) []func() {
	gw.mu.Lock()
	defer gw.mu.Unlock()
	stored := gw.pendingReactionCleanups[key]
	delete(gw.pendingReactionCleanups, key)
	return stored
}

func runReactionCleanups(cleanups []func()) {
	for _, cleanup := range cleanups {
		if cleanup != nil {
			cleanup()
		}
	}
}

func makeReactionCleanup(cleanups []func()) func() {
	if len(cleanups) == 0 {
		return nil
	}
	return func() {
		runReactionCleanups(cleanups)
	}
}

func (gw *BotGateway) addPendingReaction(ctx context.Context, plat Platform, adapter Adapter, msg InboundMessage) func() {
	if strings.TrimSpace(msg.MessageID) == "" {
		return nil
	}
	reactor, ok := adapter.(pendingReactionAdapter)
	if !ok {
		return nil
	}
	cleanup, err := reactor.AddPendingReaction(ctx, msg.MessageID)
	if err != nil {
		gw.logger.Warn("pending reaction failed", "platform", plat, "err", err)
		return nil
	}
	return cleanup
}

func (gw *BotGateway) isSelfMessage(msg InboundMessage) bool {
	if !gw.cfg.IgnoreSelfMessages {
		return false
	}
	actor := strings.TrimSpace(msg.UserID)
	if strings.TrimSpace(msg.OperatorID) != "" {
		actor = strings.TrimSpace(msg.OperatorID)
	}
	if actor != "" && gw.selfUserIDs[msg.Platform][actor] {
		return true
	}
	messageID := strings.TrimSpace(msg.MessageID)
	if messageID == "" {
		return false
	}
	key := outboundMessageKey(msg.Platform, msg.ConnectionID, msg.Domain, msg.ChatID, messageID)
	now := time.Now()
	gw.mu.Lock()
	defer gw.mu.Unlock()
	gw.pruneOutboundMessagesLocked(now)
	_, ok := gw.outboundMessageIDs[key]
	return ok
}

func (gw *BotGateway) rememberOutboundMessage(platform Platform, connID, domain, chatID, messageID string) {
	messageID = strings.TrimSpace(messageID)
	if !gw.cfg.IgnoreSelfMessages || messageID == "" {
		return
	}
	now := time.Now()
	key := outboundMessageKey(platform, connID, domain, chatID, messageID)
	gw.mu.Lock()
	defer gw.mu.Unlock()
	gw.pruneOutboundMessagesLocked(now)
	gw.outboundMessageIDs[key] = now.Add(outboundEchoTTL)
}

func (gw *BotGateway) pruneOutboundMessagesLocked(now time.Time) {
	for key, expiresAt := range gw.outboundMessageIDs {
		if !expiresAt.After(now) {
			delete(gw.outboundMessageIDs, key)
		}
	}
}

func outboundMessageKey(platform Platform, connID, domain, chatID, messageID string) string {
	return strings.Join([]string{
		string(platform),
		strings.TrimSpace(connID),
		strings.TrimSpace(domain),
		strings.TrimSpace(chatID),
		strings.TrimSpace(messageID),
	}, "\x00")
}

func (gw *BotGateway) sendText(ctx context.Context, adapter Adapter, msg InboundMessage, text string) error {
	out := OutboundMessage{
		ConnectionID: msg.ConnectionID,
		Domain:       msg.Domain,
		ChatID:       msg.ChatID,
		ChatType:     msg.ChatType,
		Text:         text,
		ReplyToMsgID: msg.MessageID,
	}
	binding := AdapterBinding{
		ID:       strings.TrimSpace(msg.ConnectionID),
		Domain:   strings.TrimSpace(msg.Domain),
		Platform: msg.Platform,
		Adapter:  adapter,
	}
	if binding.Platform == "" && adapter != nil {
		binding.Platform = adapter.Platform()
	}
	if binding.ID == "" && adapter != nil {
		binding.ID = adapter.Name()
	}
	result, err := gw.sendViaAdapter(ctx, binding, out)
	if err != nil {
		gw.logger.Warn("bot send failed", "platform", msg.Platform, "chat_type", msg.ChatType, "chat", hashID(msg.ChatID), "reply_to", hashID(msg.MessageID), "err", err)
		return err
	}
	gw.logger.Info("bot send completed", "platform", msg.Platform, "chat_type", msg.ChatType, "chat", hashID(msg.ChatID), "reply_to", hashID(msg.MessageID), "message", hashID(result.MessageID))
	return err
}

func (gw *BotGateway) sendViaAdapter(ctx context.Context, binding AdapterBinding, msg OutboundMessage) (SendResult, error) {
	if binding.Adapter == nil {
		return SendResult{}, errors.New("bot send: adapter is nil")
	}
	if strings.TrimSpace(msg.ConnectionID) == "" {
		msg.ConnectionID = binding.ID
	}
	if strings.TrimSpace(msg.Domain) == "" {
		msg.Domain = binding.Domain
	}
	result, err := binding.Adapter.Send(ctx, msg)
	gw.markAdapterSend(binding, err)
	for _, messageID := range result.DeliveredMessageIDs() {
		gw.rememberOutboundMessage(binding.Platform, binding.ID, binding.Domain, msg.ChatID, messageID)
	}
	return result, err
}

// SendToAdapter sends a message through the adapter identified by connID.
// Returns an error if no matching adapter is found.
func (gw *BotGateway) SendToAdapter(ctx context.Context, connID, domain string, msg OutboundMessage) (SendResult, error) {
	connID = strings.TrimSpace(connID)
	domain = strings.TrimSpace(domain)
	var target AdapterBinding
	gw.mu.Lock()
	for _, binding := range gw.adapters {
		if strings.TrimSpace(binding.ID) == connID &&
			(domain == "" || strings.EqualFold(strings.TrimSpace(binding.Domain), domain)) {
			target = binding
			break
		}
	}
	gw.mu.Unlock()
	if target.Adapter != nil {
		return gw.sendViaAdapter(ctx, target, msg)
	}
	return SendResult{}, fmt.Errorf("SendToAdapter: no adapter found for connection %q (domain %q)", connID, domain)
}

// SendTextToAdapter sends a plain text message through the adapter identified by connID.
func (gw *BotGateway) SendTextToAdapter(ctx context.Context, connID, domain, chatID string, chatType ChatType, text string) (SendResult, error) {
	return gw.SendToAdapter(ctx, connID, domain, OutboundMessage{
		ChatID:   chatID,
		ChatType: chatType,
		Text:     text,
	})
}
