package agent

import (
	"encoding/json"
	"fmt"

	"github.com/Autumn-27/artex/db"
	actool "github.com/Autumn-27/norma/tool"
)

const findingIDGuidance = "\n\n**취약점 번호 규칙**: finding_id는 독립 취약점 기록 ID, finding_node_id는 탐색 노드 ID입니다. list_findings / list_task_findings / node_detail / get_task_node_detail의 id는 탐색 노드 ID이며 독립 번호는 같은 반환값의 finding_id에서 읽습니다. get_finding_traffic / bind_finding_traffic은 독립 finding_id를 사용합니다. 기존 update_finding_report의 finding_id 매개변수에는 finding_node_id를 전달합니다. report_finding 첫 줄의 숫자를 증거 도구에 사용하거나 번호 오류 후 다른 숫자를 추측하지 않습니다."

// The server supplies the persisted setting. A missing setting/host is off.
// Consulted at assembly and again on writes so an already-running session
// cannot keep binding after the user switches the feature off.
var FindingTrafficBindingEnabled func() bool

func findingTrafficBindingEnabled() bool {
	return FindingTrafficBindingEnabled != nil && FindingTrafficBindingEnabled()
}

// Applied after ToolResolve: user descriptions and prompts remain intact, while
// all actual reporters (including Planner and custom chat agents) see the same
// API contract. Disabled/unbound tools are never reintroduced here.
func findingWorkflowTools(agentKey string, tools []actool.CoreTool) ([]actool.CoreTool, string) {
	if !findingTrafficBindingEnabled() {
		out := make([]actool.CoreTool, 0, len(tools))
		for _, tool := range tools {
			if tool.Name() == "bind_finding_traffic" {
				continue
			}
			if agentKey == "reporter" && (tool.Name() == "traffic_search" || tool.Name() == "traffic_get" || tool.Name() == "traffic_blob") {
				continue
			}
			switch tool.Name() {
			case "report_finding", "add_hint", "add_task_hint":
				// Work on a copy: toggling back on must restore the original schema.
				raw, _ := json.Marshal(tool.InputSchema())
				var schema map[string]any
				if json.Unmarshal(raw, &schema) == nil {
					stripTrafficParameters(schema)
					tool = DecorateTool(tool, tool.Description(), schema)
				}
			}
			out = append(out, tool)
		}
		return out, ""
	}
	out := append([]actool.CoreTool(nil), tools...)
	has := map[string]bool{}
	for i, tool := range out {
		has[tool.Name()] = true
		note := ""
		switch tool.Name() {
		case "report_finding":
			note = "\n기본적으로 보고서 에이전트가 작성 전에 트래픽을 확인하고 연결합니다. 보고자는 evidence에 검증 명령·핵심 출력·이미 확인한 실제 트래픽 ID와 용도를 보존하여 실행 기록과 대조할 수 있게 합니다. 연결만을 위한 추가 조회는 필요하지 않습니다. 즉시 연결하려면 확인한 traffic_refs 또는 이 작업의 hint에서 구조화 참조를 읽는 evidence_hint_id를 전달할 수 있습니다. 하나라도 잘못되면 보고 전체가 실패합니다. TCP/기록 없음이면 선택 매개변수가 필요하지 않습니다. 반환 finding_id와 finding_node_id는 각각 독립 기록과 탐색 노드입니다."
		case "add_hint", "add_task_hint":
			note = "\n확인된 취약점을 인계할 때 해당 힌트의 traffic_refs에 검증된 트래픽 ID·용도·설명·순서를 보존합니다(단일 항목은 최상위, 일괄 입력은 각 hints 요소). text에는 어떤 취약점을 증명하는지 설명합니다. 글만 전달하고 기존 트래픽 참조를 버리지 않으며 확인하지 않은 후보는 증거로 전달하지 않습니다."
		case "get_finding_traffic", "bind_finding_traffic", "list_findings", "list_task_findings", "node_detail", "get_task_node_detail", "update_finding_report":
			note = findingIDGuidance
		}
		if note != "" {
			out[i] = DecorateTool(tool, tool.Description()+note, tool.InputSchema())
		}
	}
	guidance := ""
	if has["report_finding"] || has["add_task_hint"] || has["add_hint"] {
		guidance = "\n\n**트래픽 증거 인계(선택 사항)**: 기본 자동 연결은 취약점 저장 후 보고서 작성 전에 보고서 에이전트가 수행합니다. 보고자는 evidence에 검증 명령·핵심 출력·기존 실제 트래픽 ID와 용도를 보존하고 작업에서는 intent_id를 포함하여 추적할 수 있게 합니다. 연결을 위해 추가로 기록을 찾을 필요는 없습니다. Auto / Planner가 대신 보고해도 실행자의 참조를 보존합니다. add_hint / add_task_hint는 traffic_refs로 인계하며 report_finding의 traffic_refs / evidence_hint_id 즉시 연결도 지원합니다. TCP나 기록 없음도 정상 등록하고 ID 추측이나 보충 목적의 재탐지는 하지 않습니다."
		if has["add_task_hint"] && !has["add_hint"] {
			guidance += "\n플랫폼 대화에 작업 맥락이 없으면 report_finding을 직접 호출하지 않습니다. add_task_hint로 해당 기존 작업에 인계하여 작업 에이전트가 등록하도록 하고 list_task_findings로 결과를 확인합니다."
		}
		if has["prove_goal"] || has["goal_met"] {
			guidance += "\n목표 완료 판정 전에 확보한 증거의 보고·인계를 마칩니다. 인계가 끝나지 않았는데 글로 취약점을 등록했다는 이유만으로 작업을 종료하거나 Worker를 취소하지 않습니다. 트래픽이 없으면 기다리거나 억지로 수집할 필요는 없습니다."
		}
	}
	if has["update_finding_report"] && has["bind_finding_traffic"] && has["get_finding_traffic"] {
		guidance += "\n\n**보고서 작성 전 트래픽 자동 연결(활성)**: 이번 취약점의 트래픽을 확인·연결한 뒤 보고서를 작성합니다. report_finding 반환 JSON 또는 get_task_node_detail / list_task_findings에서 명확한 finding_id와 finding_node_id를 읽습니다. 취약점 상세·해당 의도 실행 기록·기존 증거 목록을 확인하고 보고자가 넘긴 실제 ID를 우선 사용합니다. HTTP 검증이고 트래픽 도구가 있으면 traffic_search로 후보를 고른 뒤 traffic_get으로 요청/응답이 해당 취약점을 뒷받침하는지 각각 확인합니다. 도메인과 시각만으로 소속을 판단하지 않습니다. 확인된 증거를 재현 순서대로 bind_finding_traffic(finding_id, traffic_refs)에 연결하고 baseline / proof / verification / supporting 역할과 용도를 설명합니다. 이번 취약점만 처리하며 중복 생성하거나 대상을 재탐지하지 않습니다. 성공 후 get_finding_traffic으로 최신 version과 필요한 본문을 다시 읽고 실제 읽은 version을 update_finding_report의 evidence_version으로 전달합니다(그 도구의 finding_id는 여전히 finding_node_id). 기존 연결은 중복 추가하지 않습니다. TCP, 미수집, 도구 사용 불가, 일치 항목 없음이면 연결을 건너뛰고 글/명령 증거로 보고서를 작성하며 이유를 설명합니다. 트래픽을 채우려고 추측하지 않습니다. 연결 실패를 성공이라고 하지 말고 기존 증거와 미연결 이유를 보존합니다."
	}
	if guidance != "" || has["get_finding_traffic"] || has["update_finding_report"] {
		guidance += findingIDGuidance
	}
	return out, guidance
}

func stripTrafficParameters(schema map[string]any) {
	props, _ := schema["properties"].(map[string]any)
	delete(props, "traffic_refs")
	delete(props, "evidence_hint_id")
	if required, ok := schema["required"].([]any); ok {
		kept := required[:0]
		for _, key := range required {
			if key != "traffic_refs" && key != "evidence_hint_id" {
				kept = append(kept, key)
			}
		}
		schema["required"] = kept
	}
	if hints, ok := props["hints"].(map[string]any); ok {
		if items, ok := hints["items"].(map[string]any); ok {
			stripTrafficParameters(items)
		}
	}
}

// HintTrafficSchema is shared by the task-local and cross-task hint tools.
func HintTrafficSchema() map[string]any {
	return map[string]any{"type": "array", "description": "선택 사항: 이 힌트의 취약점에 대응하며 검증된 트래픽 참조를 순서대로 보존합니다. 인계 후 report_finding에 evidence_hint_id로 전달할 수 있습니다.", "items": obj(map[string]any{"traffic_id": str("실제 트래픽 ID"), "role": str("baseline / proof / verification / supporting"), "note": str("이 트래픽이 뒷받침하는 결론")}, "traffic_id")}
}

func (t *ToolSet) findingRefsFromHint(hintID int64, explicit []db.TrafficRef) ([]db.TrafficRef, error) {
	if hintID <= 0 {
		return db.NormalizeTrafficRefs(explicit)
	}
	n, err := t.ts.GetNode(hintID) // local store only: inherited hints cannot supply evidence
	if err != nil {
		return nil, err
	}
	if n == nil || n.Kind != db.KindHint {
		return nil, fmt.Errorf("evidence_hint_id=%d은(는) 이 작업의 힌트 노드여야 합니다(상속된 힌트는 직접 연결에 사용할 수 없음)", hintID)
	}
	var payload struct {
		Refs []db.TrafficRef `json:"traffic_refs"`
	}
	if err := json.Unmarshal(n.Payload, &payload); err != nil {
		return nil, err
	}
	return db.NormalizeTrafficRefs(append(append([]db.TrafficRef{}, explicit...), payload.Refs...))
}
