package db

import "encoding/json"

// UpgradeToolText preserves edits made after the caller read the tool. Bindings,
// defaults, enabled flags and execution configuration are not reset.
func (d *DB) UpgradeToolText(before *Tool, description string, schema json.RawMessage) (bool, error) {
	result, err := d.Exec(`UPDATE tools SET description=$2,schema=$3::jsonb,updated_at=now()
WHERE key=$1 AND system AND description=$4 AND schema=$5::jsonb`,
		before.Key, description, string(schema), before.Description, string(before.Schema))
	if err != nil {
		return false, err
	}
	n, err := result.RowsAffected()
	return n == 1, err
}
