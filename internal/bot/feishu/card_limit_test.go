package feishu

import (
	"errors"
	"fmt"
	"testing"
)

// The codes were read out of the error's text, so any message quoting 11310
// matched and a send that carried the code in a field did not. The send path
// now returns the typed error the reply path already did.
func TestCardLimitIsReadFromTheCodeNotTheText(t *testing.T) {
	for _, code := range []int{feishuCardTooLargeCode, feishuCardInvalidCode} {
		err := error(&feishuAPIError{op: "send", code: code, msg: "card too large"})
		if !isCardLimitError(err) {
			t.Fatalf("code %d did not read as a card limit", code)
		}
		if !isCardLimitError(fmt.Errorf("sending: %w", err)) {
			t.Fatalf("code %d did not survive wrapping", code)
		}
	}
	if isCardLimitError(&feishuAPIError{op: "send", code: 99999, msg: "x"}) {
		t.Fatal("an unrelated code read as a card limit")
	}
	// A message that merely quotes the digits is not the API saying so.
	if isCardLimitError(errors.New("user said 11310 in chat")) {
		t.Fatal("text quoting the code read as a card limit")
	}
}

// isReplyFallbackError checks the op as well, so the send path taking the same
// type must not start looking like a recalled reply.
func TestSendErrorsAreNotMistakenForRecalledReplies(t *testing.T) {
	if isReplyFallbackError(&feishuAPIError{op: "send", code: feishuReplyRecalledCode, msg: "x"}) {
		t.Fatal("a send error read as a recalled reply")
	}
	if !isReplyFallbackError(&feishuAPIError{op: "reply", code: feishuReplyRecalledCode, msg: "x"}) {
		t.Fatal("a recalled reply stopped reading as one")
	}
}
