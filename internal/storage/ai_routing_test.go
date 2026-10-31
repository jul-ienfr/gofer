package storage

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"
)

func TestAIRoutingOutboxIsIdempotentAndScoped(t *testing.T) {
	ctx := context.Background()
	db := newContactsTestDB(t)
	if _, err := db.Write().Exec(`
		INSERT INTO accounts (id, user_id, email_address, provider) VALUES ('acc', 'default', 'ana@example.com', 'imap');
		INSERT INTO messages (id, account_id, internet_message_id, subject) VALUES (1, 'acc', '<one@example.com>', 'One');
		INSERT INTO users (id, username, username_normalized, name) VALUES ('other', 'other', 'other', 'Other');`); err != nil {
		t.Fatal(err)
	}
	project, err := db.CreateAIProject(ctx, AIProject{UserID: "default", Name: "Employment", Slug: "employment"})
	if err != nil {
		t.Fatal(err)
	}
	connector, err := db.CreateAIConnector(ctx, AIConnector{
		UserID: "default", Name: "n8n", Type: "n8n", URL: "https://hooks.example.test/hook", SecretEncrypted: "sealed", Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	item, err := db.EnqueueAIRouting(ctx, "default", AIRoutingItem{
		MessageID: 1, ProjectID: project.ID, ConnectorID: connector.ID,
		PayloadJSON: `{"event":"routed"}`, IdempotencyKey: "evt-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if item.State != "queued" || item.UserID != "default" || item.Attempts != 0 {
		t.Fatalf("item = %+v", item)
	}
	again, err := db.EnqueueAIRouting(ctx, "default", AIRoutingItem{
		MessageID: 1, ProjectID: project.ID, ConnectorID: connector.ID,
		PayloadJSON: `{"event":"routed"}`, IdempotencyKey: "evt-1",
	})
	if err != nil || again.ID != item.ID {
		t.Fatalf("duplicate = %+v, %v", again, err)
	}
	if _, err := db.EnqueueAIRouting(ctx, "other", AIRoutingItem{
		MessageID: 1, ConnectorID: connector.ID, PayloadJSON: `{}`, IdempotencyKey: "evt-stolen",
	}); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("cross-user connector = %v", err)
	}
	claimed, err := db.ClaimAIRouting(ctx, "worker")
	if err != nil || claimed == nil || claimed.ID != item.ID || claimed.Attempts != 1 {
		t.Fatalf("claimed = %+v, %v", claimed, err)
	}
	if next, err := db.ClaimAIRouting(ctx, "worker"); err != nil || next != nil {
		t.Fatalf("second claim = %+v, %v", next, err)
	}
	if err := db.CompleteAIRouting(ctx, item.ID, 1, 204); err != nil {
		t.Fatal(err)
	}
	done, err := db.getAIRoutingByKey(ctx, "default", "evt-1")
	if err != nil || done.State != "done" || done.ResponseStatus != 204 {
		t.Fatalf("done = %+v, %v", done, err)
	}
}

func TestAIRoutingRetriesDeadLettersAndRecoversLeases(t *testing.T) {
	ctx := context.Background()
	db := newContactsTestDB(t)
	if _, err := db.Write().Exec(`
		INSERT INTO accounts (id, user_id, email_address, provider) VALUES ('acc', 'default', 'ana@example.com', 'imap');
		INSERT INTO messages (id, account_id, internet_message_id, subject) VALUES (1, 'acc', '<one@example.com>', 'One');`); err != nil {
		t.Fatal(err)
	}
	item, err := db.EnqueueAIRouting(ctx, "default", AIRoutingItem{MessageID: 1, PayloadJSON: `{}`, IdempotencyKey: "evt-2"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ClaimAIRouting(ctx, "worker"); err != nil {
		t.Fatal(err)
	}
	if err := db.FailAIRouting(ctx, item.ID, 1, 500, true, "upstream down"); err != nil {
		t.Fatal(err)
	}
	retry, err := db.getAIRoutingByKey(ctx, "default", "evt-2")
	if err != nil || retry.State != "queued" || !retry.NextAttemptAt.After(time.Now()) {
		t.Fatalf("retry = %+v, %v", retry, err)
	}
	if err := db.FailAIRouting(ctx, item.ID, 8, 500, true, "still down"); err != nil {
		t.Fatal(err)
	}
	dead, err := db.getAIRoutingByKey(ctx, "default", "evt-2")
	if err != nil || dead.State != "dead" {
		t.Fatalf("dead = %+v, %v", dead, err)
	}
	if _, err := db.Write().Exec(`UPDATE ai_routing_outbox SET state = 'running', lease_expires_at = datetime('now', '-1 minute') WHERE id = ?`, item.ID); err != nil {
		t.Fatal(err)
	}
	recovered, err := db.RecoverExpiredAIRouting(ctx)
	if err != nil || recovered != 1 {
		t.Fatalf("recovered = %d, %v", recovered, err)
	}
	after, err := db.getAIRoutingByKey(ctx, "default", "evt-2")
	if err != nil || after.State != "dead" || after.Attempts != 9 {
		t.Fatalf("after recovery = %+v, %v", after, err)
	}
}
