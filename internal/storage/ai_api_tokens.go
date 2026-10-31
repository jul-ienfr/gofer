package storage

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

const AIAPITokenLimit = 20

type AIAPIToken struct {
	ID         string
	UserID     string
	Name       string
	TokenHash  string `json:"-"`
	Scopes     string
	ExpiresAt  time.Time
	LastUsedAt time.Time
	CreatedAt  time.Time
	RevokedAt  time.Time
}

func HashAIAPIToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func (db *DB) CreateAIAPIToken(ctx context.Context, token AIAPIToken) (AIAPIToken, error) {
	token.UserID = strings.TrimSpace(token.UserID)
	token.Name = strings.TrimSpace(token.Name)
	token.Scopes = strings.TrimSpace(token.Scopes)
	if token.UserID == "" || token.Name == "" || token.Scopes == "" {
		return AIAPIToken{}, fmt.Errorf("api token requires user, name and scopes")
	}
	if token.ID == "" {
		token.ID = uuid.NewString()
	}
	if token.ExpiresAt.IsZero() {
		token.ExpiresAt = time.Now().UTC().AddDate(0, 0, 90)
	}
	_, err := db.Write().ExecContext(ctx, `
		INSERT INTO ai_api_tokens (id, user_id, name, token_hash, scopes, expires_at)
		VALUES (?, ?, ?, ?, ?, ?)`,
		token.ID, token.UserID, token.Name, token.TokenHash, token.Scopes,
		token.ExpiresAt.UTC().Format("2006-01-02 15:04:05"))
	if err != nil {
		return AIAPIToken{}, err
	}
	return db.GetAIAPIToken(ctx, token.UserID, token.ID)
}

func (db *DB) GetAIAPIToken(ctx context.Context, userID, id string) (AIAPIToken, error) {
	return scanAIAPIToken(db.Read().QueryRowContext(ctx, `
		SELECT id, user_id, name, scopes, expires_at, last_used_at, created_at, revoked_at
		FROM ai_api_tokens WHERE id = ? AND user_id = ?`, id, userID))
}

func (db *DB) ListAIAPITokens(ctx context.Context, userID string) ([]AIAPIToken, error) {
	rows, err := db.Read().QueryContext(ctx, `
		SELECT id, user_id, name, scopes, expires_at, last_used_at, created_at, revoked_at
		FROM ai_api_tokens
		WHERE user_id = ?
		ORDER BY created_at DESC, id DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var tokens []AIAPIToken
	for rows.Next() {
		token, err := scanAIAPIToken(rows)
		if err != nil {
			return nil, err
		}
		tokens = append(tokens, token)
	}
	return tokens, rows.Err()
}

func (db *DB) CountAIAPITokens(ctx context.Context, userID string) (int, error) {
	var count int
	err := db.Read().QueryRowContext(ctx, `
		SELECT COUNT(*) FROM ai_api_tokens
		WHERE user_id = ? AND revoked_at IS NULL`, userID).Scan(&count)
	return count, err
}

func (db *DB) RevokeAIAPIToken(ctx context.Context, userID, id string) (bool, error) {
	result, err := db.Write().ExecContext(ctx, `
		UPDATE ai_api_tokens SET revoked_at = CURRENT_TIMESTAMP
		WHERE id = ? AND user_id = ? AND revoked_at IS NULL`, id, userID)
	if err != nil {
		return false, err
	}
	affected, err := result.RowsAffected()
	return affected > 0, err
}

func (db *DB) AuthenticateAIAPIToken(ctx context.Context, tokenHash string, now time.Time) (AIAPIToken, error) {
	token, err := scanAIAPIToken(db.Read().QueryRowContext(ctx, `
		SELECT id, user_id, name, scopes, expires_at, last_used_at, created_at, revoked_at
		FROM ai_api_tokens
		WHERE token_hash = ? AND revoked_at IS NULL
		  AND (expires_at IS NULL OR expires_at > ?)`,
		tokenHash, now.UTC().Format("2006-01-02 15:04:05")))
	if err != nil {
		return AIAPIToken{}, err
	}
	_, _ = db.Write().ExecContext(ctx, `UPDATE ai_api_tokens SET last_used_at = CURRENT_TIMESTAMP WHERE id = ?`, token.ID)
	return token, nil
}

type aiAPITokenScanner interface {
	Scan(dest ...any) error
}

func scanAIAPIToken(row aiAPITokenScanner) (AIAPIToken, error) {
	var token AIAPIToken
	var expires, lastUsed, created, revoked *string
	if err := row.Scan(&token.ID, &token.UserID, &token.Name, &token.Scopes, &expires, &lastUsed, &created, &revoked); err != nil {
		return AIAPIToken{}, err
	}
	token.ExpiresAt = parseAIWhen(expires)
	token.LastUsedAt = parseAIWhen(lastUsed)
	token.CreatedAt = parseAIWhen(created)
	token.RevokedAt = parseAIWhen(revoked)
	return token, nil
}
