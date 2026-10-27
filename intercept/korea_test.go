package intercept

import (
	"encoding/json"
	"strings"
	"testing"
	"unicode"
)

func TestKoreanJudgeVerdicts(t *testing.T) {
	for _, decision := range []string{"allow", "ask", "deny"} {
		comment := "실제 작업: 파일 읽기; 성공 시 영향: 내용 반환; 적용 규칙: A5"
		raw, _ := json.Marshal(map[string]string{"decision": decision, "comment": comment})
		got := ParseVerdict(string(raw))
		if got.Action != decision || got.Reason != comment {
			t.Fatalf("lost Korean verdict: %+v", got)
		}
		for _, invalid := range []string{
			"실제 작업: ; 성공 시 영향: 내용 반환; 적용 규칙: A5",
			"실제 작업: 파일 읽기; 성공 시 영향: ; 적용 규칙: A5",
			"실제 작업: 파일 읽기; 성공 시 영향: 내용 반환; 적용 규칙: ",
		} {
			raw, _ = json.Marshal(map[string]string{"decision": decision, "comment": invalid})
			if ParseVerdict(string(raw)).Action != "" {
				t.Fatal("accepted incomplete Korean verdict")
			}
		}
	}
	if strings.IndexFunc(DefaultJudgePrompt, func(r rune) bool { return unicode.Is(unicode.Han, r) }) >= 0 {
		t.Fatal("Chinese default judge prompt")
	}
}
