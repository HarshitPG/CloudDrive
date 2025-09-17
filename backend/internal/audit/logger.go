package audit

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"
)

func Log(ctx context.Context, db *sql.DB, userID, action, targetType, targetID string, meta map[string]interface{}) error {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	var metaJSON []byte
	if meta != nil {
		metaJSON, _ = json.Marshal(meta)
	} else {
		metaJSON = []byte("{}")
	}

	_, err := db.ExecContext(ctx, `
		INSERT INTO audit_logs (user_id, action, target_type, target_id, meta, created_at)
		VALUES ($1,$2,$3,$4,$5,now())
	`, userID, action, targetType, targetID, string(metaJSON))
	return err
}
