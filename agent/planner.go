package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/Autumn-27/artex/db"
	"github.com/Autumn-27/artex/intercept"
	"github.com/Autumn-27/norma/agentcore"
	"github.com/Autumn-27/norma/llm"
	"github.com/Autumn-27/norma/permission"
	actool "github.com/Autumn-27/norma/tool"
	"github.com/Autumn-27/norma/transcript"
)

// Planner is the event-driven LLM planner (docs §4.3): each time the asset or
// exploration graph changes (debounced), it reads the exploration route, queries
// assets, judges whether the task goal is met, and emits 0..N exploration intents
// into the frontier. It is the sole intent generator.
type Planner struct {
	findingRecorder   FindingRecorder
	prov              llm.Provider
	model             string
	tx                *transcript.Store                      // raw LLM conversation persistence (nil = off)
	window            int                                    // context window in tokens (for compaction)
	windowFn          func() int                             // optional dynamic task-chain minimum
	maxTurns          int                                    // max agent turns per run (0 = unlimited)
	killWork          func(intentID int64) error             // engine callback to terminate a running work (nil = off)
	steerWork         func(intentID int64, msg string) error // engine callback to steer a running work mid-run (nil = off)
	proxyAddr         string                                 // recording proxy for WebFetch (empty = direct)
	proxyCACert       string                                 // recording proxy's CA cert path (HTTPS verify)
	webSearch         WebSearchOpts                          // web_search tool backend selection (off by default)
	workDir           string                                 // shared work dir (surfaced in prompt as artifact-output target)
	injectConstraints func() bool                            // resolver: inject task operation constraints into system prompt? (nil = yes)
	nonStreamingFn    func() bool                            // resolver: use non-streaming (Complete) path? (nil = streaming)
	noaEnabledFn      func() bool                            // resolver: use experimental noa compaction? (nil = off)
	maxTokensFn       func() int                             // resolver: per-reply output cap (nil/0 = send no cap)
	compactor         *Compactor                             // cold-node compaction (§7); nil = disabled

	// todos keeps ONE plan-scratchpad per task (keyed by exploration id) so the
	// planner's multi-step plan survives across wake-ups — each Plan() is a fresh
	// session, but the shared store lets it record a serial exploit chain once and
	// dispatch it step-by-step over rounds instead of front-loading it in parallel.
	todoMu sync.Mutex
	todos  map[int64]*actool.TodoStore
}

func NewPlanner(prov llm.Provider, model, workDir string, tx *transcript.Store, window, maxTurns int) *Planner {
	return &Planner{prov: prov, model: model, workDir: workDir, tx: tx, window: window, maxTurns: maxTurns, todos: map[int64]*actool.TodoStore{}}
}

func (p *Planner) SetCompactionWindowResolver(fn func() int) { p.windowFn = fn }

// SetCompactor wires the cold-node compactor (cold-digest §7). Called each
// planner wake-up to advance the round counter, maintain cold stamps, and
// (off the hot path) fold cold nodes into digests. nil = feature disabled.
func (p *Planner) SetCompactor(c *Compactor) { p.compactor = c }

// SetNonStreaming wires a resolver deciding whether runs use the non-streaming
// model path (true = non-streaming). nil/unset = streaming (default).
func (p *Planner) SetNonStreaming(fn func() bool) { p.nonStreamingFn = fn }

func (p *Planner) nonStreaming() bool { return p.nonStreamingFn != nil && p.nonStreamingFn() }

// SetNoaEnabled wires a resolver deciding whether runs use the experimental noa
// context-compression mechanism. nil/unset = off (built-in compaction). Read per
// run so the settings toggle takes effect without rebuilding the agent.
func (p *Planner) SetNoaEnabled(fn func() bool) { p.noaEnabledFn = fn }

// SetMaxTokens wires a resolver for the per-reply output cap. nil/unset or 0 =
// send no cap and let the endpoint decide. Read per run, like nonStreaming.
func (p *Planner) SetMaxTokens(fn func() int) { p.maxTokensFn = fn }

func (p *Planner) maxTokens() int {
	if p.maxTokensFn == nil {
		return 0
	}
	return p.maxTokensFn()
}

func (p *Planner) compactionWindow() int {
	if p.windowFn != nil {
		return p.windowFn()
	}
	return p.window
}

// SetProxy points the planner's WebFetch at the recording proxy plus the CA cert
// it trusts to verify HTTPS through it (empty addr = direct).
func (p *Planner) SetProxy(addr, caCert string) { p.proxyAddr, p.proxyCACert = addr, caCert }

// SetWebSearch selects the web_search backend for the planner (off by default).
func (p *Planner) SetWebSearch(o WebSearchOpts) { p.webSearch = o }

// SetConstraintInject wires a resolver deciding whether this task's operation
// constraints get injected into the planner system prompt. Read per round so the
// settings toggle takes effect without rebuilding the agent. nil = inject (default).
func (p *Planner) SetConstraintInject(fn func() bool) { p.injectConstraints = fn }

// wantConstraints reports whether constraint injection is enabled (default yes).
func (p *Planner) wantConstraints() bool { return p.injectConstraints == nil || p.injectConstraints() }

// todoFor returns the task's persistent planning todo store, creating it on first
// use. Shared across all of this task's planner wake-ups.
func (p *Planner) todoFor(expID int64) *actool.TodoStore {
	p.todoMu.Lock()
	defer p.todoMu.Unlock()
	s := p.todos[expID]
	if s == nil {
		s = actool.NewTodoStore()
		p.todos[expID] = s
	}
	return s
}

// SetKillWork wires the engine's per-work terminate callback so the planner's
// kill_work tool can stop a single running worker.
func (p *Planner) SetKillWork(fn func(intentID int64) error) { p.killWork = fn }

// SetSteerWork wires the engine's per-work steering callback so the planner's
// steer_work tool can inject a mid-run course-correction into a running worker.
func (p *Planner) SetSteerWork(fn func(intentID int64, msg string) error) { p.steerWork = fn }

// renderPlannerTodos formats the persistent planning todo for injection into the
// wake-up prompt (empty when there are no todos yet — first wake-up).
func renderPlannerTodos(items []actool.Todo) string {
	if len(items) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("\n\n【계획 할 일(이전 라운드에서 작성했으며 호출 사이에 유지됨)】:\n")
	for _, it := range items {
		mark := map[actool.TodoStatus]string{actool.TodoPending: "☐", actool.TodoInProgress: "▶", actool.TodoCompleted: "✔"}[it.Status]
		if mark == "" {
			mark = "☐"
		}
		b.WriteString(fmt.Sprintf("  %s %s\n", mark, it.Content))
	}
	b.WriteString("선행 단계가 완료되었거나 필요한 fact가 존재하는 다음 단계만 파견합니다. TodoWrite로 목록을 갱신하고 fact로 충족된 단계는 completed로 표시합니다. 이미 pending/in_progress인 단계를 중복 파견하지 않습니다.")
	return b.String()
}

// TriggerEvent describes what concretely caused this planning round to fire, so
// the planner looks first at the actual change instead of re-scanning the whole
// overview. Kind:
//
//	"done"    — a worker finished intent IntentID (its output conclusion is fetched).
//	"finding" — a worker reported a finding on intent IntentID (Detail = 摘要).
//	"goal"    — the human (via 主 agent 的 set_goals) added one OR MORE goals in a
//	            single call (Goals = 本次新增的目标文本，1+ 条；set_goals 支持批量).
//	"goal_deleted" — the human deleted a goal from 总览的目标管理 (Detail = 被删目标文本).
//	"goal_edited"  — the human edited a goal from 总览的目标管理 (OldGoal→NewGoal 文本).
//	"cancelled" — the human deleted intent IntentID (Detail = 删除原因). The intent is
//	            stopped (not deleted) and the reason is attached to it as a fact.
type TriggerEvent struct {
	Kind     string
	IntentID int64
	Detail   string
	Summary  string   // Kind=="cancelled" 专用：删除前捕获的意图摘要（真删除后节点已不存在，无法再查）
	Goals    []string // Kind=="goal" 专用：本次 set_goals 新增的目标文本（1 条或多条）
	OldGoal  string   // Kind=="goal_edited" 专用：修改前的目标文本
	NewGoal  string   // Kind=="goal_edited" 专用：修改后的目标文本
	Hints    []string // Kind=="hint" 专用：本次 add_hint 新增的提示文本（1 条或多条）
}

// renderTriggers spells out the change(s) that fired this round: for a finished
// worker — which intent + its output conclusion; for a finding — which intent +
// what was found. Empty for time/heartbeat wakes. Reads the store (best-effort;
// a blank field never blocks the round).
func renderTriggers(ts *db.ExplorationStore, evs []TriggerEvent) string {
	if len(evs) == 0 || ts == nil {
		return ""
	}
	var b strings.Builder
	b.WriteString("\n\n【이번 라운드를 호출한 실제 변경 사항(먼저 읽고 방향 추가 여부 판단)】:")
	for _, ev := range evs {
		switch ev.Kind {
		case "goal":
			if len(ev.Goals) == 1 {
				b.WriteString(fmt.Sprintf("\n- 사용자(주 에이전트)가 목표를 추가했습니다: %s — 아직 대응 의도가 없으면 새 목표에 필요한 탐색 방향을 추가합니다.", ev.Goals[0]))
			} else {
				b.WriteString(fmt.Sprintf("\n- 사용자(주 에이전트)가 목표 %d개를 추가했습니다: %s — 대응 의도가 없는 목표별로 탐색 방향을 추가합니다.", len(ev.Goals), strings.Join(ev.Goals, "；")))
			}
		case "hint":
			if len(ev.Hints) == 1 {
				b.WriteString(fmt.Sprintf("\n- 사용자(주 에이전트)가 전략 힌트를 추가했습니다: %s — 그래프에 등록되어 있으며 대응 의도가 없다면 이를 반영해 방향을 조정·추가합니다.", ev.Hints[0]))
			} else {
				b.WriteString(fmt.Sprintf("\n- 사용자(주 에이전트)가 전략 힌트 %d개를 추가했습니다: %s — 그래프에 등록되어 있으므로 각각 방향을 조정·추가합니다.", len(ev.Hints), strings.Join(ev.Hints, "；")))
			}
		case "goal_deleted":
			b.WriteString(fmt.Sprintf("\n- 사용자가 목표를 삭제했습니다: %s — 남은 목표와 방향을 다시 판단하며 삭제한 목표에는 의도를 파견하지 않습니다.", ev.Detail))
		case "goal_edited":
			b.WriteString(fmt.Sprintf("\n- 사용자가 목표를 「%s」에서 「%s」로 변경했습니다. 새 목표에 맞춰 조정하고 더 이상 유효하지 않은 방향은 파견하지 않습니다.", ev.OldGoal, ev.NewGoal))
		case "finding":
			b.WriteString(fmt.Sprintf("\n- 의도 #%d(%s)의 worker가 finding을 보고했습니다: %s", ev.IntentID, intentSummary(ts, ev.IntentID), ev.Detail))
		case "cancelled":
			// 意图内容优先用删除时捕获的 Summary（真删除后节点已不存在，intentSummary 查不到）。
			sm := ev.Summary
			if sm == "" {
				sm = intentSummary(ts, ev.IntentID)
			}
			b.WriteString(fmt.Sprintf("\n- 사용자가 의도 #%d를 삭제했습니다. 내용: %s, 삭제 이유: %s. 더 이상 실행하지 않으므로 이를 반영해 다시 계획합니다.", ev.IntentID, sm, ev.Detail))
		default: // "done"
			b.WriteString(fmt.Sprintf("\n- 의도 #%d(%s)의 worker가 종료했습니다. 결과: %s", ev.IntentID, intentSummary(ts, ev.IntentID), workerOutput(ts, ev.IntentID)))
			if fids := factIDsYielded(ts, ev.IntentID); fids != "" {
				b.WriteString(fmt.Sprintf("; 이번 의도에서 생성한 사실 id: %s ", fids))
			}
		}
	}
	b.WriteString("\n(상세 내용은 node_detail / get_worker_output / list_findings로 확인합니다.)")
	return b.String()
}

// factIDsYielded lists the fact ids an intent produced this run as "#12、#15", so the
// planner can jump straight to the round's incremental facts. Empty (best-effort) when
// the intent yielded no facts or the lookup fails.
func factIDsYielded(ts *db.ExplorationStore, id int64) string {
	ids, err := ts.FactsYielded(id)
	if err != nil || len(ids) == 0 {
		return ""
	}
	parts := make([]string, len(ids))
	for i, fid := range ids {
		parts[i] = fmt.Sprintf("#%d", fid)
	}
	return strings.Join(parts, "、")
}

// intentSummary reads an intent node's one-line summary (best-effort, "?" on miss).
func intentSummary(ts *db.ExplorationStore, id int64) string {
	n, err := ts.GetNode(id)
	if err != nil || n == nil {
		return "?"
	}
	var p map[string]any
	if json.Unmarshal(n.Payload, &p) == nil {
		if s, ok := p["summary"].(string); ok && s != "" {
			return s
		}
	}
	return "?"
}

// workerOutput returns the finished worker's conclusion for an intent — the last
// 'result' (else 'text') activity's full detail, truncated. Same source get_worker_output uses.
func workerOutput(ts *db.ExplorationStore, id int64) string {
	acts, _, err := ts.ActivityList(&id, 0, 1000)
	if err != nil {
		return "(출력을 가져오지 못함)"
	}
	var pick *db.Activity
	for i := range acts {
		if acts[i].Kind == "result" {
			pick = &acts[i]
		} else if acts[i].Kind == "text" && pick == nil {
			pick = &acts[i]
		}
	}
	if pick == nil {
		return "(아직 work 출력 기록이 없음)"
	}
	out, _ := ts.ActivityDetail(pick.ID)
	if out == "" {
		out = pick.Summary
	}
	return truncOutput(out, 800)
}

// truncOutput caps a worker-output blob so the trigger context doesn't bloat the
// system prompt every round; full text is one get_worker_output call away.
func truncOutput(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + " …(일부 생략, 전체 내용은 get_worker_output 참고)"
}

// renderGraphOverview folds the pre-computed graph_overview snapshot into the
// wake-up prompt so the planner starts each round with the full situation in
// hand — saving the round-trip it would otherwise spend calling the tool. It is
// the exact same JSON graph_overview would return; deeper detail is still one
// tool call away (node_detail / list_facts / …).
func renderGraphOverview(data map[string]any) string {
	b, err := json.Marshal(data)
	if err != nil {
		return "" // fall back to the model calling graph_overview itself
	}
	return "\n\n【현재 상황(graph_overview 사전 조회 결과이며 도구 반환값과 동일함. 자세한 내용이 필요할 때만 node_detail/list_facts 등을 호출)】:\n" + string(b)
}

// plannerDefaultTmpl is the built-in EDITABLE body (段 [A]) of the planner prompt,
// seeded into agent_prompts. Goal is a {{.Goal}} template var; the 中间产物输出规约
// tail is code-owned (artifactSpec) and appended by plannerSystem after rendering.
const plannerDefaultTmpl = `당신은 보안 플랫폼의 허가된 침투 테스트 시스템에서 "계획자"입니다. 그래프가 바뀔 때마다 자주 호출됩니다. 임무는 상황 파악 → 목표 판정 → **아직 다루지 않은 새 방향이 있을 때만** 탐색 의도 추가입니다. 실행자가 아니므로 이번 라운드의 산출물은 **의도 생성·구체화 또는 목표 판정**이어야 하며 plan 안에서 직접 작업을 수행하지 않습니다.

작업 목표: {{.Goal}}

**이번 라운드에서 생성할 의도 수(먼저 판단)**:
- **최우선 필수 조건**: 목표가 미달성이고 open/running 의도가 하나도 없으면(frontier_open=0이며 running_intents가 비어 있음) 목표를 향한 의도를 **최소 하나** 생성합니다. 실행 중이거나 대기 중인 작업이 없을 때 0개를 생성하면 작업이 멈춥니다. 알려진 방향이 모두 recent_done에만 있어도 아래 done/exhausted/blocked 기준에 따라 새로운 방향이나 후속 의도를 등록합니다.
- 이 조건 외에는 **의도 0개도 정상적인 결과지만 정당한 이유가 있어야 합니다.** ① 이미 open/running 의도가 해당 방향을 처리 중인 경우(표현만 바꾼 중복 생성은 심각한 오류), ② 다음 단계가 실행 중인 work의 아직 나오지 않은 결과에 의존하는 경우(선행 결과 없이 파견하면 빈 작업이 반복되므로 다음 그래프 갱신을 기다림).
- 아직 다루지 않았고 실행 중인 work에 의존하지 않는 새 방향이 있거나 목표 미달성 상태에서 허가 범위 안의 미검증 영역이 있으면 파견합니다. 0개를 기본 선택으로 삼지 않습니다.

**호출될 때마다 따르는 판단 절차**:

1. **전체 상황이 이 프롬프트 아래에 첨부되어 있습니다.** graph_overview 반환값이므로 다시 호출할 필요가 없습니다. task(원래 제목+목표/루트 노드), 자산 수, goals와 상태, open/running/recent_done 의도, sites_without_endpoints(엔드포인트 없는 사이트), facts(탐색 사실 수이며 취약점과 다름), recent_facts({id,summary,confidence?})가 포함됩니다.
   - **범위**: 탐색 노드(goals/의도/facts/findings)는 이 작업만 포함합니다. **자산 그래프는 모든 작업이 공유**하며 자산 수는 허가 범위 안의 전역 수입니다. 이 작업에 해당하지 않는 자산은 무시합니다.
   - **연결 관계**: 의도마다 parents(어떤 사실/의도에서 파생됐는지)와 yields(어떤 사실/발견을 만들었는지)가 있고 recent_facts마다 from_intent가 있습니다. 이를 통해 사실의 출처와 새로운 방향을 종합할 가능성을 파악합니다.
   - **부정적이거나 불확실한 관찰**(「포트 닫힘/주입 불가」 등)은 worker의 관찰이며 확정 결론이 아닙니다. node_detail(id)로 evidence를 먼저 확인합니다. 증거가 확실하고 confidence=observed이며 필요한 수단을 검토한 경우에만 일단 막힌 방향으로 봅니다. 증거가 없거나 한 번만 시도했거나 confidence=inferred이면 **아직 확인되지 않은 상태**로 보고, 허가 범위 안이며 다른 의도가 다루지 않는 경우 재확인 의도를 파견합니다. **동일한 부정적 방향은 최대 한 번만 재확인**하며 다시 부정적인 결과가 나오고 증거가 타당하면 결론을 존중합니다.
   - **더 자세한 정보가 필요할 때만** list_facts(최신순, 기본 20개, q 필터와 before 페이지, total/has_more), list_findings(전체 취약점), node_detail(id)(전체 증거/상세), list_assets(q 검색, type/company_id/task_id 필터, 페이지 또는 id/ids 직접 조회), asset_neighbors를 사용합니다. 공유 자산 전체를 기본으로 불러오지 않습니다.

2. **목표 판정(핵심 임무)**: goals에 목표와 상태가 포함됩니다. 발견이나 사실로 미달성 목표가 증명되면 prove_goal(goal_id, evidence_id, reason)으로 met 표시합니다. **마지막 미달성 목표가 met가 되면 시스템이 작업 전체를 자동 완료**합니다. 완료는 개별 prove_goal 호출로만 결정되며 별도의 일괄 완료 수단은 없습니다.
   - **정량 수용 조건 확인**: 커버리지 X%, flag N개, 특정 권한 획득처럼 측정 가능한 조건은 prove_goal 전에 graph_overview의 실제 값(coverage.pct, findings_total 등)으로 반드시 확인합니다. 미달이면 prove_goal을 호출하지 말고 부족한 부분을 검증할 의도를 파견합니다. 「대체로 달성/핵심은 확보」를 이유로 조기 met 처리하지 않습니다. 예: 요구 100%, 실측 coverage.pct=40%이면 미달성입니다.

3. **선택 사항: 시작 시 아주 가벼운 확인**: fact가 거의 없고(recent_facts가 비어 있으며 작업 시작 단계) 상황 정보만으로 초기 의도를 구체화할 수 없을 때만 Bash 등으로 극소수의 읽기 전용 확인을 합니다(예: 홈페이지/지문 확인 curl 1~2회). **허용되는 산출물은 더 구체적인 의도 설명 한 문장뿐**입니다. 취약점 발견·검증·이용, 엔드포인트/디렉터리/매개변수 열거는 worker에게 의도로 전달합니다.
   - worker가 만든 fact가 있으면(facts>0 또는 recent_facts가 비어 있지 않음) 직접 탐지하지 않습니다. 기존 fact로 판단하고 새 의도를 파견하거나 종료합니다. 자세히 확인할 단서는 worker에게 맡깁니다.
   - 시작 단계라도 최대 3회만 확인합니다. 엔드포인트·디렉터리·id 열거, 디코딩 체인, 같은 인터페이스 반복 확인, 주입·권한 위반·취약점 검증 등 깊은 조사로 넘어가면 즉시 멈추고 의도로 작성합니다.
   - 기존 사실과 상황으로 판단할 수 있다면 탐지할 필요가 없습니다.

4. **추가할 방향 선택**: 여기서 절제는 **기존 의도의 중복 방지**를 뜻하며 가능한 한 적게 파견하라는 뜻이 아닙니다. 목표가 미달성이면 아직 다루지 않은 더 깊은 접근을 검토합니다. 의도는 정해진 유형이나 메뉴가 아닌 **열린 탐색 방향**이며 사실·자산·목표를 종합해 open + running + recent_done과 비교합니다.
   - open/running이 이미 다루면 추가하지 않습니다.
   - recent_done에 있으면 **state로 종료 경위를 구분**합니다.
     · **done(정상 완료)**: 같은 내용을 다시 파견하지 않습니다. 막힌 방향인지 여부는 state가 아니라 yields의 fact로 판단합니다. **실질적인 새 기법이나 근거**(새 사실/자산/매개변수/명백히 다른 방식)가 있을 때만 재파견하며 summary에 차이를 설명합니다. 표현 변경이나 막연한 재시도는 근거가 아닙니다.
     · **exhausted(예산 소진으로 중단, 일부만 기록) / blocked(모델·네트워크 오류로 거의 수행하지 못함)**: get_worker_trace / get_worker_output으로 실제 진행과 장애 지점을 확인합니다. 돌파 직전에 예산이 끝났으면 이어서 진행하고, 외부 오류로 수행하지 못했으면 같은 방향을 다시 파견하며, 같은 지점에서 계속 막히면 방식이나 방향을 바꿉니다. 판단 근거는 state 자체가 아니라 실제 trace입니다.
   - 어떤 의도도 다루지 않는 새 방향은 생성합니다.
   - 모든 방향을 open/running이 다루면 추가하지 않고 종료하여 진행을 기다립니다. recent_done만 있고 open/running이 없으며 목표가 미달성이면 최상단 필수 조건에 따라 새로운 방향이나 후속 의도를 생성합니다.
   - **커버리지보다 깊이 우선**: coverage는 하한·수용 조건이지 탐색 목적 자체가 아닙니다. 가치가 높은 진입점(RCE/권한 상승/데이터 유출로 이어질 가능성)을 발견하면 자산마다 얕은 테스트로 커버리지만 맞추기보다 해당 경로를 깊이 검증할 의도를 우선합니다.
   - **경로의 다양성 유지**: 목표가 미달성이고 현재 의도가 한 경로에 몰려 있으며 본질적으로 다른 미검증 방향(다른 진입면/자산 유형/이용 체인)이 있으면 그 방향을 추가합니다. 표현이 아니라 실질적인 차이로 판단하고 이미 다루는 방향은 중복하지 않습니다. 서로 다른 원리의 경로 2~3개를 유지하고 목표에 가까워지는 증거가 나온 뒤 집중합니다. **다양성도 작업 제약보다 우선하지 않습니다.** 제외된 진입점·포트·호스트·작업은 의도로 생성하지 않습니다.

   **직렬 이용 체인은 단계별로 파견합니다.** ①→②→③에서 다음 단계가 앞 단계의 실제 결과에 의존하면 동시에 파견하지 않습니다. TodoWrite에 단계별로 기록하고 이번 라운드에서는 선행 조건이 충족된 단계만 파견합니다. fact가 나온 뒤 다음 호출의 할 일 목록을 보고 이어서 파견하고 완료 단계를 completed로 표시합니다. 「발생 지점 확인」과 「그 지점에서 발생시키기」처럼 같은 일을 두 의도로 나누지 않습니다. 서로 의존하지 않는 병렬 차원만 여러 의도로 동시에 파견합니다.

5. **제출**: add_intent **한 번**으로 새 방향을 일괄 제출합니다(intents 배열, 가치가 높은 최대 4개). 한 개씩 반복 호출하지 않습니다.
   - **summary**: 전체 대상 주소 + 수행할 일 + 이유를 한 문장으로 씁니다. 고정 분류를 적용하지 않으며 기존 의도와의 중복 판별에 사용됩니다.
   - **asset_ids**: 해당 방향의 대상 자산 id(list_assets에서 확인한 0/1/여러 개). 구체적인 사이트·인터페이스·매개변수·호스트를 대상으로 한다면 반드시 전달하고, 여러 자산이면 모두 포함합니다. 구체적인 자산이 없는 전역 정찰일 때만 비웁니다.
   - **parent_ids**: 의도를 도출한 상위 노드 id(선택, 0/1/여러 개). 여러 사실을 종합했다면 모두 전달하고 상위 의도나 발견에서 파생됐다면 그 id도 전달합니다. 최상위 새 방향이면 비웁니다.

중복하거나 억지로 만들지 않습니다. 목표가 미달성이고 아직 다루지 않은 더 깊은 방향이 있으면 파견합니다. 간결하고 집중해서 판단합니다.`

func plannerSystem(goal, dataDir, workDir string) string {
	body := renderSystem("planner", plannerDefaultTmpl, PlannerVars{Goal: goal, DataDir: dataDir, Now: nowStr()})
	return body + artifactSpec(workDir) + langDirective(resolveOutputLang())
}

// Plan runs one planning round. emit, if non-nil, receives the planner's execution
// steps (so users can see how it reads the situation and judges goals — the
// planner is the intent generator and was previously a black box). Returns whether
// the planner judged the goal met.
// triggers carries the concrete change(s) that fired this round — worker(s) done
// and/or finding(s) reported (may be several — the engine debounces a burst; empty
// for time/heartbeat wakes). They are spelled out at the top of the prompt so the
// planner looks first at the actual change (which intent, its output/finding).
func (p *Planner) Plan(ctx context.Context, taskID int64, as *db.AssetStore, ts *db.ExplorationStore, goal string, triggers []TriggerEvent, emit func(db.Activity)) (met bool, reason string, err error) {
	// cold-digest §2.3/§7: advance this task's planner-round counter, maintain the
	// cold_since_round stamps, and (if a threshold is hit) kick off background
	// compaction. Synchronous part is cheap (a few queries); the LLM compaction
	// runs in a detached goroutine so it never adds latency to this round.
	p.compactor.OnPlannerRound(ctx, ts)
	tsx := NewToolSet(ts, "planner")
	tsx.SetFindingRecorder(p.findingRecorder)
	if as != nil {
		tsx.SetAssetStore(as, as.Companies())
	}
	tsx.SetTaskID(taskID)
	tsx.SetCoverageEnabled(as == nil || as.CoverageEnabled(taskID))
	tsx.killWork = p.killWork   // enable kill_work tool (nil = unavailable)
	tsx.steerWork = p.steerWork // enable steer_work tool (nil = unavailable)
	if origin, _ := ts.OriginFactID(); origin > 0 {
		tsx.SetOwnerNode(origin) // planner-side anchors default to the task root (origin fact)
	}
	// 领域工具 + 基础默认工具集（Read/Write/Edit/MultiEdit/LS/Glob/Grep/Bash）
	// 资产覆盖度功能关闭时剔除 add_task_scope/list_untested_assets（不入 prompt）。
	base := append(tsx.DropCoverageTools(tsx.PlannerTools()), actool.DefaultTools()...)
	ctx = WithRunInfo(ctx, RunInfo{TaskID: taskID, ExplorationID: explorationID(ts)})
	tools, def, cleanup := AugmentTools(ctx, "planner", base)
	defer cleanup()
	// 关键态势（刚完成的意图 + 预取的完整图）改放【本轮 user 输入】(见下方 input)，system
	// 只留静态规划正文。move-out 让 system 每轮稳定、更利于缓存；代价是若单轮变长，态势可能
	// 被 compaction 压缩（planner 单轮通常短，风险低）。situational 会拼进下方 input。
	situational := renderTriggers(ts, triggers) + renderGraphOverview(tsx.graphOverviewData())
	// 任务级 deadline / 终局模式(经 ctx 注入,见 taskclock.go)。终局那一轮把任务超时
	// planner 收尾词作为【本轮操作指令】拼进本轮 user 输入(随 situational),让它只做最后
	// 目标判定、不产新意图。
	tc := taskClockFrom(ctx)
	if tc.Final {
		situational += "\n\n【작업 최종 마무리(이번 라운드의 특별 지시이며 일반 계획 절차보다 우선)】:" + resolveTaskTimeoutWrapup("planner")
	}
	// 本任务的工作目录 <workDir>/tasks/<taskID>，先建好。
	taskDir := ensureRunDir(p.workDir, taskID, 0)
	ctx = intercept.WithReviewContext(ctx, taskDir, intercept.ReviewBackground{})
	sysBody := plannerSystem(goal, p.workDir, taskDir)
	if p.wantConstraints() {
		sysBody += constraintBlock(ts) // 操作约束(若有)注入系统提示,框定探索边界
	}
	system, boundary := deferredSystem(sysBody, def)
	// planner 无自身墙钟预算;有 deadline 时把 MaxDuration 夹逼到剩余,让在跑的规划轮在
	// 任务到点时进收尾(因超时→任务超时词,因步数→per-run 词)。
	maxDur, clamped := clampMaxDuration(tc.DeadlineUnix, 0)
	settle := wrapupSettlement("planner", nil)
	if tc.DeadlineUnix > 0 {
		settle = wrapupSettlementForTask("planner", nil, clamped)
	}
	opts := agentcore.Options{
		Provider:        p.prov,
		SystemPrompt:    system,
		DynamicBoundary: boundary,
		Tools:           tools,
		DeferredTools:   def.Deferred,
		UnlockSet:       def.Unlock,
		PermissionMode:  permission.ModeBypass,
		EnableWebFetch:  true, // 走记录代理留痕；载入代理 CA 验证 MITM 重签的 HTTPS 证书
		WebFetchProxy:   p.proxyAddr,
		WebFetchCACert:  p.proxyCACert,
		// 联网搜索(可选)。ddgs 无需 key；brave-free 需 BraveKey；tavily 需 TavilyKey。
		// WebSearchProxy 是独立出口代理(http/https/socks5)，与记录流量的 MITM 代理无关；空则直连。
		EnableWebSearch:       p.webSearch.Enabled,
		WebSearchBackend:      p.webSearch.Backend,
		BraveSearchAPIKey:     p.webSearch.BraveKey,
		TavilySearchAPIKey:    p.webSearch.TavilyKey,
		DeepSeekSearchBaseURL: p.webSearch.DeepSeekBaseURL,
		DeepSeekSearchAPIKey:  p.webSearch.DeepSeekAPIKey,
		DeepSeekSearchModel:   p.webSearch.DeepSeekModel,
		WebSearchProxy:        p.webSearch.Proxy,
		BashEnv:               proxyEnv(p.proxyAddr, p.proxyCACert), // Bash 子命令默认走代理+信任 CA
		WorkingDir:            taskDir,                              // 本任务工作目录 <workDir>/tasks/<taskID>
		ToolOutputDir:         cmdOutDir(taskDir),
		MaxTurns:              p.maxTurns, // 0 = unlimited (configurable in agent management)
		MaxDuration:           maxDur,     // 0=不限;有 deadline 时=距 deadline 剩余
		Compaction:            compactionConfig(p.compactionWindow()),
		// 跨唤醒共享的规划待办：让串行链在多轮之间保留（session 是新的，store 不是）。
		Todos: p.todoFor(ts.ID()),
		// 命中【本轮】步数预算→ SDK 跑收尾:把本轮已想清楚的结论落地(该派的 add_intent、
		// 能证的 prove_goal、串行链记 TodoWrite),而非停止规划——planner 之后仍会被反复唤醒。
		// clamped(被任务 deadline 夹逼)时改用 PromptByReason(见 wrapupSettlementForTask)。
		Settlement:   settle,
		NonStreaming: p.nonStreaming(), // 该 profile 选非流式时走 Provider.Complete
		MaxTokens:    p.maxTokens(),    // 0 = 不发上限,由服务端默认值决定
	}
	if p.tx != nil { // persist raw LLM conversation; one accumulating file per task's planner
		opts.Transcript = p.tx
		opts.SessionID = fmt.Sprintf("exp%d-planner", ts.ID())
	}
	// 实验功能:开启后由 noa 接管上下文压缩(归档集中在 <workDir>/noa/<SessionID> 下,持久)。
	noaSession := fmt.Sprintf("exp%d-planner", ts.ID())
	enableNoa(&opts, p.noaEnabledFn, p.workDir, noaSession, noaWarn(noaSession))
	// 态势（刚完成的意图 + 完整图）现在拼进本轮 user 输入（见下方 input）。user 里还有
	// 指令 + 跨唤醒待办（todo 是模型自己的规划便签，可再生，放 user 即可）。
	// 开场白按「本轮有无具体变动」分两种：有变动 → 指向下方【实际变动】块；无变动
	// (心跳定时巡检 / hint / 恢复等) → 别谎称"图发生了变化",转而提示顺带复查在跑意图。
	lead := "구체적인 변경이 발생했습니다(아래 실제 변경 사항 참고). 이를 근거로 다음 단계를 계획합니다:"
	if len(triggers) == 0 {
		lead = "이번 호출은 **정기 점검(주기 도달)/구체적인 변경 신호 없음**입니다. 그래프가 바뀌지 않았을 수 있습니다. 실행 의도를 살펴 오래 멈췄거나 벗어난 방향은 steer_work로 조정하고, 방향 전체가 잘못되었다면 kill_work로 중단합니다. 목표를 판정하고 추가 방향이 필요한지 판단합니다:"
		// 心跳/无变动唤醒时,若全图已无任何 open 或 running 意图 → 探索已停摆(没 worker 在跑、
		// 也没排队方向)。明确告知 planner 并强制其本轮补出新方向,别只复查在跑意图后空转一轮。
		if active, err := ts.HasActiveIntent(); err == nil && !active {
			lead = "이번 호출은 **정기 점검**이며 현재 **open/running 의도가 없습니다.** 실행 중인 worker도 대기 방향도 없어 탐색이 멈춘 상태입니다. 먼저 목표 달성 여부를 확인하고 미달성이면 기존 의도와 중복되지 않는 새 의도를 하나 이상 반드시 생성합니다. 미달성 상태에서 의도 0개로 끝내지 않습니다:"
		}
	}
	input := lead + situational + "\n\n위 상황으로 목표를 판정합니다. 실제 목표 결과나 확인된 취약점을 얻어 달성한 경우 prove_goal로 각각 표시합니다. **필수 조건: 목표가 미달성이며 open/running 의도가 없으면(frontier_open=0, running_intents가 비어 있음) 목표를 향한 의도를 최소 하나 생성합니다.** 기다릴 실행이나 대기 방향이 없을 때 의도 0개는 작업을 멈춥니다. 기존 open/running이 진행 중이거나 목표가 달성됐을 때만 새 의도 없이 끝낼 수 있습니다." +
		renderPlannerTodos(opts.Todos.List())
	// MaxDuration 现在会在墙钟到点打断在跑工具并就地进收尾(在活 ctx 上),单轮卡死不再
	// 绕过收尾,无需外部硬 ctx 兜底。ctx 只承载 pause / kill / shutdown。
	_, _, err = captureRun(ctx, opts, input,
		func(r db.Activity) {
			if emit != nil {
				r.Worker = "planner" // planner activity has no intent_id (it generates them)
				emit(r)
			}
		})
	return tsx.GoalMet, tsx.Reason, err
}
