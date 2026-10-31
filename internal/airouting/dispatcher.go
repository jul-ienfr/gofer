package airouting

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/cristianadrielbraun/gofer/internal/aiaudit"
	"github.com/cristianadrielbraun/gofer/internal/routing"
	"github.com/cristianadrielbraun/gofer/internal/storage"
)

// SecretResolver opens a connector signing secret for one owner.
type SecretResolver func(ctx context.Context, userID, connectorID, encrypted string) (string, error)

type Dispatcher struct {
	DB        *storage.DB
	WorkerID  string
	Resolve   SecretResolver
	Now       func() time.Time
	Logf      func(format string, args ...any)
	Interval  time.Duration
	BatchSize int
}

func (d *Dispatcher) Run(ctx context.Context) {
	if d == nil || d.DB == nil {
		return
	}
	interval := d.Interval
	if interval <= 0 {
		interval = 15 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		if _, err := d.ProcessOnce(ctx); err != nil && ctx.Err() == nil && d.Logf != nil {
			d.Logf("ai routing: %v", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (d *Dispatcher) ProcessOnce(ctx context.Context) (bool, error) {
	if d == nil || d.DB == nil {
		return false, errors.New("AI routing dispatcher requires a database")
	}
	if _, err := d.DB.RecoverExpiredAIRouting(ctx); err != nil {
		return false, err
	}
	workerID := strings.TrimSpace(d.WorkerID)
	if workerID == "" {
		workerID = "gofer-routing"
	}
	item, err := d.DB.ClaimAIRouting(ctx, workerID)
	if err != nil {
		return false, err
	}
	if item == nil {
		return false, nil
	}
	worked := true
	userID, err := d.DB.AIRoutingOwner(ctx, item.MessageID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			_ = d.DB.FailAIRouting(ctx, item.ID, item.Attempts, 0, false, "message no longer exists")
			d.audit(ctx, "", item, "dead", 0)
			return worked, nil
		}
		return worked, err
	}
	allowed, err := d.DB.AIProjectAllowsExternalRouting(ctx, userID, item.ProjectID)
	if err != nil || !allowed {
		_ = d.DB.FailAIRouting(ctx, item.ID, item.Attempts, 0, false, "project no longer allows external routing")
		d.audit(ctx, userID, item, "dead", 0)
		return worked, nil
	}
	connector, err := d.DB.GetAIConnector(ctx, userID, item.ConnectorID)
	if err != nil {
		_ = d.DB.FailAIRouting(ctx, item.ID, item.Attempts, 0, false, "connector not found")
		d.audit(ctx, userID, item, "dead", 0)
		return worked, nil
	}
	if !connector.Enabled {
		_ = d.DB.FailAIRouting(ctx, item.ID, item.Attempts, 0, false, "connector is disabled")
		d.audit(ctx, userID, item, "dead", 0)
		return worked, nil
	}
	if connector.SecretEncrypted == "" || d.Resolve == nil {
		_ = d.DB.FailAIRouting(ctx, item.ID, item.Attempts, 0, false, "connector secret is unavailable")
		d.audit(ctx, userID, item, "dead", 0)
		return worked, nil
	}
	secret, err := d.Resolve(ctx, userID, connector.ID, connector.SecretEncrypted)
	if err != nil || strings.TrimSpace(secret) == "" {
		_ = d.DB.DisableAIConnector(ctx, connector.ID)
		_ = d.DB.FailAIRouting(ctx, item.ID, item.Attempts, 0, false, "connector secret cannot be decrypted")
		d.audit(ctx, userID, item, "dead", 0)
		return worked, nil
	}
	now := time.Now().UTC()
	if d.Now != nil {
		now = d.Now()
	}
	result, sendErr := routing.SendWebhook(ctx, routing.WebhookRequest{
		URL: connector.URL, Secret: secret, Payload: []byte(item.PayloadJSON),
		IdempotencyKey: item.IdempotencyKey, AllowPrivate: connector.AllowPrivate, Now: now,
	})
	if sendErr == nil {
		if err := d.DB.CompleteAIRouting(ctx, item.ID, item.Attempts, result.StatusCode); err != nil {
			return worked, err
		}
		d.audit(ctx, userID, item, "done", result.StatusCode)
		return worked, nil
	}
	retryable, status := true, 0
	var webhookErr *routing.WebhookError
	if errors.As(sendErr, &webhookErr) {
		retryable = webhookErr.Retryable && !webhookErr.Dead
		status = webhookErr.StatusCode
	}
	if err := d.DB.FailAIRouting(ctx, item.ID, item.Attempts, status, retryable, sendErr.Error()); err != nil {
		return worked, err
	}
	d.audit(ctx, userID, item, "failed", status)
	if d.Logf != nil {
		d.Logf("ai routing: connector=%s status=%d retryable=%t: %v", connector.ID, status, retryable, sendErr)
	}
	return worked, nil
}

func (d *Dispatcher) audit(ctx context.Context, userID string, item *storage.AIRoutingItem, state string, status int) {
	if userID == "" {
		return
	}
	payload, err := json.Marshal(map[string]any{
		"connector_id": item.ConnectorID, "project_id": item.ProjectID,
		"event_type": item.EventType, "state": state, "status": status,
		"attempts": item.Attempts,
	})
	if err != nil {
		return
	}
	if _, err := aiaudit.Record(ctx, d.DB, aiaudit.Event{
		UserID: userID, ActorType: aiaudit.ActorSystem, Action: "ai.routing.dispatch",
		SubjectType: "message", SubjectID: fmt.Sprintf("%d", item.MessageID), AfterJSON: string(payload),
	}); err != nil && d.Logf != nil {
		d.Logf("ai routing audit message=%d: %v", item.MessageID, err)
	}
}
