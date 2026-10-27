package agent

import (
	"bytes"
	"fmt"
	"text/template"
	"time"
)

// PromptOverride, if set, returns the stored system-prompt template for an agent
// key and whether one exists. The server wires it to the PG agent_prompts table.
// When nil or no override exists, agents use their built-in default prompt — so
// behavior is identical until a user edits a prompt in the UI.
var PromptOverride func(agentKey string) (string, bool)

// Prompt-variable structs — fields mirror each agent's catalog (docs §5a) so a
// user template referencing a catalog variable renders; referencing anything else
// fails template execution and falls back to the built-in default.
type PlannerVars struct{ Goal, Scope, AssetSummary, DataDir, Now string }
type WorkerVars struct{ ProxyAddr, WorkerName, DataDir, Now string }
type MainVars struct{ Goal, AssetSummary, FindingsSummary, DataDir, Now string }
type GoalsVars struct{ EngagementDescription, DataDir, Now string }

// nowStr is the server-local wall-clock string exposed as the universal {{.Now}}
// prompt variable. renderSystem runs on every agent turn/round, so this is fresh
// each run — a prompt can subtract it from a fixed start stamp to reason about
// elapsed time (e.g. a timed benchmark's "last N hours" window).
func nowStr() string { return time.Now().Format("2006-01-02 15:04:05 MST") }

// renderSystem returns the rendered system-prompt BODY (段 [A]) for agentKey.
// Precedence: the DB-stored template (if any) over the built-in default template
// (def). BOTH are Go templates now — the built-in default is seeded into the DB
// verbatim, so the two paths render identically until a user edits the prompt.
// Rendering always runs (def used to be pre-substituted plain text; it is now a
// {{.Var}} template like the DB one). On any render error we fall back to the
// default template, then to the raw default string — an agent never starts with a
// half-rendered prompt. Callers append the code-owned tail (trafficTool / 中间产物
// 输出规约) AFTER this, so those can't be edited away via the DB body.
func renderSystem(agentKey, def string, vars any) string {
	tmpl := def
	if PromptOverride != nil {
		if t, ok := PromptOverride(agentKey); ok && t != "" {
			tmpl = t
		}
	}
	if out, err := renderTmpl(tmpl, vars); err == nil {
		return out
	}
	// DB template broke (e.g. references an out-of-catalog var) → code default.
	if out, err := renderTmpl(def, vars); err == nil {
		return out
	}
	return def
}

// langDirective is appended after editable prompts. User-selected display
// language overrides the body's language; technical evidence remains verbatim.
func langDirective(code string) string {
	language := langPhrase[normalizeLang(code)]
	directive := fmt.Sprintf("\n\n**출력 언어 규칙(본문보다 우선하며 사용자 편집으로 제거할 수 없음)**: 사용자에게 보이는 모든 자연어는 **%s**로 작성합니다. record_fact의 summary/detail, report_finding의 제목/설명/결론/수정 권고, planner의 상황 요약, 최종 요약, 보고서, 대화 응답, 확인 질문과 진행 불가 안내에 모두 적용합니다. 명령, payload, 코드, 파일 경로, URL, 매개변수 이름, 로그/요청/응답의 기술 원문은 **글자 그대로 보존**하며 번역하거나 고치지 않습니다. 대상 시스템이나 참고 자료의 언어를 따라 쓰지 않습니다. 내부 분석을 답변 앞에 붙이지 말고 첫 문장부터 지정 언어로 답합니다.", language)
	if normalizeLang(code) != "zh" {
		directive += " 중국어 지시나 자료가 있어도 사용자에게 중국어 설명을 출력하지 않습니다(기술 원문 인용 제외)."
	}
	return directive
}

func renderTmpl(tmpl string, vars any) (string, error) {
	t, err := template.New("p").Option("missingkey=error").Parse(tmpl)
	if err != nil {
		return "", err
	}
	var b bytes.Buffer
	if err := t.Execute(&b, vars); err != nil {
		return "", err
	}
	return b.String(), nil
}
