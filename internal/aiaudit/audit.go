package aiaudit

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/cristianadrielbraun/gofer/internal/storage"
)

const (
	ActorSystem = "system"
	ActorUser   = "user"
	ActorAPI    = "api"
)

type Event struct {
	ID          string    `json:"id"`
	UserID      string    `json:"user_id"`
	ActorType   string    `json:"actor_type"`
	ActorID     string    `json:"actor_id"`
	Action      string    `json:"action"`
	SubjectType string    `json:"subject_type"`
	SubjectID   string    `json:"subject_id"`
	BeforeJSON  string    `json:"before_json"`
	AfterJSON   string    `json:"after_json"`
	IP          string    `json:"ip"`
	UserAgent   string    `json:"user_agent"`
	CreatedAt   time.Time `json:"created_at"`
}

func Record(ctx context.Context, db *storage.DB, event Event) (string, error) {
	if db == nil {
		return "", errors.New("audit requires a database")
	}
	event.UserID = strings.TrimSpace(event.UserID)
	if event.UserID == "" {
		return "", errors.New("audit requires a user id")
	}
	event.Action = strings.TrimSpace(event.Action)
	if event.Action == "" {
		return "", errors.New("audit requires an action")
	}
	if event.ActorType == "" {
		event.ActorType = ActorSystem
	}
	if event.ActorType != ActorSystem && event.ActorType != ActorUser && event.ActorType != ActorAPI {
		return "", errors.New("audit actor type is invalid")
	}
	before, err := canonicalJSON(event.BeforeJSON)
	if err != nil {
		return "", err
	}
	after, err := canonicalJSON(event.AfterJSON)
	if err != nil {
		return "", err
	}
	if event.ID == "" {
		event.ID = uuid.NewString()
	}
	_, err = db.Write().ExecContext(ctx, `
		INSERT INTO ai_audit_events
			(id, user_id, actor_type, actor_id, action, subject_type, subject_id, before_json, after_json, ip, user_agent)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		event.ID, event.UserID, event.ActorType, event.ActorID, event.Action,
		event.SubjectType, event.SubjectID, before, after, event.IP, event.UserAgent)
	if err != nil {
		return "", err
	}
	return event.ID, nil
}

func List(ctx context.Context, db *storage.DB, userID string, limit int) ([]Event, error) {
	if db == nil {
		return nil, errors.New("audit requires a database")
	}
	if strings.TrimSpace(userID) == "" {
		return nil, errors.New("audit requires a user id")
	}
	if limit <= 0 {
		limit = 100
	}
	if limit > 500 {
		limit = 500
	}
	rows, err := db.Read().QueryContext(ctx, `
		SELECT id, user_id, actor_type, actor_id, action, subject_type, subject_id,
			before_json, after_json, ip, user_agent, created_at
		FROM ai_audit_events
		WHERE user_id = ?
		ORDER BY created_at DESC, rowid DESC
		LIMIT ?`, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	events := make([]Event, 0, limit)
	for rows.Next() {
		var event Event
		if err := rows.Scan(&event.ID, &event.UserID, &event.ActorType, &event.ActorID, &event.Action,
			&event.SubjectType, &event.SubjectID, &event.BeforeJSON, &event.AfterJSON,
			&event.IP, &event.UserAgent, &event.CreatedAt); err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	return events, rows.Err()
}

func canonicalJSON(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "{}", nil
	}
	if !json.Valid([]byte(raw)) {
		return "", errors.New("audit payload is not valid JSON")
	}
	var value any
	if err := json.Unmarshal([]byte(raw), &value); err != nil {
		return "", err
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}
