package boot

import (
	"log/slog"
	"strings"

	"reasonix/internal/agent"
	"reasonix/internal/completioneval"
	"reasonix/internal/config"
	"reasonix/internal/event"
	"reasonix/internal/netclient"
)

// completionEval wires the completion validator into agent/task options.
type completionEval struct {
	factory func() completioneval.Evaluator
	mode    string
}

func newCompletionEval(cfg *config.Config, modelRef string, proxySpec netclient.ProxySpec, sink event.Sink) completionEval {
	mode := cfg.Agent.CompletionValidationMode()
	return completionEval{factory: newCompletionEvalFactory(cfg, modelRef, proxySpec, sink), mode: mode}
}

func (c completionEval) options(opts agent.Options) agent.Options {
	opts.CompletionEvaluatorFactory = c.factory
	opts.CompletionValidation = c.mode
	return opts
}

func (c completionEval) taskOptions(opts agent.TaskToolOptions) agent.TaskToolOptions {
	opts.CompletionEvaluatorFactory = c.factory
	opts.CompletionValidation = c.mode
	return opts
}

// newCompletionEvalFactory builds the completion-validator session factory.
// The validator follows the working model unless explicitly configured, so
// session content never implicitly crosses to another model. Each agent
// (executor, planner, every sub-agent) draws its own session from the factory,
// so concurrent validation calls never serialize. When the provider cannot be
// built the factory stays nil and the deterministic host repairs keep working.
func newCompletionEvalFactory(cfg *config.Config, modelRef string, proxySpec netclient.ProxySpec, sink event.Sink) func() completioneval.Evaluator {
	mode := cfg.Agent.CompletionValidationMode()
	if mode == config.CompletionValidationOff {
		return nil
	}
	evalRef := modelRef
	if m := strings.TrimSpace(cfg.Agent.CompletionEvaluatorModel); m != "" {
		evalRef = m
	}
	ce, ok := cfg.ResolveModel(evalRef)
	if !ok {
		slog.Warn("completion evaluator model not found — validation runs off", "model", evalRef)
		return nil
	}
	cProv, err := NewProviderWithProxy(ce, proxySpec)
	if err != nil {
		slog.Warn("completion evaluator provider construction failed — validation runs off", "model", evalRef, "err", err)
		return nil
	}
	price, refLabel := ce.Price, modelRefFromEntry(ce)
	return func() completioneval.Evaluator {
		return completioneval.NewSession(cProv, price, refLabel, sink)
	}
}
