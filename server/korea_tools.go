package server

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"log"
	"strconv"

	"github.com/Autumn-27/artex/agent"
	"github.com/Autumn-27/artex/traffic"
)

type koreanToolText struct {
	description string
	schema      map[string]any
}

func (s *Server) koreanToolDefaults() map[string]koreanToolText {
	defaults := map[string]koreanToolText{}
	for _, seed := range agent.BuiltinToolSeeds() {
		defaults[seed.Key] = koreanToolText{seed.Desc, seed.Schema}
	}
	tools := append(s.orchestrationTools(), s.platformTools()...)
	tools = append(tools, s.findingRetestTools()...)
	tools = append(tools, traffic.SeedToolMetas()...)
	for _, tool := range tools {
		defaults[tool.Name()] = koreanToolText{tool.Description(), tool.InputSchema()}
	}
	return defaults
}

func matchesLegacyToolText(text, hash string) bool {
	if hash == "" {
		return false
	}
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:]) == hash
}

// Only description leaves with exact legacy values change. A customized default,
// description, property or schema structure remains the user's choice.
func translateToolSchema(current, replacement any, path string, hashes map[string]string) {
	switch value := current.(type) {
	case map[string]any:
		next, ok := replacement.(map[string]any)
		if !ok {
			return
		}
		for key, child := range value {
			childPath := path + "/" + key
			if key == "description" {
				old, oldOK := child.(string)
				translated, newOK := next[key].(string)
				if oldOK && newOK && matchesLegacyToolText(old, hashes[childPath]) {
					value[key] = translated
				}
				continue
			}
			translateToolSchema(child, next[key], childPath, hashes)
		}
	case []any:
		next, ok := replacement.([]any)
		if !ok {
			return
		}
		for i, child := range value {
			if i < len(next) {
				translateToolSchema(child, next[i], path+"/"+strconv.Itoa(i), hashes)
			}
		}
	}
}

func (s *Server) upgradeKoreanTools() {
	const flag = "korea_tools_v1"
	done, _, err := s.m.pg.GetSetting(flag)
	if err != nil {
		log.Printf("[tools] 한국어 이전 상태 조회 실패: %v", err)
		return
	}
	if done == "true" {
		return
	}
	for key, defaults := range s.koreanToolDefaults() {
		before, err := s.m.pg.GetTool(key)
		if err != nil {
			log.Printf("[tools] %s 조회 실패: %v", key, err)
			return
		}
		if before == nil || !before.System {
			continue
		}
		hashes := legacyToolTextHashes[key]
		description := before.Description
		if matchesLegacyToolText(description, hashes["/description"]) {
			description = defaults.description
		}
		var schema map[string]any
		if err := json.Unmarshal(before.Schema, &schema); err != nil {
			log.Printf("[tools] %s 스키마 해석 실패: %v", key, err)
			return
		}
		// Normalize arrays (including []string) to JSON values before walking.
		raw, err := json.Marshal(defaults.schema)
		if err != nil {
			return
		}
		var next map[string]any
		if err := json.Unmarshal(raw, &next); err != nil {
			return
		}
		translateToolSchema(schema, next, "/schema", hashes)
		raw, err = json.Marshal(schema)
		if err != nil {
			return
		}
		updated, err := s.m.pg.UpgradeToolText(before, description, raw)
		if err != nil {
			log.Printf("[tools] %s 한국어 이전 실패: %v", key, err)
			return
		}
		if !updated {
			return // A concurrent edit won; retry the remaining fields next startup.
		}
	}
	if err := s.m.pg.SetSetting(flag, "true"); err != nil {
		log.Printf("[tools] 한국어 이전 상태 저장 실패: %v", err)
	}
}
