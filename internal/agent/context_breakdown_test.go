package agent

import (
	"testing"

	"reasonix/internal/provider"
)

// The gauge says a session is at 70%; this says what put it there. Each class
// is measured with the same estimator the thresholds use, so the parts describe
// the same prompt the gauge does rather than a second opinion about it.
func TestContextBreakdownSeparatesWhatFillsTheWindow(t *testing.T) {
	sess := NewSession("你是一个助手")
	a := New(nil, nil, sess, Options{ContextWindow: 128000}, nil)
	sess.Add(provider.Message{Role: provider.RoleUser, Content: "把这个仓库跑一遍测试"})
	sess.Add(provider.Message{Role: provider.RoleAssistant, Content: "好，我先看构建脚本"})
	sess.Add(provider.Message{Role: provider.RoleTool, Content: string(make([]byte, 4000))})

	b := a.ContextBreakdown()
	if b.User == 0 || b.Reply == 0 || b.Output == 0 {
		t.Fatalf("every class that has messages must report tokens: %+v", b)
	}
	if b.Output <= b.User {
		t.Fatalf("a 4KB tool result must outweigh a one-line prompt: %+v", b)
	}
	if b.Total == 0 {
		t.Fatalf("total must match the gauge, got %+v", b)
	}
}

// An empty class costs nothing to report and must not be guessed at.
func TestContextBreakdownReportsZeroForClassesWithNoMessages(t *testing.T) {
	a := New(nil, nil, NewSession("你是一个助手"), Options{ContextWindow: 128000}, nil)
	b := a.ContextBreakdown()
	if b.User != 0 || b.Reply != 0 || b.Output != 0 {
		t.Fatalf("a fresh session has no turns yet: %+v", b)
	}
}
