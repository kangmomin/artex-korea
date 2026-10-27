package agent

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
	"unicode"
)

// Concatenated raw/string literals in the original templates must be hashed as
// compiled text; source-token extraction produces different hashes.
func TestKoreanPromptUpgradeCompiledLegacyDefaults(t *testing.T) {
	raw, err := os.ReadFile("testdata/legacy-korea-prompts.json")
	if err != nil {
		t.Fatal(err)
	}
	var legacy map[string]string
	if err := json.Unmarshal(raw, &legacy); err != nil {
		t.Fatal(err)
	}
	for key, current := range legacy {
		if next, ok := KoreanPromptUpgrade(key, current); !ok || next != BuiltinPromptSeeds()[key] {
			t.Errorf("legacy %s was not translated", key)
		}
		if _, ok := KoreanPromptUpgrade(key, current+"\nuser edit"); ok {
			t.Errorf("custom %s was overwritten", key)
		}
	}
}

func TestKoreanDefaultsAndTemplateVariables(t *testing.T) {
	prompts := BuiltinPromptSeeds()
	prompts["reporter"] = ReporterDefaultPrompt
	prompts["retester"] = RetesterDefaultPrompt
	prompts["assistant"] = DefaultAssistantPrompt
	prompts["scope"] = goalsScopeTail
	prompts["traffic"] = workerTrafficBlock(true)
	prompts["artifacts"] = artifactSpec("/work") + workerArtifactSpec("/work")
	prompts["language"] = langDirective("ko")
	for key, prompt := range prompts {
		t.Run(key, func(t *testing.T) {
			if strings.IndexFunc(prompt, func(r rune) bool { return unicode.Is(unicode.Han, r) }) >= 0 {
				t.Fatal("Korean default contains Chinese text")
			}
			if strings.IndexFunc(prompt, func(r rune) bool { return unicode.Is(unicode.Hangul, r) }) < 0 {
				t.Fatal("Korean default has no Hangul")
			}
			if _, err := renderTmpl(prompt, map[string]string{"Goal": "허가된 테스트", "Scope": "example.test", "ProxyAddr": "", "DataDir": "/data", "Now": "2026-10-27", "WorkerName": "worker-1", "AssetSummary": "", "FindingsSummary": "", "EngagementDescription": ""}); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestKoreanPromptUpgradePreservesEdits(t *testing.T) {
	legacy := "你是一个乐于助人的 AI 助手。请用简洁、准确的中文回答用户的问题；在需要时使用可用的工具来完成任务。只做用户要求的事，不臆造信息。"
	if next, ok := KoreanPromptUpgrade("my-assistant", legacy); !ok || next != DefaultAssistantPrompt {
		t.Fatal("unchanged legacy assistant was not translated")
	}
	for _, custom := range []string{legacy + " 사용자 추가 지침", DefaultAssistantPrompt, "custom prompt", ""} {
		if _, ok := KoreanPromptUpgrade("my-assistant", custom); ok {
			t.Fatal("overwrote a custom/current prompt")
		}
	}
}
