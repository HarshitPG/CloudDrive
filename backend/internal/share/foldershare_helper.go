package share

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
)

func generateShareToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("failed to generate random token: %w", err)
	}
	return hex.EncodeToString(b), nil
}

func buildPath(pathParts []string) string {
	if len(pathParts) == 0 {
		return "/"
	}
	result := ""
	for _, part := range pathParts {
		result += "/" + part
	}
	return result
}

func GetShareOwnership(ctx context.Context, db *sql.DB, shareID string) (string, string, error) {
	var creatorID, token string
	err := db.QueryRowContext(ctx,
		"SELECT creator_id, token FROM shares WHERE id=$1", shareID).Scan(&creatorID, &token)
	return creatorID, token, err
}
