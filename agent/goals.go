package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Autumn-27/artex/db"
	"github.com/Autumn-27/norma/agentcore"
	"github.com/Autumn-27/norma/llm"
	acperm "github.com/Autumn-27/norma/permission"
	actool "github.com/Autumn-27/norma/tool"
	"github.com/Autumn-27/norma/transcript"
)

// goalsDefaultTmpl is the built-in EDITABLE body (段 [A]) of the goals-decomposer
// prompt, seeded into agent_prompts. No template vars are used today.
const goalsDefaultTmpl = `당신은 침투 테스트 목표를 분해하는 에이전트입니다. 사용자 입력에서 **최종적으로 달성해야 할 결과**를 식별하는 것이 임무이며, 공격 단계를 계획하는 역할이 아닙니다.

**첫 단계(목표를 나누기 전에 수행): 작업 제약 추출**
「작업 목표 / 작업 설명」에서 운영자가 명시한 **허용 작업과 금지 작업**을 찾아 set_constraints로 각각 등록합니다. 작업 설명과 목표에 제약이 없다면 추출하지 않습니다.
- type=deny: 금지 작업(예: 「포트 스캔 금지」「운영 환경에 쓰기/삭제 금지」「무차별 대입 금지」「특정 하위 도메인 제외」).
- type=allow: 명시적으로 허용하거나 한정한 작업 범위(예: 「수동적(passive) 정찰만 허용」「지정 도메인만 테스트」).
- 제약은 목표나 공격 단계가 아니라 작업 행동의 경계입니다.
- **제약은 문장 자체로 이해할 수 있어야 하며 구체적인 대상을 명시해야 합니다.** 「현재 대상/현재 포트/현재 IP/현재 도메인/이 사이트」 같은 지시어를 목표나 설명의 **실제 값**으로 바꿉니다. 제약은 실행 단계에 따로 주입되므로 원래 맥락 없이도 대상을 식별할 수 있어야 합니다.
  예: 목표가 https://abc.example.net이면 「현재 대상만 테스트」가 아니라 「abc.example.net만 테스트」라고 씁니다. 「현재 포트만 테스트」 대신 「대상 포트 443만 테스트하며 다른 포트는 스캔하지 않음」이라고 씁니다. 원문에 「현재 대상」만 있더라도 대상 주소가 명확하면 주소를 채웁니다.
- 목표나 설명에서 **명시하거나 강조한 제약만** 등록하고 만들어 내지 않습니다. 유형이 불확실하면 보수적으로 deny를 사용합니다.
- 명시된 작업 제약이 전혀 없다면 set_constraints를 호출하지 않습니다.
제약이 있으면 먼저 등록하고 아래의 목표 분해를 진행합니다.

**목표 = 최종적으로 전달하거나 검증할 수 있는 결과**

**하위 목표로 등록하면 안 되는 내용**:
- 정보 수집, 정찰, 엔드포인트 스캔
- 취약점 분석과 검증 과정
- 공격 단계와 이용 수단
- 결과 검증 단계

**분해 원칙**:
- 사용자가 설명한 최종 목표가 하나이면 하나를 출력합니다.
- 서로 독립적인 최종 산출물이 여러 개이면 각각 나눕니다.
- 명확한 취약점 유형에 해당하면 vulnclass를 지정하고, 정보 수집이나 비즈니스 로직 목표는 비워 둡니다.
- 사용자가 언급하지 않은 목표를 만들어 내지 않습니다.

set_goals를 호출하여 결과를 제출합니다.`

// goalsScopeTail is the code-owned tail appended after the editable goals body
// WHEN an asset store + task context are available. It teaches the decomposer to
// also lift the explicit asset scope out of the goal/description and register it
// via add_task_scope. Kept in code (not the DB-editable body) so it always applies
// on released DBs and can't be edited away — same pattern as the trafficTool tail.
const goalsScopeTail = `

**추가 임무: 테스트 자산 범위 등록**
목표 분해와 함께 「작업 목표 / 작업 설명」에서 **명시된 테스트 자산 범위**를 찾아 add_task_scope로 등록합니다. 이는 작업의 허가 경계이며 자산 테스트 커버리지의 분모입니다. **최소 범위 원칙: 사용자가 지정한 대상만 등록하고 임의로 넓히지 않습니다.**
- 대상이 URL이나 호스트명 주소(예: https://xxx.example.com/path, app.example.com)이면 **전체 호스트명**을 사용하고 kind=subdomain, value=전체 호스트명으로 등록합니다.
  예: https://a1b2c3.lab.example.net/path → kind=subdomain, value=a1b2c3.lab.example.net(**example.net이 아님**).
  하위 도메인을 포함한 호스트명을 루트 도메인으로 줄이지 않습니다. xxx.example.com 대신 example.com 전체를 등록하면 사용자 허가 범위를 벗어납니다.
- 사용자가 **하위 도메인이 없는 루트 도메인 자체**(예: example.com)를 주거나 「전체 사이트 / 모든 하위 도메인 / 전체 도메인」을 명시했을 때만 kind=root_domain, value=example.com을 사용합니다.
- IP 주소나 네트워크 대역이면 kind=ip / cidr, value=IP 또는 CIDR로 등록합니다.
- 회사 범위(company)는 등록하지 않습니다. 새 작업에서는 자산 시스템에 회사가 아직 없을 수 있으므로 회사 범위는 이후 계획 단계에서 처리합니다.
추가 규칙:
- 목표나 설명에 **명시된 범위만** 등록하며 언급되지 않은 도메인/IP를 추론하거나 만들어 내지 않습니다.
- reason에 근거가 된 문장을 간략히 설명하여 감사할 수 있게 합니다.
- 명확한 자산 범위가 없다면 add_task_scope를 호출하지 않습니다.
범위가 있으면 먼저 add_task_scope로 등록한 뒤 set_goals로 목표를 제출합니다.`

// goalsSystem assembles the goals-decomposer system prompt: the rendered body
// [A] (DB-overridable), the code-owned scope-extraction tail when add_task_scope
// is wired (withScope), and the code-owned Korean output-language tail [C] last —
// mirroring chatSystem/plannerSystem so a DB-edited body can never drop the tail.
// DecomposeGoalsWithProvider and the localization test share this one assembly, so
// the langDirective tail can't drift between runtime and test. EngagementDescription
// is intentionally left empty: the task description rides in the user message, not
// the {{.EngagementDescription}} var (see DecomposeGoalsWithProvider).
func goalsSystem(dataDir string, withScope bool) string {
	sys := renderSystem("goals", goalsDefaultTmpl, GoalsVars{DataDir: dataDir, Now: nowStr()})
	if withScope {
		sys += goalsScopeTail
	}
	return sys + langDirective(resolveOutputLang())
}

// GoalSpec is one decomposed objective.
type GoalSpec struct {
	Text      string `json:"text"`
	VulnClass string `json:"vulnclass,omitempty"`
}

// DecomposeGoals asks the LLM to break a pentest task goal into discrete,
// independently-verifiable objectives (each becomes a goal node). Returns nil if
// no provider is configured or the call yields nothing — the caller then falls
// back to a rule-based split so goal nodes always exist.
//
// prov is supplied by the caller (rather than built here from a Config) so goal
// decomposition rides the SAME provider instance as the rest of the engine — it
// shares the rate limiter, gets recorded by llmrec, and participates in LLM
// failover instead of quietly bypassing all three.
//
// desc is the task's free-text description (背景：靶标范围/flag 数量/交战说明等).
// It is fed alongside the goal so the decomposer no longer splits blind — the
// prompt still forbids inventing anything the two texts don't state.
//
// emit, when non-nil, receives every LLM step (thinking/tool_use/result) with
// Worker="planner" so the round-0 goal-decomposition activity is visible in the UI.
//
// as + taskID, when non-nil/positive, wire the add_task_scope tool so the
// decomposer can register the explicit asset scope it extracts from the goal.
//
// ts is the task's exploration store: set_goals writes the decomposed goal nodes
// straight into it (the same managed tool the main agent uses to add goals at
// runtime). The returned specs are read back from the store so callers can emit
// per-goal activity and detect the "LLM produced nothing" case for their fallback.
func DecomposeGoals(ctx context.Context, prov llm.Provider, dataDir, goalText, desc string, as *db.AssetStore, ts *db.ExplorationStore, taskID int64, emit func(db.Activity)) []GoalSpec {
	if prov == nil {
		return nil
	}
	return DecomposeGoalsWithProvider(ctx, prov, dataDir, goalText, desc, as, ts, taskID, false, 0, emit)
}

// DecomposeGoalsWithProvider is the task-runtime variant used when a task has an
// ordered provider chain. It preserves the same tools and write behavior while
// letting the caller own provider selection/failover. maxTokens is the profile's
// per-reply output cap (0 = send none).
func DecomposeGoalsWithProvider(ctx context.Context, prov llm.Provider, dataDir, goalText, desc string, as *db.AssetStore, ts *db.ExplorationStore, taskID int64, nonStreaming bool, maxTokens int, emit func(db.Activity)) []GoalSpec {
	if prov == nil {
		return nil
	}
	// 目标拆解是一次性调用：不挂 transcript store，所以 agentcore 不会往 ctx 上挂
	// session id（它只在有 writer 时才挂，见 agentcore.Prompt）。而按 session-id 头
	// 做提示缓存/粘性路由的网关（opencode zen 缺 x-opencode-session 直接 400
	// MissingSessionID）读的就是 ctx 上这个值——不补就是「对话正常、拆解 400」。
	// 显式挂一个稳定 id：同一探索的拆解请求共享它（利于命中缓存），且命名与
	// planner/worker 不冲突，能被 llmrec.parseSession 正确归因。
	if ts != nil {
		ctx = transcript.WithSessionID(ctx, fmt.Sprintf("exp%d-goals", ts.ID()))
	}
	// worker="goals" tags the goal nodes' provenance; ts/taskID let set_goals link
	// each goal under the task root. This is the catalog's real set_goals tool, so a
	// web-edited description/schema on it applies here too.
	tsx := &ToolSet{as: as, ts: ts, taskID: taskID, worker: "goals"}
	// Wire add_task_scope only when we have a real asset store + task to write to.
	// goalsSystem appends the scope-extraction tail in lockstep (withScope) so the
	// prompt never asks for a tool that isn't present, and it owns the output-language
	// tail last so a DB-edited body can't drop it. Description rides in the user
	// message, NOT the {{.EngagementDescription}} var, so a prompt can't inject it twice.
	withScope := as != nil && taskID > 0
	sys := goalsSystem(dataDir, withScope)
	// set_constraints 始终可用(不依赖 asset store):正文已含「先抽操作约束再拆目标」这步
	// (可在 agent 编辑页改措辞),这里只需接上工具。
	tools := []actool.CoreTool{tsx.setGoals(), tsx.setConstraints()}
	if withScope {
		tools = append(tools, tsx.addTaskScope())
	}
	userMsg := "작업 목표: " + goalText
	if d := strings.TrimSpace(desc); d != "" {
		userMsg += "\n\n작업 설명(대상 범위/flag 수/테스트 조건 등의 배경 정보이며 참고용입니다. 언급되지 않은 내용을 만들어 내지 않습니다):\n" + d
	}
	// Use captureRun so every LLM step is emitted as an activity record (visible in
	// the plan tab under the round-0 marker). Falls back gracefully when emit is nil.
	captureEmit := func(r db.Activity) {
		if emit != nil {
			r.Worker = "planner"
			emit(r)
		}
	}
	captureRun(ctx, agentcore.Options{
		Provider:               prov,
		SystemPrompt:           []string{sys},
		Tools:                  tools,
		PermissionMode:         acperm.ModeBypass,
		DisableBackgroundTasks: true,
		// 3 步(抽约束 → 登记范围 → 拆目标)各需一次工具调用,给足回合避免收尾前漏调 set_goals。
		MaxTurns:     8,
		NonStreaming: nonStreaming, // 该 profile 选非流式时走 Provider.Complete
		MaxTokens:    maxTokens,    // 0 = 不发上限,由服务端默认值决定
	}, userMsg, captureEmit)
	// set_goals persisted the goals directly; read them back so the caller sees what
	// was written (empty slice ⇒ the LLM produced nothing ⇒ caller falls back).
	if ts == nil {
		return nil
	}
	nodes, _ := ts.ListByKind(db.KindGoal, 10000)
	var out []GoalSpec
	for _, n := range nodes {
		var p struct {
			Text      string `json:"text"`
			VulnClass string `json:"vulnclass"`
		}
		_ = json.Unmarshal(n.Payload, &p)
		if strings.TrimSpace(p.Text) != "" {
			out = append(out, GoalSpec{Text: p.Text, VulnClass: p.VulnClass})
		}
	}
	return out
}
