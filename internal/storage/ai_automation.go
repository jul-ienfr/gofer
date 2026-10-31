package storage

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"
)

type AIAutomationAction struct {
	ID           string
	MessageID    int64
	Kind         string
	Mode         string
	State        string
	GateJSON     string
	Confidence   float64
	Reason       string
	ExecutedAt   time.Time
	UndoDeadline time.Time
	UndoneAt     time.Time
	Result       string
}

func (db *DB) RecordAIAutomationAction(ctx context.Context, action AIAutomationAction) (string, error) {
	if action.ID == "" {
		action.ID = uuid.NewString()
	}
	if action.Mode == "" {
		action.Mode = "auto"
	}
	if action.State == "" {
		action.State = "executed"
	}
	_, err := db.Write().ExecContext(ctx, `
		INSERT INTO ai_automation_actions
			(id, message_id, kind, mode, state, gate_json, confidence, reason, executed_at, undo_deadline, result)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		action.ID, action.MessageID, action.Kind, action.Mode, action.State, action.GateJSON,
		action.Confidence, action.Reason, action.ExecutedAt.UTC(), action.UndoDeadline.UTC(), action.Result)
	if err != nil {
		return "", err
	}
	return action.ID, nil
}

func (db *DB) GetAIAutomationAction(ctx context.Context, userID, id string) (AIAutomationAction, error) {
	var action AIAutomationAction
	var executed, deadline, undone *string
	err := db.Read().QueryRowContext(ctx, `
		SELECT x.id, x.message_id, x.kind, x.mode, x.state, x.gate_json, x.confidence,
		       x.reason, x.executed_at, x.undo_deadline, x.undone_at, x.result
		FROM ai_automation_actions x
		JOIN messages m ON m.id = x.message_id
		JOIN accounts a ON a.id = m.account_id
		WHERE x.id = ? AND a.user_id = ?`, id, userID).Scan(
		&action.ID, &action.MessageID, &action.Kind, &action.Mode, &action.State, &action.GateJSON,
		&action.Confidence, &action.Reason, &executed, &deadline, &undone, &action.Result)
	if err != nil {
		return AIAutomationAction{}, err
	}
	action.ExecutedAt = parseAIWhen(executed)
	action.UndoDeadline = parseAIWhen(deadline)
	action.UndoneAt = parseAIWhen(undone)
	return action, nil
}

func (db *DB) LatestAIAutomationForMessage(ctx context.Context, userID string, messageID int64) (AIAutomationAction, error) {
	var action AIAutomationAction
	var executed, deadline, undone *string
	err := db.Read().QueryRowContext(ctx, `
		SELECT x.id, x.message_id, x.kind, x.mode, x.state, x.confidence, x.reason,
		       x.executed_at, x.undo_deadline, x.undone_at, x.result
		FROM ai_automation_actions x
		JOIN messages m ON m.id = x.message_id
		JOIN accounts a ON a.id = m.account_id
		WHERE x.message_id = ? AND a.user_id = ?
		ORDER BY x.created_at DESC, x.id DESC
		LIMIT 1`, messageID, userID).Scan(&action.ID, &action.MessageID, &action.Kind, &action.Mode,
		&action.State, &action.Confidence, &action.Reason, &executed, &deadline, &undone, &action.Result)
	if err != nil {
		return AIAutomationAction{}, err
	}
	action.ExecutedAt = parseAIWhen(executed)
	action.UndoDeadline = parseAIWhen(deadline)
	action.UndoneAt = parseAIWhen(undone)
	return action, nil
}

func (db *DB) ListAIAutomationActions(ctx context.Context, userID string, limit int) ([]AIAutomationAction, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	rows, err := db.Read().QueryContext(ctx, `
		SELECT x.id, x.message_id, x.kind, x.mode, x.state, x.confidence, x.reason,
		       x.executed_at, x.undo_deadline, x.undone_at, x.result
		FROM ai_automation_actions x
		JOIN messages m ON m.id = x.message_id
		JOIN accounts a ON a.id = m.account_id
		WHERE a.user_id = ?
		ORDER BY x.created_at DESC, x.id DESC
		LIMIT ?`, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var actions []AIAutomationAction
	for rows.Next() {
		var action AIAutomationAction
		var executed, deadline, undone *string
		if err := rows.Scan(&action.ID, &action.MessageID, &action.Kind, &action.Mode, &action.State,
			&action.Confidence, &action.Reason, &executed, &deadline, &undone, &action.Result); err != nil {
			return nil, err
		}
		action.ExecutedAt = parseAIWhen(executed)
		action.UndoDeadline = parseAIWhen(deadline)
		action.UndoneAt = parseAIWhen(undone)
		actions = append(actions, action)
	}
	return actions, rows.Err()
}

func (db *DB) MarkAIAutomationUndone(ctx context.Context, id, result string) error {
	res, err := db.Write().ExecContext(ctx, `
		UPDATE ai_automation_actions
		SET state = 'undone', undone_at = CURRENT_TIMESTAMP, result = ?
		WHERE id = ? AND state = 'executed'`, result, id)
	if err != nil {
		return err
	}
	changed, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if changed == 0 {
		return ErrMessageMutationSuperseded
	}
	return nil
}

func parseAIWhen(value *string) time.Time {
	if value == nil || strings.TrimSpace(*value) == "" {
		return time.Time{}
	}
	for _, layout := range []string{"2006-01-02 15:04:05", time.RFC3339, "2006-01-02T15:04:05Z"} {
		if parsed, err := time.Parse(layout, *value); err == nil {
			return parsed.UTC()
		}
	}
	return time.Time{}
}
