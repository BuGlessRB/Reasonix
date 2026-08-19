// One Client's MCP session over a connection: request dispatch, per-call
// timeouts, progress-token plumbing, and the initialize handshake. Every
// dispatch names the connection it targets, so the handshake that brings a
// replacement connection up can run on it before it is published — otherwise it
// would recurse into the reconnect it is completing.
package plugin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"time"

	"reasonix/internal/tool"
)

func (c *Client) call(ctx context.Context, method string, params any) (json.RawMessage, error) {
	return c.callOn(ctx, c.t, method, params)
}

func (c *Client) callOn(ctx context.Context, t transport, method string, params any) (json.RawMessage, error) {
	params, unregisterProgress := c.withProgress(ctx, t, method, params)
	defer unregisterProgress()

	callCtx, cancel, timeout := c.contextWithCallTimeout(ctx, method, params)
	if cancel != nil {
		defer cancel()
	}

	res, err := c.callTransport(callCtx, t, method, params)
	if timeout > 0 && errors.Is(err, context.DeadlineExceeded) && callCtx.Err() == context.DeadlineExceeded && ctx.Err() == nil {
		slog.Warn("plugin: MCP call timed out",
			"server", c.name, "method", method, "tool", rawToolNameFromCallParams(params), "timeout", timeout)
		return nil, c.timeoutError(method, params, timeout)
	}
	return res, err
}

func (c *Client) withProgress(ctx context.Context, t transport, method string, params any) (any, func()) {
	if method != "tools/call" {
		return params, func() {}
	}
	sink, ok := tool.ProgressFrom(ctx)
	if !ok {
		return params, func() {}
	}
	router, ok := t.(progressTransport)
	if !ok {
		return params, func() {}
	}
	callParams, ok := params.(map[string]any)
	if !ok {
		return params, func() {}
	}

	token := fmt.Sprintf("reasonix-%d", c.progressID.Add(1))
	copyParams := make(map[string]any, len(callParams))
	maps.Copy(copyParams, callParams)
	meta := map[string]any{}
	if existing, ok := callParams["_meta"].(map[string]any); ok {
		maps.Copy(meta, existing)
	}
	meta["progressToken"] = token
	copyParams["_meta"] = meta
	unregister := router.registerProgress(token, sink)
	return copyParams, unregister
}

func (c *Client) callTransport(ctx context.Context, t transport, method string, params any) (json.RawMessage, error) {
	res, err := t.call(ctx, method, params)
	if err == nil || method == "initialize" || !isHTTPSessionExpired(err) {
		return res, err
	}
	if initErr := c.initializeSessionOn(ctx, t, false); initErr != nil {
		return nil, fmt.Errorf("%w; reinitialize failed: %w", err, initErr)
	}
	return t.call(ctx, method, params)
}

func (c *Client) contextWithCallTimeout(ctx context.Context, method string, params any) (context.Context, context.CancelFunc, time.Duration) {
	if _, ok := ctx.Deadline(); ok {
		return ctx, nil, 0
	}
	timeout := c.callTimeout(method, params)
	if timeout <= 0 {
		timeout = defaultCallTimeout
	}
	callCtx, cancel := context.WithTimeout(ctx, timeout)
	return callCtx, cancel, timeout
}

func (c *Client) callTimeout(method string, params any) time.Duration {
	if method == "tools/call" {
		if raw := rawToolNameFromCallParams(params); raw != "" {
			if timeout := c.spec.ToolTimeouts[raw]; timeout > 0 {
				return timeout
			}
		}
	}
	if c.spec.CallTimeout > 0 {
		return c.spec.CallTimeout
	}
	if c.spec.DefaultCallTimeout > 0 {
		return c.spec.DefaultCallTimeout
	}
	return defaultCallTimeout
}

func rawToolNameFromCallParams(params any) string {
	m, ok := params.(map[string]any)
	if !ok {
		return ""
	}
	name, _ := m["name"].(string)
	return name
}

func (c *Client) timeoutError(method string, params any, timeout time.Duration) error {
	if method == "tools/call" {
		if raw := rawToolNameFromCallParams(params); raw != "" {
			return fmt.Errorf("MCP tool %q timed out after %s; increase tool_timeout_seconds or call_timeout_seconds to allow longer runs: %w",
				c.name+"."+raw, formatTimeout(timeout), context.DeadlineExceeded)
		}
	}
	return fmt.Errorf("MCP method %q on server %q timed out after %s; increase mcp_call_timeout_seconds or call_timeout_seconds to allow longer runs: %w",
		method, c.name, formatTimeout(timeout), context.DeadlineExceeded)
}

func formatTimeout(timeout time.Duration) string {
	if timeout > 0 && timeout%time.Second == 0 {
		return fmt.Sprintf("%ds", int(timeout/time.Second))
	}
	return timeout.String()
}

func (c *Client) notify(ctx context.Context, method string, params any) error {
	return c.t.notify(ctx, method, params)
}

func (c *Client) notifyOn(ctx context.Context, t transport, method string, params any) error {
	return t.notify(ctx, method, params)
}

func isHTTPSessionExpired(err error) bool {
	var expired *httpSessionExpiredError
	return errors.As(err, &expired)
}

func (c *Client) initialize(ctx context.Context) error {
	return c.initializeSessionOn(ctx, c.t, true)
}

func (c *Client) initializeSessionOn(ctx context.Context, t transport, recordCapabilities bool) error {
	capabilities := map[string]any{}
	if len(mcpRoots(c.spec.WorkspaceRoot)) > 0 {
		capabilities["roots"] = map[string]any{"listChanged": false}
	}
	res, err := c.callOn(ctx, t, "initialize", map[string]any{
		"protocolVersion": protocolVersion,
		"capabilities":    capabilities,
		"clientInfo":      map[string]any{"name": "reasonix", "version": "dev"},
	})
	if err != nil {
		return err
	}
	if !recordCapabilities {
		// Runtime session refresh must not rewrite startup-only capability flags.
		return c.notifyOn(ctx, t, "notifications/initialized", map[string]any{})
	}
	// Record which optional capabilities the server advertises. Presence of the
	// key (even with an empty object) signals support.
	var ir struct {
		Capabilities map[string]json.RawMessage `json:"capabilities"`
	}
	if err := json.Unmarshal(res, &ir); err != nil {
		slog.Warn("plugin: parse initialize capabilities", "server", c.name, "err", err)
	}
	_, c.hasTools = ir.Capabilities["tools"]
	_, c.hasPrompts = ir.Capabilities["prompts"]
	_, c.hasResources = ir.Capabilities["resources"]

	return c.notifyOn(ctx, t, "notifications/initialized", map[string]any{})
}

// redial opens a replacement connection for this client's server. It re-clears
// the same launcher lock and project launch grant the first connection did, so
// a replacement child can never start on an authorization the user has since
// withdrawn.
func (c *Client) redial(lifeCtx, callCtx context.Context) (transport, error) {
	s, err := applyStoredLauncherLock(c.spec)
	if err != nil {
		return nil, err
	}
	s, err = resolveProjectLaunchAuthorization(callCtx, s)
	if err != nil {
		return nil, err
	}
	return newTransport(lifeCtx, s)
}
