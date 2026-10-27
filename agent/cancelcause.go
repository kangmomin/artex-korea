package agent

import (
	"context"
	"errors"
	"fmt"
)

// AbortCause names why an agent run's context was cancelled. Every cancellation
// site should attach one so the activity trace can report the real initiator.
type AbortCause struct {
	Code  string
	Short string
	Text  string
}

func (c *AbortCause) Error() string { return c.Text }

func cause(code, short, text string) *AbortCause {
	return &AbortCause{Code: code, Short: short, Text: text}
}

// Causef builds a cause that includes runtime-specific detail.
func Causef(code, short, format string, args ...any) *AbortCause {
	return &AbortCause{Code: code, Short: short, Text: fmt.Sprintf(format, args...)}
}

var (
	// Task-level execution context.
	AbortPausedByUser = cause("paused_by_user", "사용자가 작업을 일시 정지함",
		"사용자가 작업 제어 API(POST /api/tasks/{id}/control, action=pause)로 일시 정지했습니다. Planner/Worker 실행이 취소되고 실행 중 의도는 frontier(open)로 돌아가 재개 시 처음부터 실행됩니다")
	AbortPausedByOrchestrator = cause("paused_by_orchestrator", "운영 에이전트가 작업을 일시 정지함",
		"운영 에이전트가 pause_task를 호출했습니다. Planner/Worker 실행이 취소되고 의도는 frontier(open)로 돌아가 재개 시 다시 실행됩니다")
	AbortTaskDeleted = cause("task_deleted", "작업이 삭제됨",
		"작업 삭제(DELETE /api/tasks/{id}) 중입니다. 삭제 보호 절차가 Planner·Worker·주 에이전트를 취소했으며 이번 결과는 사용되지 않습니다")
	AbortPausedOnReload = cause("paused_on_reload", "백엔드가 작업의 일시 정지 상태를 복원함",
		"백엔드 시작 시 DB에 저장된 일시 정지 상태를 복원하여 실행을 취소했습니다. 정상적인 복원 단계에는 실행 중인 에이전트가 없습니다")
	AbortGoalMet = cause("goal_met", "계획 에이전트가 목표 달성으로 판단함",
		"목표 달성으로 작업을 done 처리하고 남은 Worker를 취소했습니다. 해당 의도는 실패가 아닌 stopped로 표시됩니다")
	AbortSettleDrainTimeout = cause("settle_drain_timeout", "작업 시간 초과 후 마무리 대기 시간이 소진됨",
		"작업 시간 초과 후 Worker의 정상 마무리를 기다렸지만 90초 유예 시간이 소진되어 강제 취소했습니다. 의도는 exhausted이며 이미 저장한 사실과 자산은 보존됩니다")

	// Per-work context.
	AbortKilledByPlanner = cause("killed_by_planner", "계획 에이전트가 의도를 종료함",
		"계획 에이전트가 kill_work로 의도를 종료했습니다. 방향이 잘못되었거나 계속할 가치가 없다고 판단했을 수 있으며 stopped로 표시되고 자동 재배정되지 않습니다")
	AbortWorkPausedByUser = cause("work_paused_by_user", "사용자가 Worker 의도를 일시 정지함",
		"사용자가 실행 중인 Worker를 일시 정지했습니다. 호출은 취소되고 의도는 paused입니다. 의도·사실·취약점·활동 기록은 보존되며 재개 시 처음부터 실행됩니다")
	AbortWorkCancelledByUser = cause("work_cancelled_by_user", "사용자가 Worker 의도를 삭제함",
		"사용자가 실행 중인 Worker를 삭제하여 호출이 취소됐습니다. 쓰기 구간 종료 후 선택한 삭제 방식이 적용됩니다. 논리 삭제는 산출물을 보존하고, 영구 삭제는 의도와 그것만을 근거로 하는 하위 노드를 함께 제거합니다")
	AbortWorkFinished = cause("work_finished", "Worker가 정상 종료되어 context를 해제함",
		"Worker가 정상 종료되어 detachWork에서 context를 해제합니다. 실행 중단이 아니며 중단 메시지에 나타났다면 취소와 종료 이벤트가 경합한 것입니다")
	AbortPausedRaceGuard = cause("paused_race_guard", "작업 일시 정지 중 새 실행 시작이 거부됨",
		"일시 정지와 claim의 경합으로 Worker가 시작되지 않도록 새 실행 context를 거부했습니다. 가져온 의도는 frontier로 돌아갑니다")

	// Main Agent and standalone conversation contexts.
	AbortChatStoppedByUser = cause("chat_stopped_by_user", "사용자가 이번 대화를 중지함",
		"사용자가 중지를 눌러 주 에이전트 또는 대화 에이전트 실행을 중단했습니다. 활동 기록은 보존되며 다음 메시지를 보낼 수 있습니다")
	AbortChatPausedWithTask = cause("chat_paused_with_task", "작업 일시 정지로 주 에이전트 대화도 중단됨",
		"작업 일시 정지와 함께 주 에이전트 대화도 취소됐습니다. 활동 기록은 보존되며 재개해도 이번 메시지를 자동 재실행하지 않습니다")
	AbortChatTurnFinished = cause("chat_turn_finished", "이번 대화가 정상 종료되어 context를 해제함",
		"대화가 정상 종료되어 context를 해제합니다. 실행 중단이 아니며 중단 메시지에 나타났다면 취소와 종료 이벤트가 경합한 것입니다")

	// Process-level and per-run hard backstop.
	AbortShutdown = cause("shutdown", "백엔드 프로세스 종료 중",
		"백엔드가 SIGINT/SIGTERM을 받아 재시작·업데이트·종료 중입니다. 모든 에이전트 실행이 취소되며 재시작 후 남은 running 의도는 open으로 바뀌어 다시 실행됩니다")
	AbortRunHardTimeout = cause("run_hard_timeout", "실행의 강제 시간 한도가 적용됨",
		"모델 요청이나 도구가 오래 반환되지 않아 실행 예산과 추가 유예 시간을 모두 넘겼습니다. 정상적인 단계 경계 마무리를 수행하지 못했으므로 마지막 미반환 도구 호출을 확인하세요")
)

// AbortReason resolves the named cause attached to a cancelled run context.
func AbortReason(ctx context.Context) (code, short, text string, ok bool) {
	c := context.Cause(ctx)
	if c == nil {
		return "", "", "", false
	}
	var ac *AbortCause
	if errors.As(c, &ac) {
		return ac.Code, ac.Short, ac.Text, true
	}
	switch {
	case errors.Is(c, context.DeadlineExceeded):
		return "deadline_exceeded", "상위 context의 마감 시각 도달",
			"상위 context가 deadline에 도달했지만 WithTimeoutCause로 명시적인 이유가 전달되지 않았습니다: " + c.Error(), true
	case errors.Is(c, context.Canceled):
		return "canceled_no_cause", "취소 호출자가 이유를 지정하지 않음",
			"상위 context가 취소됐지만 context.WithCancelCause로 이유를 전달하지 않았습니다. agent/cancelcause.go에 이유를 등록하고 해당 취소 지점에 연결하세요", true
	default:
		return "other", firstLine(c.Error(), 80), c.Error(), true
	}
}
