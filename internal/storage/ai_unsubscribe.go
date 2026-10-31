package storage

import (
	"context"
	"strings"

	"github.com/google/uuid"
)

type AIUnsubscribeStatus struct {
	ID         string
	MessageID  int64
	Status     string
	HTTPStatus int
	Error      string
	CreatedAt  string
}

type AIUnsubscribeAttempt struct {
	ID                  string
	MessageID           int64
	ListUnsubscribeHash string
	Method              string
	TargetHash          string
	Status              string
	HTTPStatus          int
	RedirectCount       int
	Error               string
}

// ReserveAIUnsubscribeAttempt inserts a sending marker once per message and
// header. It returns false when a previous attempt already exists.
func (db *DB) ReserveAIUnsubscribeAttempt(ctx context.Context, attempt AIUnsubscribeAttempt) (string, bool, error) {
	if attempt.MessageID <= 0 || strings.TrimSpace(attempt.ListUnsubscribeHash) == "" {
		return "", false, nil
	}
	if attempt.ID == "" {
		attempt.ID = uuid.NewString()
	}
	result, err := db.Write().ExecContext(ctx, `
		INSERT INTO ai_unsubscribe_attempts
			(id, message_id, list_unsubscribe_hash, method, target_hash, status, http_status, redirect_count, error)
		SELECT ?, ?, ?, ?, ?, 'sending', 0, 0, ''
		WHERE NOT EXISTS (
			SELECT 1 FROM ai_unsubscribe_attempts
			WHERE message_id = ? AND list_unsubscribe_hash = ?
			  AND (status = 'sent'
			       OR (status = 'sending' AND created_at >= datetime('now', '-15 minutes'))
			       OR (status = 'failed' AND created_at >= datetime('now', '-1 hour')))
		)`,
		attempt.ID, attempt.MessageID, attempt.ListUnsubscribeHash, attempt.Method,
		attempt.TargetHash, attempt.MessageID, attempt.ListUnsubscribeHash)
	if err != nil {
		return "", false, err
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return "", false, err
	}
	if changed == 0 {
		var existing string
		err = db.Read().QueryRowContext(ctx, `
			SELECT id FROM ai_unsubscribe_attempts
			WHERE message_id = ? AND list_unsubscribe_hash = ?
			  AND (status = 'sent'
			       OR (status = 'sending' AND created_at >= datetime('now', '-15 minutes'))
			       OR (status = 'failed' AND created_at >= datetime('now', '-1 hour')))
			ORDER BY created_at DESC LIMIT 1`, attempt.MessageID, attempt.ListUnsubscribeHash).Scan(&existing)
		return existing, false, err
	}
	return attempt.ID, true, nil
}

func (db *DB) ListAIUnsubscribeStatus(ctx context.Context, userID string, limit int) ([]AIUnsubscribeStatus, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	rows, err := db.Read().QueryContext(ctx, `
		SELECT u.id, u.message_id, u.status, u.http_status, u.error,
		       strftime('%Y-%m-%dT%H:%M:%SZ', u.created_at)
		FROM ai_unsubscribe_attempts u
		JOIN messages m ON m.id = u.message_id
		JOIN accounts a ON a.id = m.account_id
		WHERE a.user_id = ?
		ORDER BY u.created_at DESC, u.id DESC
		LIMIT ?`, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []AIUnsubscribeStatus
	for rows.Next() {
		var item AIUnsubscribeStatus
		if err := rows.Scan(&item.ID, &item.MessageID, &item.Status, &item.HTTPStatus, &item.Error, &item.CreatedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (db *DB) FinishAIUnsubscribeAttempt(ctx context.Context, id, status string, httpStatus, redirects int, message string) error {
	_, err := db.Write().ExecContext(ctx, `
		UPDATE ai_unsubscribe_attempts
		SET status = ?, http_status = ?, redirect_count = ?, error = ?
		WHERE id = ?`, status, httpStatus, redirects, truncateAttemptError(message), id)
	return err
}

func truncateAttemptError(value string) string {
	value = strings.TrimSpace(value)
	if len(value) > 500 {
		return value[:500]
	}
	return value
}
