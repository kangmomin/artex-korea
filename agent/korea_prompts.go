package agent

import (
	"crypto/sha256"
	"encoding/hex"
)

// Hashes identify the exact unedited defaults shipped by artex-global. They
// allow translation upgrades without retaining Chinese prompts or replacing edits.
var legacyPromptHashes = map[string]string{
	"goals":     "72a7a0159443b41dde29dfba8a3de6a2c8a15b273935495a7b88561fa708aa68",
	"planner":   "14e163f4a44da9fee20a214048a607adc3659b2025275569eb512532ca8c15d5",
	"mainagent": "12bb69ee0886c4fb9beb936d882021a91104860f1da69a5e3eda3d9e345cd399",
	"worker":    "80fbb1c610dd57e266c20b9145793339eee540b809f6db8f5336261b45879f08",
	"auto":      "b2a71b82243607337b19f0642317fed88c144d536412d80eaaf90807f2905a1d",
	"pentest":   "a54bd5713b1c121d0f2ff3a72e0304820b1368c4b4b8159d5d651895663ac652",
	"reporter":  "8ba98208f6c5f2ccb103c88ef13239c4264733400c0e3dfec96d3a3572d78d39",
	"retester":  "d08326aa9fbd9a1367e716cf7ef46533e9096e9a56493f77d364efbf696f7fe0",
	"assistant": "a179c990bff1ab4e0a6f326945a1c44036ef4c8a54c7c5b635965f6cefdc9f0a",
}

// KoreanPromptUpgrade returns a translation only for a known, unchanged default.
func KoreanPromptUpgrade(key, current string) (string, bool) {
	sum := sha256.Sum256([]byte(current))
	hash := hex.EncodeToString(sum[:])
	if hash == legacyPromptHashes["assistant"] {
		return DefaultAssistantPrompt, true
	}
	if hash != legacyPromptHashes[key] {
		return "", false
	}
	switch key {
	case "reporter":
		return ReporterDefaultPrompt, true
	case "retester":
		return RetesterDefaultPrompt, true
	default:
		replacement, ok := BuiltinPromptSeeds()[key]
		return replacement, ok
	}
}
