package server

import (
	"log"

	"github.com/Autumn-27/artex/agent"
)

// Upgrade only untouched defaults, once. Version history remains available;
// a later user rollback or edit must survive subsequent restarts.
func (s *Server) upgradeKoreanPrompts() {
	const flag = "korea_prompts_v1"
	done, _, err := s.m.pg.GetSetting(flag)
	if err != nil {
		log.Printf("[prompts] 한국어 이전 상태 조회 실패: %v", err)
		return
	}
	if done == "true" {
		return
	}
	agents, err := s.m.pg.ListAgents()
	if err != nil {
		log.Printf("[prompts] 에이전트 목록 조회 실패: %v", err)
		return
	}
	for _, a := range agents {
		current, err := s.m.pg.CurrentPrompt(a.ID)
		if err != nil {
			log.Printf("[prompts] %s 조회 실패: %v", a.Key, err)
			return
		}
		replacement, ok := agent.KoreanPromptUpgrade(a.Key, current)
		if !ok {
			continue
		}
		if err := s.m.pg.UpgradePromptIfUnchanged(a.ID, current, replacement); err != nil {
			log.Printf("[prompts] %s 한국어 이전 실패: %v", a.Key, err)
			return
		}
	}
	if err := s.m.pg.SetSetting(flag, "true"); err != nil {
		log.Printf("[prompts] 한국어 이전 상태 저장 실패: %v", err)
	}
}
