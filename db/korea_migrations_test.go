package db

import (
	"encoding/json"
	"testing"
)

func TestKoreanRuleMigrationPreservesPolicyAndUserText(t *testing.T) {
	d, err := Open(testDSN(t))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	legacy := koreanRuleLabels[0]
	var id int64
	if err := d.QueryRow(`INSERT INTO intercept_rules(name,enabled,priority,match_target,match_type,pattern,action,message)
VALUES($1,false,777,'tool_input','string','user-pattern','allow','custom explanation') RETURNING id`, legacy[0]).Scan(&id); err != nil {
		t.Fatal(err)
	}
	defer d.Exec(`DELETE FROM intercept_rules WHERE id=$1`, id)
	if err := d.SetSetting("korea_rule_labels_v1", "false"); err != nil {
		t.Fatal(err)
	}
	if err := d.upgradeKoreanRuleLabels(); err != nil {
		t.Fatal(err)
	}
	var name, pattern, action, message string
	var enabled bool
	var priority int
	if err := d.QueryRow(`SELECT name,pattern,action,message,enabled,priority FROM intercept_rules WHERE id=$1`, id).Scan(&name, &pattern, &action, &message, &enabled, &priority); err != nil {
		t.Fatal(err)
	}
	if name != legacy[1] || pattern != "user-pattern" || action != "allow" || message != "custom explanation" || enabled || priority != 777 {
		t.Fatalf("migration changed policy or user text: %q %q %q %q %v %d", name, pattern, action, message, enabled, priority)
	}
	// A later deliberate revert must survive the next startup.
	if _, err := d.Exec(`UPDATE intercept_rules SET name=$2 WHERE id=$1`, id, legacy[0]); err != nil {
		t.Fatal(err)
	}
	if err := d.upgradeKoreanRuleLabels(); err != nil {
		t.Fatal(err)
	}
	if err := d.QueryRow(`SELECT name FROM intercept_rules WHERE id=$1`, id).Scan(&name); err != nil || name != legacy[0] {
		t.Fatalf("repeat migration: %q %v", name, err)
	}
}

func TestKoreanPromptMigrationKeepsHistoryAndEdits(t *testing.T) {
	d, err := Open(testDSN(t))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	var id int64
	if err := d.QueryRow(`INSERT INTO agents(key,name,role,builtin) VALUES('test_korea_prompt','test','assistant',false) RETURNING id`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	defer d.Exec(`DELETE FROM agents WHERE id=$1`, id)
	if _, err := d.SavePrompt(id, "old default", "seed", "system"); err != nil {
		t.Fatal(err)
	}
	if err := d.UpgradePromptIfUnchanged(id, "old default", "한국어 기본값"); err != nil {
		t.Fatal(err)
	}
	if err := d.UpgradePromptIfUnchanged(id, "old default", "한국어 기본값"); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := d.QueryRow(`SELECT count(*) FROM agent_prompts WHERE agent_id=$1`, id).Scan(&count); err != nil || count != 2 {
		t.Fatalf("history count=%d err=%v", count, err)
	}
	if _, err := d.SavePrompt(id, "user edit", "custom", "user"); err != nil {
		t.Fatal(err)
	}
	if err := d.UpgradePromptIfUnchanged(id, "한국어 기본값", "replacement"); err != nil {
		t.Fatal(err)
	}
	if current, err := d.CurrentPrompt(id); err != nil || current != "user edit" {
		t.Fatalf("stale migration overwrote edit: %q %v", current, err)
	}
}

func TestKoreanToolMigrationCompareAndSwap(t *testing.T) {
	d, err := Open(testDSN(t))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	const key = "test-korea-tool"
	defer d.Exec(`DELETE FROM tools WHERE key=$1`, key)
	if err := d.SeedTool(key, "old default", json.RawMessage(`{"properties":{"host":{"default":"custom.example"}}}`), json.RawMessage(`["worker"]`)); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Exec(`UPDATE tools SET enabled=false WHERE key=$1`, key); err != nil {
		t.Fatal(err)
	}
	before, err := d.GetTool(key)
	if err != nil {
		t.Fatal(err)
	}
	updated, err := d.UpgradeToolText(before, "한국어 설명", before.Schema)
	if err != nil || !updated {
		t.Fatalf("upgrade: %v %v", updated, err)
	}
	updated, err = d.UpgradeToolText(before, "stale overwrite", before.Schema)
	if err != nil || updated {
		t.Fatalf("stale comparison: %v %v", updated, err)
	}
	after, err := d.GetTool(key)
	if err != nil || after.Description != "한국어 설명" || after.Enabled || len(after.Agents) != 1 || after.Agents[0] != "worker" || string(after.Schema) != string(before.Schema) {
		t.Fatalf("metadata was reset: %+v %v", after, err)
	}
}
