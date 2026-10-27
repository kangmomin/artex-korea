package server

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"testing"
	"unicode"
)

func TestKoreanToolSchemaPreservesCustomFields(t *testing.T) {
	current := map[string]any{}
	next := map[string]any{}
	if err := json.Unmarshal([]byte(`{"properties":{"host":{"description":"旧说明","default":"my.example"},"port":{"description":"my instructions","default":8443}},"required":["host"],"custom":true}`), &current); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(`{"properties":{"host":{"description":"대상 호스트","default":"example.test"},"port":{"description":"포트","default":80},"new":{"description":"새 항목"}},"required":["port"]}`), &next); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256([]byte("旧说明"))
	hash := hex.EncodeToString(sum[:])
	hashes := map[string]string{"/schema/properties/host/description": hash, "/schema/properties/port/description": hash}
	translateToolSchema(current, next, "/schema", hashes)
	props := current["properties"].(map[string]any)
	host := props["host"].(map[string]any)
	port := props["port"].(map[string]any)
	if host["description"] != "대상 호스트" || host["default"] != "my.example" || port["description"] != "my instructions" || port["default"] != float64(8443) {
		t.Fatalf("translation overwrote customization: %#v", props)
	}
	if _, added := props["new"]; added || current["custom"] != true || current["required"].([]any)[0] != "host" {
		t.Fatalf("translation changed schema structure: %#v", current)
	}
	translateToolSchema(current, next, "/schema", hashes)
	if host["description"] != "대상 호스트" {
		t.Fatal("translation is not idempotent")
	}
}

func TestKoreanToolDefaults(t *testing.T) {
	s := &Server{}
	for key, defaults := range s.koreanToolDefaults() {
		if len(legacyToolTextHashes[key]) == 0 {
			t.Errorf("missing legacy descriptions for %s", key)
		}
		raw, err := json.Marshal(defaults.schema)
		if err != nil {
			t.Fatal(err)
		}
		for _, r := range defaults.description + string(raw) {
			if unicode.Is(unicode.Han, r) {
				t.Errorf("Chinese text remains in %s", key)
				break
			}
		}
	}
}
