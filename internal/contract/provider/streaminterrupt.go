package provider

import (
	"errors"
	"io"

	"reasonix/internal/base/neterr"
)

// Fixed stream-interrupt reasons for observability. Values are a closed enum
// and must never carry URLs, tool arguments, file paths, or raw error text.
const (
	StreamInterruptConnectionReset = "connection_reset"
	StreamInterruptPrematureEOF    = "premature_eof"
	StreamInterruptIdleTimeout     = "idle_timeout"
)

// StreamInterruptedError marks a sampling attempt that never reached a clean
// provider terminal event and is therefore uncommitted; the Agent may replay
// the exact request. Replay lives there, not in a provider, so retry budgets,
// UI rollback and tool execution stay single-owner. context.Canceled, auth,
// 4xx/schema errors and complete-but-unparseable payloads are not this type.
type StreamInterruptedError struct {
	Err    error
	Reason string // one of the StreamInterrupt* constants; may be empty for older callers
}

func (e *StreamInterruptedError) Error() string {
	if e == nil || e.Err == nil {
		return "stream interrupted"
	}
	return e.Err.Error()
}

func (e *StreamInterruptedError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// StreamInterrupt wraps err as a StreamInterruptedError with a fixed reason.
func StreamInterrupt(err error, reason string) error {
	if err == nil {
		return nil
	}
	return &StreamInterruptedError{Err: err, Reason: reason}
}

// StreamInterruptReason returns the fixed reason when err is a stream
// interruption, or empty otherwise.
func StreamInterruptReason(err error) string {
	var interrupted *StreamInterruptedError
	if !errors.As(err, &interrupted) || interrupted == nil {
		return ""
	}
	if interrupted.Reason != "" {
		return interrupted.Reason
	}
	return ClassifyStreamInterrupt(interrupted.Err)
}

// ClassifyStreamInterrupt maps a transport error onto a fixed reason enum from
// its identity, never its text. An idle timeout is not in that identity at all
// -- only the read loop that set the deadline knows one happened -- so a stall
// attaches StreamInterruptIdleTimeout where it is detected, and never here.
func ClassifyStreamInterrupt(err error) string {
	if err == nil {
		return StreamInterruptPrematureEOF
	}
	switch {
	case errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, io.EOF):
		return StreamInterruptPrematureEOF
	default:
		if neterr.IsConnReset(err) {
			return StreamInterruptConnectionReset
		}
		return StreamInterruptPrematureEOF
	}
}

func IsStreamInterrupted(err error) bool {
	var interrupted *StreamInterruptedError
	return errors.As(err, &interrupted)
}
