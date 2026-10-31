package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	aiRoutingMaxAttempts = 8
	aiRoutingLease       = 2 * time.Minute
)

type AIConnector struct {
	ID              string
	UserID          string
	Name            string
	Type            string
	URL             string
	SecretEncrypted string
	Enabled         bool
	AllowPrivate    bool
}

type AIRoutingStatus struct {
	ID             string
	Project        string
	Connector      string
	State          string
	Attempts       int
	ResponseStatus int
	LastError      string
	UpdatedAt      string
}

type AIRoutingItem struct {
	ID             string
	MessageID      int64
	ProjectID      string
	ConnectorID    string
	EventType      string
	PayloadJSON    string
	IdempotencyKey string
	State          string
	Attempts       int
	NextAttemptAt  time.Time
	LeaseOwner     string
	LeaseExpiresAt time.Time
	ResponseStatus int
	LastError      string
	UserID         string
}

func (db *DB) CreateAIConnector(ctx context.Context, connector AIConnector) (AIConnector, error) {
	connector.Name = strings.TrimSpace(connector.Name)
	connector.URL = strings.TrimSpace(connector.URL)
	if connector.UserID == "" || connector.Name == "" || connector.URL == "" {
		return AIConnector{}, fmt.Errorf("connector requires user, name and URL")
	}
	switch connector.Type {
	case "webhook", "http", "n8n", "make", "zapier":
	default:
		return AIConnector{}, fmt.Errorf("invalid connector type")
	}
	if connector.ID == "" {
		connector.ID = uuid.NewString()
	}
	_, err := db.Write().ExecContext(ctx, `
		INSERT INTO ai_connectors (id, user_id, name, type, url, secret_encrypted, enabled, allow_private)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		connector.ID, connector.UserID, connector.Name, connector.Type, connector.URL,
		connector.SecretEncrypted, connector.Enabled, connector.AllowPrivate)
	if err != nil {
		return AIConnector{}, err
	}
	return connector, nil
}

func (db *DB) GetAIConnector(ctx context.Context, userID, id string) (AIConnector, error) {
	var connector AIConnector
	err := db.Read().QueryRowContext(ctx, `
		SELECT id, user_id, name, type, url, secret_encrypted, enabled, allow_private
		FROM ai_connectors WHERE id = ? AND user_id = ?`, id, userID).Scan(
		&connector.ID, &connector.UserID, &connector.Name, &connector.Type,
		&connector.URL, &connector.SecretEncrypted, &connector.Enabled, &connector.AllowPrivate)
	return connector, err
}

func (db *DB) ListAIConnectors(ctx context.Context, userID string) ([]AIConnector, error) {
	rows, err := db.Read().QueryContext(ctx, `
		SELECT id, user_id, name, type, url, secret_encrypted, enabled, allow_private
		FROM ai_connectors WHERE user_id = ? ORDER BY name ASC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var connectors []AIConnector
	for rows.Next() {
		var connector AIConnector
		if err := rows.Scan(&connector.ID, &connector.UserID, &connector.Name, &connector.Type,
			&connector.URL, &connector.SecretEncrypted, &connector.Enabled, &connector.AllowPrivate); err != nil {
			return nil, err
		}
		connectors = append(connectors, connector)
	}
	return connectors, rows.Err()
}

func (db *DB) DeleteAIConnector(ctx context.Context, userID, id string) error {
	result, err := db.Write().ExecContext(ctx, `DELETE FROM ai_connectors WHERE id = ? AND user_id = ?`, id, userID)
	if err != nil {
		return err
	}
	if changed, _ := result.RowsAffected(); changed == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (db *DB) EnqueueAIRouting(ctx context.Context, userID string, item AIRoutingItem) (AIRoutingItem, error) {
	if item.MessageID <= 0 || item.IdempotencyKey == "" || item.PayloadJSON == "" {
		return AIRoutingItem{}, fmt.Errorf("routing item requires message, idempotency key and payload")
	}
	if item.EventType == "" {
		item.EventType = "message.routed"
	}
	if item.ID == "" {
		item.ID = uuid.NewString()
	}
	result, err := db.Write().ExecContext(ctx, `
		INSERT INTO ai_routing_outbox (
			id, message_id, project_id, connector_id, event_type, payload_json,
			idempotency_key, state, next_attempt_at
		)
		SELECT ?, m.id, ?, ?, ?, ?, ?, 'queued', CURRENT_TIMESTAMP
		FROM messages m
		JOIN accounts a ON a.id = m.account_id
		WHERE m.id = ? AND a.user_id = ?
		  AND (? = '' OR EXISTS (SELECT 1 FROM ai_projects p WHERE p.id = ? AND p.user_id = ?))
		  AND (? = '' OR EXISTS (SELECT 1 FROM ai_connectors c WHERE c.id = ? AND c.user_id = ?))
		ON CONFLICT(idempotency_key) DO NOTHING`,
		item.ID, nullableID(item.ProjectID), nullableID(item.ConnectorID),
		item.EventType, item.PayloadJSON, item.IdempotencyKey,
		item.MessageID, userID,
		item.ProjectID, item.ProjectID, userID,
		item.ConnectorID, item.ConnectorID, userID)
	if err != nil {
		return AIRoutingItem{}, err
	}
	if changed, _ := result.RowsAffected(); changed == 0 {
		existing, err := db.getAIRoutingByKey(ctx, userID, item.IdempotencyKey)
		if err == nil {
			return existing, nil
		}
		return AIRoutingItem{}, sql.ErrNoRows
	}
	return db.getAIRoutingByKey(ctx, userID, item.IdempotencyKey)
}

func (db *DB) getAIRoutingByKey(ctx context.Context, userID, key string) (AIRoutingItem, error) {
	var item AIRoutingItem
	var next, lease sql.NullString
	err := db.Read().QueryRowContext(ctx, `
		SELECT o.id, o.message_id, COALESCE(o.project_id, ''), COALESCE(o.connector_id, ''),
		       o.event_type, o.payload_json, o.idempotency_key, o.state, o.attempts,
		       o.next_attempt_at, o.lease_owner, o.lease_expires_at, o.response_status, o.last_error,
		       a.user_id
		FROM ai_routing_outbox o
		JOIN messages m ON m.id = o.message_id
		JOIN accounts a ON a.id = m.account_id
		WHERE o.idempotency_key = ? AND a.user_id = ?`, key, userID).Scan(
		&item.ID, &item.MessageID, &item.ProjectID, &item.ConnectorID, &item.EventType,
		&item.PayloadJSON, &item.IdempotencyKey, &item.State, &item.Attempts,
		&next, &item.LeaseOwner, &lease, &item.ResponseStatus, &item.LastError, &item.UserID)
	if err != nil {
		return AIRoutingItem{}, err
	}
	item.NextAttemptAt = parseLegitimacyTime(next)
	item.LeaseExpiresAt = parseLegitimacyTime(lease)
	return item, nil
}

func (db *DB) ClaimAIRouting(ctx context.Context, workerID string) (*AIRoutingItem, error) {
	if strings.TrimSpace(workerID) == "" {
		return nil, errors.New("routing worker id is required")
	}
	leaseUntil := formatDBTime(time.Now().UTC().Add(aiRoutingLease))
	var item AIRoutingItem
	err := db.Write().QueryRowContext(ctx, `
		UPDATE ai_routing_outbox
		SET state = 'running', lease_owner = ?, lease_expires_at = ?, attempts = attempts + 1,
			updated_at = CURRENT_TIMESTAMP
		WHERE id = (
			SELECT id FROM ai_routing_outbox
			WHERE state = 'queued' AND next_attempt_at <= CURRENT_TIMESTAMP
			ORDER BY next_attempt_at ASC, created_at ASC
			LIMIT 1
		)
		RETURNING id, message_id, COALESCE(project_id, ''), COALESCE(connector_id, ''),
			event_type, payload_json, idempotency_key, state, attempts, response_status, last_error`,
		workerID, leaseUntil).Scan(
		&item.ID, &item.MessageID, &item.ProjectID, &item.ConnectorID, &item.EventType,
		&item.PayloadJSON, &item.IdempotencyKey, &item.State, &item.Attempts,
		&item.ResponseStatus, &item.LastError)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("claim routing item: %w", err)
	}
	item.LeaseOwner = workerID
	return &item, nil
}

// ListAIRoutingStatus returns recent deliveries for a user without exposing
// the payload. It is used by the settings page to show retries and dead letters.
func (db *DB) ListAIRoutingStatus(ctx context.Context, userID string, limit int) ([]AIRoutingStatus, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	rows, err := db.Read().QueryContext(ctx, `
		SELECT o.id, COALESCE(p.slug, ''), COALESCE(c.name, ''), o.state,
		       o.attempts, o.response_status, o.last_error,
		       strftime('%Y-%m-%dT%H:%M:%SZ', o.updated_at)
		FROM ai_routing_outbox o
		JOIN messages m ON m.id = o.message_id
		JOIN accounts a ON a.id = m.account_id
		LEFT JOIN ai_projects p ON p.id = o.project_id
		LEFT JOIN ai_connectors c ON c.id = o.connector_id
		WHERE a.user_id = ?
		ORDER BY o.updated_at DESC, o.id DESC
		LIMIT ?`, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []AIRoutingStatus
	for rows.Next() {
		var item AIRoutingStatus
		if err := rows.Scan(&item.ID, &item.Project, &item.Connector, &item.State,
			&item.Attempts, &item.ResponseStatus, &item.LastError, &item.UpdatedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

// AIProjectAllowsExternalRouting reports whether the project still permits
// outbound routing. It is rechecked at dispatch time so revoking the
// permission also cancels queued deliveries.
func (db *DB) AIProjectAllowsExternalRouting(ctx context.Context, userID, projectID string) (bool, error) {
	var allowed bool
	err := db.Read().QueryRowContext(ctx, `
		SELECT allow_external_routing FROM ai_projects
		WHERE id = ? AND user_id = ? AND status = 'active'
		  AND lower(data_classification) IN ('public', 'internal')`, projectID, userID).Scan(&allowed)
	return allowed, err
}

// AIRoutingOwner resolves the owner of the message behind an outbox row.
func (db *DB) AIRoutingOwner(ctx context.Context, messageID int64) (string, error) {
	var userID string
	err := db.Read().QueryRowContext(ctx, `
		SELECT a.user_id FROM messages m JOIN accounts a ON a.id = m.account_id
		WHERE m.id = ?`, messageID).Scan(&userID)
	return userID, err
}

func (db *DB) CompleteAIRouting(ctx context.Context, id string, attempts int, status int) error {
	_, err := db.Write().ExecContext(ctx, `
		UPDATE ai_routing_outbox
		SET state = 'done', attempts = ?, response_status = ?, last_error = '',
			lease_owner = '', lease_expires_at = NULL, updated_at = CURRENT_TIMESTAMP
		WHERE id = ?`, attempts, status, id)
	return err
}

func (db *DB) FailAIRouting(ctx context.Context, id string, attempts int, status int, retryable bool, message string) error {
	state := "dead"
	next := time.Now().UTC()
	if retryable && attempts < aiRoutingMaxAttempts {
		state = "queued"
		next = next.Add(routingBackoff(attempts))
	}
	_, err := db.Write().ExecContext(ctx, `
		UPDATE ai_routing_outbox
		SET state = ?, attempts = ?, response_status = ?, last_error = ?,
			next_attempt_at = ?, lease_owner = '', lease_expires_at = NULL, updated_at = CURRENT_TIMESTAMP
		WHERE id = ?`, state, attempts, status, truncateError(message), formatDBTime(next), id)
	return err
}

func (db *DB) SetAIConnectorEnabled(ctx context.Context, userID, id string, enabled bool) error {
	result, err := db.Write().ExecContext(ctx, `
		UPDATE ai_connectors SET enabled = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = ? AND user_id = ?`, enabled, id, userID)
	if err != nil {
		return err
	}
	if changed, _ := result.RowsAffected(); changed == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (db *DB) DisableAIConnector(ctx context.Context, id string) error {
	_, err := db.Write().ExecContext(ctx, `UPDATE ai_connectors SET enabled = 0, updated_at = CURRENT_TIMESTAMP WHERE id = ?`, id)
	return err
}

func (db *DB) RecoverExpiredAIRouting(ctx context.Context) (int64, error) {
	result, err := db.Write().ExecContext(ctx, `
		UPDATE ai_routing_outbox
		SET state = CASE WHEN attempts + 1 >= ? THEN 'dead' ELSE 'queued' END,
			attempts = attempts + 1, lease_owner = '', lease_expires_at = NULL,
			next_attempt_at = CURRENT_TIMESTAMP, updated_at = CURRENT_TIMESTAMP
		WHERE state = 'running' AND lease_expires_at IS NOT NULL
		  AND lease_expires_at <= CURRENT_TIMESTAMP`, aiRoutingMaxAttempts)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func routingBackoff(attempts int) time.Duration {
	delay := time.Duration(1<<minRoutingAttempt(attempts, 7)) * 30 * time.Second
	if delay > time.Hour {
		delay = time.Hour
	}
	return delay
}

func minRoutingAttempt(attempts, max int) int {
	if attempts < 1 {
		return 1
	}
	if attempts > max {
		return max
	}
	return attempts
}

func nullableID(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func truncateError(value string) string {
	value = strings.TrimSpace(value)
	runes := []rune(value)
	if len(runes) > 500 {
		return string(runes[:500])
	}
	return value
}
