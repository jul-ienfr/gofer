package airouting

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cristianadrielbraun/gofer/internal/storage"
)

func routingFixture(t *testing.T) (*storage.DB, storage.AIConnector) {
	t.Helper()
	db, err := storage.New(filepath.Join(t.TempDir(), "gofer.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.Write().Exec(`
		INSERT INTO users (id, username, username_normalized, name) VALUES ('default', 'default', 'default', 'Default');
		INSERT INTO accounts (id, user_id, email_address, provider) VALUES ('acc', 'default', 'ana@example.com', 'imap');
		INSERT INTO folders (id, account_id, remote_id, name, role) VALUES ('acc:INBOX', 'acc', 'INBOX', 'Inbox', 'inbox');
		INSERT INTO messages (id, account_id, internet_message_id, subject, from_email)
			VALUES (1, 'acc', '<one@example.com>', 'Hello', 'alice@example.com');
		INSERT INTO message_folder_state (message_id, folder_id, is_deleted) VALUES (1, 'acc:INBOX', 0);`); err != nil {
		t.Fatal(err)
	}
	project, err := db.CreateAIProject(context.Background(), storage.AIProject{
		UserID: "default", Name: "Work", Slug: "work", Status: "active", AllowExternalRouting: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	connector, err := db.CreateAIConnector(context.Background(), storage.AIConnector{
		UserID: "default", Name: "hook", Type: "webhook", URL: "http://127.0.0.1/hook",
		SecretEncrypted: "sealed", Enabled: true, AllowPrivate: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.EnqueueAIRouting(context.Background(), "default", storage.AIRoutingItem{
		MessageID: 1, ProjectID: project.ID, ConnectorID: connector.ID, PayloadJSON: `{"version":"routing-v1.0.0"}`,
		IdempotencyKey: "route:1",
	}); err != nil {
		t.Fatal(err)
	}
	return db, connector
}

func TestDispatcherSignsAndCompletesWebhook(t *testing.T) {
	db, connector := routingFixture(t)
	var received string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		received = string(body)
		if r.Header.Get("X-Gofer-Signature") == "" || r.Header.Get("Idempotency-Key") != "route:1" {
			t.Errorf("headers = %v", r.Header)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	if _, err := db.Write().Exec(`UPDATE ai_connectors SET url = ? WHERE id = ?`, server.URL, connector.ID); err != nil {
		t.Fatal(err)
	}
	dispatcher := &Dispatcher{
		DB: db, WorkerID: "test",
		Resolve: func(_ context.Context, userID, connectorID, encrypted string) (string, error) {
			if userID != "default" || connectorID != connector.ID || encrypted != "sealed" {
				t.Fatalf("resolve args = %q %q %q", userID, connectorID, encrypted)
			}
			return "shared-secret", nil
		},
	}
	worked, err := dispatcher.ProcessOnce(t.Context())
	if err != nil || !worked {
		t.Fatalf("ProcessOnce = %t, %v", worked, err)
	}
	if received != `{"version":"routing-v1.0.0"}` {
		t.Fatalf("payload = %q", received)
	}
	var state string
	if err := db.Read().QueryRow(`SELECT state FROM ai_routing_outbox WHERE idempotency_key = 'route:1'`).Scan(&state); err != nil || state != "done" {
		t.Fatalf("state = %q, %v", state, err)
	}
}

func TestDispatcherRetriesServerErrors(t *testing.T) {
	db, connector := routingFixture(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "try later", http.StatusInternalServerError)
	}))
	defer server.Close()
	if _, err := db.Write().Exec(`UPDATE ai_connectors SET url = ? WHERE id = ?`, server.URL, connector.ID); err != nil {
		t.Fatal(err)
	}
	dispatcher := &Dispatcher{
		DB: db, WorkerID: "test",
		Resolve: func(context.Context, string, string, string) (string, error) { return "shared-secret", nil },
	}
	worked, err := dispatcher.ProcessOnce(t.Context())
	if err != nil || !worked {
		t.Fatalf("ProcessOnce = %t, %v", worked, err)
	}
	var state string
	var attempts int
	if err := db.Read().QueryRow(`SELECT state, attempts FROM ai_routing_outbox WHERE idempotency_key = 'route:1'`).Scan(&state, &attempts); err != nil {
		t.Fatal(err)
	}
	if state != "queued" || attempts != 1 {
		t.Fatalf("state=%q attempts=%d", state, attempts)
	}
}

func TestDispatcherStopsWhenProjectRoutingIsRevoked(t *testing.T) {
	db, _ := routingFixture(t)
	if _, err := db.Write().Exec(`UPDATE ai_projects SET allow_external_routing = 0`); err != nil {
		t.Fatal(err)
	}
	dispatcher := &Dispatcher{DB: db, WorkerID: "test"}
	worked, err := dispatcher.ProcessOnce(t.Context())
	if err != nil || !worked {
		t.Fatalf("ProcessOnce = %t, %v", worked, err)
	}
	var state, lastError string
	if err := db.Read().QueryRow(`SELECT state, last_error FROM ai_routing_outbox WHERE idempotency_key = 'route:1'`).Scan(&state, &lastError); err != nil {
		t.Fatal(err)
	}
	if state != "dead" || !strings.Contains(lastError, "external routing") {
		t.Fatalf("state=%q error=%q", state, lastError)
	}
}

func TestDispatcherDeadLettersDisabledConnector(t *testing.T) {
	db, connector := routingFixture(t)
	if err := db.SetAIConnectorEnabled(t.Context(), "default", connector.ID, false); err != nil {
		t.Fatal(err)
	}
	dispatcher := &Dispatcher{DB: db, WorkerID: "test"}
	worked, err := dispatcher.ProcessOnce(t.Context())
	if err != nil || !worked {
		t.Fatalf("ProcessOnce = %t, %v", worked, err)
	}
	var state, lastError string
	if err := db.Read().QueryRow(`SELECT state, last_error FROM ai_routing_outbox WHERE idempotency_key = 'route:1'`).Scan(&state, &lastError); err != nil {
		t.Fatal(err)
	}
	if state != "dead" || !strings.Contains(lastError, "disabled") {
		t.Fatalf("state=%q error=%q", state, lastError)
	}
	deliveries, err := db.ListAIRoutingStatus(t.Context(), "default", 10)
	if err != nil || len(deliveries) != 1 || deliveries[0].State != "dead" || deliveries[0].Connector != "hook" {
		t.Fatalf("deliveries = %+v, %v", deliveries, err)
	}
}
