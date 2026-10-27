package agent

import (
	"strings"

	"github.com/Autumn-27/artex/db"
)

// constraintBlock renders this task's operation constraints (task_constraints) as a
// high-priority block appended to the planner/worker system prompt. allow/deny are
// grouped; empty string when there are no constraints (or ts is nil). The framing
// deliberately puts these ABOVE the exploration/expansion heuristics so a declared
// boundary wins the tug-of-war against "chase another entry surface".
func constraintBlock(ts *db.ExplorationStore) string {
	if ts == nil {
		return ""
	}
	rows, err := ts.ListConstraints()
	if err != nil || len(rows) == 0 {
		return ""
	}
	var allow, deny []string
	for _, c := range rows {
		text := strings.TrimSpace(c.Text)
		if text == "" {
			continue
		}
		if c.Kind == "allow" {
			allow = append(allow, "- "+text)
		} else {
			deny = append(deny, "- "+text)
		}
	}
	if len(allow) == 0 && len(deny) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("\n\n【작업 제약(최우선: 아래의 모든 탐색/범위 확장 지침보다 우선합니다. 의도를 생성하거나 행동을 수행하기 전에 반드시 위반 여부를 확인하고, 위반하면 진행하지 않습니다)】:")
	if len(allow) > 0 {
		b.WriteString("\n허용 작업:\n")
		b.WriteString(strings.Join(allow, "\n"))
	}
	if len(deny) > 0 {
		b.WriteString("\n금지 작업:\n")
		b.WriteString(strings.Join(deny, "\n"))
	}
	b.WriteString("\n(새 대상/포트/호스트를 발견했다고 허가를 얻은 것은 아닙니다. 위 허용 범위에 없으면 out-of-scope 사실로 기록하고 건너뜁니다. 해당 대상으로 의도를 파생하거나 행동하지 않습니다.)")
	return b.String()
}
