package boot

import (
	"context"
	"errors"

	"reasonix/internal/agent"
	"reasonix/internal/event"
	"reasonix/internal/provider"
	"reasonix/internal/tool"
)

func preserveSubagentFailure(run *agent.SubagentRun, store *agent.SubagentStore, cause error) (string, error) {
	subErr := agent.NewSubagentRunError(run, cause)
	var saveErr error
	if store != nil {
		saveErr = store.SaveOutcome(run, subErr.Outcome)
	}
	return subErr.SubagentOutput(), errors.Join(subErr, saveErr)
}

func runReadOnlySkillSession(ctx context.Context, prov provider.Provider, reg *tool.Registry, prompt string, opts agent.Options, sink event.Sink, systemPrompt string,
	runner func(context.Context, provider.Provider, *tool.Registry, *agent.Session, string, agent.Options, event.Sink) (string, error),
) (string, error) {
	run := agent.EphemeralSubagentRun(systemPrompt)
	answer, err := runner(ctx, prov, reg, run.Session, prompt, opts, sink)
	if err != nil {
		return preserveSubagentFailure(run, nil, err)
	}
	return answer, nil
}

func saveSubagentCompleted(store *agent.SubagentStore, run *agent.SubagentRun) error {
	if store == nil {
		return nil
	}
	return store.SaveCompleted(run)
}
