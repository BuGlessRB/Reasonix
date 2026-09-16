package neterr

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"testing"
)

func TestIsConnReset(t *testing.T) {
	if IsConnReset(nil) {
		t.Error("nil is not a conn reset")
	}
	if IsConnReset(context.Canceled) || IsConnReset(context.DeadlineExceeded) {
		t.Error("ctx cancel/deadline must not look like a recoverable reset")
	}
	if IsConnReset(errors.New("decode stream: invalid character")) {
		t.Error("a plain protocol error must not be treated as a conn reset")
	}
	for _, err := range []error{
		io.ErrUnexpectedEOF,
		&net.OpError{Op: "read", Err: resetErrors[0]},
		fmt.Errorf("read stream: %w", &net.OpError{Op: "read", Err: errors.New("wsarecv: forcibly closed")}),
	} {
		if !IsConnReset(err) {
			t.Errorf("want conn reset for %v", err)
		}
	}
}

func TestIsTransient(t *testing.T) {
	if IsTransient(nil) {
		t.Error("nil is not transient")
	}
	if IsTransient(context.Canceled) || IsTransient(context.DeadlineExceeded) {
		t.Error("the caller's own cancellation is never worth retrying")
	}
	if IsTransient(errors.New("invalid request: field missing")) {
		t.Error("a protocol error is not a transport failure")
	}
	for _, err := range []error{
		io.ErrUnexpectedEOF,
		&net.OpError{Op: "read", Err: resetErrors[0]},
		&net.OpError{Op: "dial", Err: refusedErrors[0]},
		timeoutError{},
	} {
		if !IsTransient(err) {
			t.Errorf("want transient for %v", err)
		}
	}
}

// resetErrors and refusedErrors hold the codes this platform actually
// reports, so a lookup that matched nothing would make both tests vacuous.
func TestPlatformCodesAreDistinct(t *testing.T) {
	if len(resetErrors) == 0 || len(refusedErrors) == 0 {
		t.Fatal("platform error identities are empty")
	}
	for _, reset := range resetErrors {
		for _, refused := range refusedErrors {
			if errors.Is(reset, refused) {
				t.Fatalf("%v and %v are the same identity", reset, refused)
			}
		}
	}
}

type timeoutError struct{}

func (timeoutError) Error() string { return "i/o timeout" }
func (timeoutError) Timeout() bool { return true }
func (timeoutError) Temporary() bool {
	return true
}
