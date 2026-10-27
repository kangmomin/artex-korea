package db

import "database/sql"

// UpgradePromptIfUnchanged appends a translated version only while the active
// prompt still matches the inspected default. A concurrent user edit wins.
func (d *DB) UpgradePromptIfUnchanged(agentID int64, expected, replacement string) error {
	tx, err := d.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var pid sql.NullInt64
	if err := tx.QueryRow(`SELECT current_prompt_id FROM agents WHERE id=$1 FOR UPDATE`, agentID).Scan(&pid); err != nil {
		return err
	}
	if !pid.Valid {
		return nil
	}
	var current string
	if err := tx.QueryRow(`SELECT template_text FROM agent_prompts WHERE id=$1`, pid.Int64).Scan(&current); err != nil {
		return err
	}
	if current != expected || current == replacement {
		return nil
	}
	var version int
	if err := tx.QueryRow(`SELECT COALESCE(max(version),0)+1 FROM agent_prompts WHERE agent_id=$1`, agentID).Scan(&version); err != nil {
		return err
	}
	var nextID int64
	if err := tx.QueryRow(`INSERT INTO agent_prompts(agent_id,version,template_text,note,updated_by)
        VALUES ($1,$2,$3,'한국어 기본 프롬프트로 이전','system') RETURNING id`, agentID, version, replacement).Scan(&nextID); err != nil {
		return err
	}
	if _, err := tx.Exec(`UPDATE agents SET current_prompt_id=$1 WHERE id=$2`, nextID, agentID); err != nil {
		return err
	}
	return tx.Commit()
}
