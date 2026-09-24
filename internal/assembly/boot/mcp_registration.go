package boot

import (
	"context"
	"fmt"
	"time"

	"reasonix/internal/contract/config"
	"reasonix/internal/contract/event"
	"reasonix/internal/contract/tool"
	"reasonix/internal/ext/plugin"
	"reasonix/internal/platform/lsp"
)

// registerMCPTools puts every enabled server into the tool catalog and returns
// the configured specs it registered. Host-session servers take a short
// readiness probe; configured ones stay process-idle until their first call.
func registerMCPTools(ctx context.Context, host *plugin.Host, reg *tool.Registry, plan mcpSpecPlan, sink event.Sink) []plugin.Spec {
	for _, s := range plan.extra {
		registerHostSessionServer(ctx, host, reg, s, sink)
	}
	// The eager tier already carries the host-session servers connected above.
	configSpecs := withoutSpecs(append(append([]plugin.Spec{}, plan.eager...), plan.background...), plan.extra)
	for _, s := range configSpecs {
		registerConfiguredServer(ctx, host, reg, s)
	}
	for _, msg := range plan.demotions {
		report(sink, event.Event{Level: event.LevelInfo, Text: msg})
	}
	return configSpecs
}

// registerHostSessionServer connects a server the host session named for this
// controller, so recovery and session-scoped servers are deterministic. A
// failure still leaves a catalog entry for /mcp to diagnose.
func registerHostSessionServer(ctx context.Context, host *plugin.Host, reg *tool.Registry, s plugin.Spec, sink event.Sink) {
	if host.HasClient(s.Name) {
		if tools, err := host.ToolsFor(ctx, s.Name); err == nil {
			addTools(reg, tools)
			return
		}
	}
	addCtx, addCancel := context.WithTimeout(ctx, 5*time.Second)
	tools, err := host.EnsureConnectedWithLifecycle(ctx, addCtx, s, 0)
	addCancel()
	if err == nil {
		addTools(reg, tools)
		return
	}
	if plugin.IsServerAlreadyConnected(err) {
		if tools, err2 := host.ToolsFor(ctx, s.Name); err2 == nil {
			addTools(reg, tools)
			return
		}
	}
	cs, _ := plugin.LoadCachedSchemaForSpec(s)
	addTools(reg, plugin.LazyToolset(s, cs, host, reg, ctx, false))
	report(sink, event.Event{Level: event.LevelWarn,
		Text: "An MCP server failed to start.", Detail: fmt.Sprintf("mcp %s: %v", s.Name, err)})
}

// registerConfiguredServer registers placeholders from the cached schema, and
// starts a process for catalog discovery only when no usable schema is cached.
func registerConfiguredServer(ctx context.Context, host *plugin.Host, reg *tool.Registry, s plugin.Spec) {
	if host.HasClient(s.Name) {
		if tools, err := host.ToolsFor(ctx, s.Name); err == nil {
			addTools(reg, tools)
			return
		}
	}
	cs, _ := plugin.LoadCachedSchemaForSpec(s)
	kick := cs == nil || len(cs.Tools) == 0
	addTools(reg, plugin.LazyToolset(s, cs, host, reg, ctx, kick))
}

func withoutSpecs(specs, drop []plugin.Spec) []plugin.Spec {
	if len(drop) == 0 {
		return specs
	}
	names := make(map[string]bool, len(drop))
	for _, s := range drop {
		names[s.Name] = true
	}
	kept := specs[:0]
	for _, s := range specs {
		if !names[s.Name] {
			kept = append(kept, s)
		}
	}
	return kept
}

func addTools(reg *tool.Registry, tools []tool.Tool) {
	for _, t := range tools {
		reg.Add(t)
	}
}

// registerLSP adds the LSP tools. Servers resolve on PATH and spawn on first
// query, so registering is cheap even with none installed.
func registerLSP(reg *tool.Registry, cfg *config.Config, root string) *lsp.Manager {
	if !cfg.LSP.Enabled {
		return nil
	}
	mgr := lsp.NewManager(root, LSPSpecs(cfg.LSP))
	for _, t := range lsp.Tools(mgr) {
		if t != nil {
			reg.Add(t)
		}
	}
	return mgr
}
