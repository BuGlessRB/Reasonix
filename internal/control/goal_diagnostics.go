package control

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"time"

	goaldomain "reasonix/internal/goal"
	"reasonix/internal/session"
)

// GoalDiagnosticMetadata is supplied by the host so a user-exported artifact
// identifies the exact build and negotiated feature surface that produced it.
type GoalDiagnosticMetadata struct {
	ApplicationVersion string   `json:"applicationVersion,omitempty"`
	BuildCommit        string   `json:"buildCommit,omitempty"`
	ProtocolVersion    int      `json:"protocolVersion,omitempty"`
	Capabilities       []string `json:"capabilities"`
}

type goalDiagnosticExport struct {
	SchemaVersion     int                        `json:"schemaVersion"`
	ExportedAt        time.Time                  `json:"exportedAt"`
	Metadata          GoalDiagnosticMetadata     `json:"metadata"`
	Runtime           session.RuntimeSnapshot    `json:"runtime"`
	Observation       any                        `json:"observation"`
	AcceptedThrough   uint64                     `json:"acceptedThrough"`
	DurableThrough    uint64                     `json:"durableThrough"`
	Commits           []session.Commit           `json:"commits"`
	ActivationChanges []goalDiagnosticTransition `json:"activationChanges"`
	Unavailable       []string                   `json:"unavailable"`
}

type goalDiagnosticTransition struct {
	Sequence      uint64                `json:"sequence"`
	OperationID   string                `json:"operationId"`
	GoalID        string                `json:"goalId,omitempty"`
	Revision      uint64                `json:"revision,omitempty"`
	Phase         goaldomain.Phase      `json:"phase,omitempty"`
	Activation    goaldomain.Activation `json:"activation"`
	RoundsStarted uint64                `json:"roundsStarted,omitempty"`
}

// ExportGoalDiagnostics is the compatibility in-memory form. Production hosts
// use WriteGoalDiagnostics so diagnostic size is not a memory or RPC limit.
func (c *Controller) ExportGoalDiagnostics(ctx context.Context, metadata GoalDiagnosticMetadata) ([]byte, error) {
	var output bytes.Buffer
	if err := c.WriteGoalDiagnostics(ctx, &output, metadata); err != nil {
		return nil, err
	}
	return output.Bytes(), nil
}

// WriteGoalDiagnostics streams the authoritative event log after a Flush
// checkpoint. Complete tool payloads are emitted one commit at a time; the
// cumulative log is never replayed into a []Commit or marshalled as one blob.
func (c *Controller) WriteGoalDiagnostics(ctx context.Context, dst io.Writer, metadata GoalDiagnosticMetadata) error {
	if c == nil {
		return session.ErrSessionNotRunning
	}
	_, runtime, exclusive := c.v3Binding()
	if !exclusive || runtime == nil {
		return errors.New("goal diagnostics require a canonical session")
	}
	temporaryRoot, err := os.MkdirTemp("", "reasonix-goal-diagnostics-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(temporaryRoot)
	frozen := filepath.Join(temporaryRoot, "session")
	// Store.Export owns the commit and drain boundaries around its Flush, so the
	// diagnostic never pairs a manifest from one prefix with events from another.
	if err := runtime.Session().Export(ctx, frozen); err != nil {
		return err
	}
	if metadata.Capabilities == nil {
		metadata.Capabilities = []string{}
	}
	fillGoalDiagnosticBuildMetadata(&metadata)
	state := runtime.StateSnapshot()
	through := state.Session.DurableSequence
	if _, err := io.WriteString(dst, "{\n"); err != nil {
		return err
	}
	fields := []struct {
		name  string
		value any
	}{
		{"schemaVersion", 1},
		{"exportedAt", time.Now().UTC()},
		{"metadata", metadata},
		{"runtime", state},
		{"observation", c.RuntimeStateSnapshot()},
		{"acceptedThrough", through},
		{"durableThrough", through},
	}
	for _, field := range fields {
		if err := writeGoalDiagnosticField(dst, field.name, field.value, true); err != nil {
			return err
		}
	}
	if _, err := io.WriteString(dst, "  \"commits\": ["); err != nil {
		return err
	}
	first := true
	changes := []goalDiagnosticTransition{}
	activation := goaldomain.ActivationDisarmed
	err = session.VisitCommits(ctx, frozen, func(commit session.Commit) error {
		encoded, err := json.MarshalIndent(commit, "    ", "  ")
		if err != nil {
			return err
		}
		separator := "\n    "
		if !first {
			separator = ",\n    "
		}
		if _, err := io.WriteString(dst, separator); err != nil {
			return err
		}
		if _, err := dst.Write(encoded); err != nil {
			return err
		}
		first = false
		changes = append(changes, goalActivationChangesForCommit(commit, &activation)...)
		return nil
	})
	if err != nil {
		return err
	}
	if !first {
		if _, err := io.WriteString(dst, "\n  "); err != nil {
			return err
		}
	}
	if _, err := io.WriteString(dst, "],\n"); err != nil {
		return err
	}
	if err := writeGoalDiagnosticField(dst, "activationChanges", changes, true); err != nil {
		return err
	}
	if err := writeGoalDiagnosticField(dst, "unavailable", []string{}, false); err != nil {
		return err
	}
	_, err = io.WriteString(dst, "}\n")
	return err
}

func writeGoalDiagnosticField(dst io.Writer, name string, value any, comma bool) error {
	encoded, err := json.MarshalIndent(value, "  ", "  ")
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(dst, "  %q: ", name); err != nil {
		return err
	}
	if _, err := dst.Write(encoded); err != nil {
		return err
	}
	if comma {
		_, err = io.WriteString(dst, ",")
		if err != nil {
			return err
		}
	}
	_, err = io.WriteString(dst, "\n")
	return err
}

func fillGoalDiagnosticBuildMetadata(metadata *GoalDiagnosticMetadata) {
	if metadata == nil {
		return
	}
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return
	}
	if metadata.ApplicationVersion == "" && info.Main.Version != "" && info.Main.Version != "(devel)" {
		metadata.ApplicationVersion = info.Main.Version
	}
	if metadata.BuildCommit != "" {
		return
	}
	for _, setting := range info.Settings {
		if setting.Key == "vcs.revision" {
			metadata.BuildCommit = setting.Value
			return
		}
	}
}

func goalActivationChanges(commits []session.Commit) []goalDiagnosticTransition {
	changes := make([]goalDiagnosticTransition, 0)
	activation := goaldomain.ActivationDisarmed
	for _, commit := range commits {
		changes = append(changes, goalActivationChangesForCommit(commit, &activation)...)
	}
	return changes
}

func goalActivationChangesForCommit(commit session.Commit, activation *goaldomain.Activation) []goalDiagnosticTransition {
	changes := []goalDiagnosticTransition{}
	hasTurnStart := false
	for _, item := range commit.Events {
		hasTurnStart = hasTurnStart || item.Kind == "turn/start"
	}
	for _, item := range commit.Events {
		if item.Kind != "goal/state" {
			continue
		}
		var document struct {
			Current *goaldomain.Snapshot `json:"current"`
		}
		if json.Unmarshal(item.Payload, &document) != nil {
			continue
		}
		transition := goalDiagnosticTransition{Sequence: item.Sequence, OperationID: commit.OperationID, Activation: goaldomain.ActivationDisarmed}
		if document.Current != nil {
			transition.GoalID = document.Current.ID
			transition.Revision = document.Current.Revision
			transition.Phase = document.Current.Phase
			transition.RoundsStarted = document.Current.RoundsStarted
			if document.Current.Phase == goaldomain.PhaseActive {
				op := strings.ToLower(commit.OperationID)
				if hasTurnStart || strings.Contains(op, ":create") || strings.Contains(op, ":resume") || strings.Contains(op, "goal-control:set") {
					*activation = goaldomain.ActivationArmed
				}
			} else {
				*activation = goaldomain.ActivationDisarmed
			}
			transition.Activation = *activation
		} else {
			*activation = goaldomain.ActivationDisarmed
		}
		changes = append(changes, transition)
	}
	return changes
}
