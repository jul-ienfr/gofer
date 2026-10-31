package unsubscribe

import (
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cristianadrielbraun/gofer/internal/storage"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) { return f(request) }

func TestParseTargetRequiresOneClickHTTPS(t *testing.T) {
	if _, _, ok := ParseTarget("<https://example.com/unsub>", ""); ok {
		t.Fatal("accepted a target without List-Unsubscribe-Post")
	}
	if _, _, ok := ParseTarget("<http://example.com/unsub>", "List-Unsubscribe=One-Click"); ok {
		t.Fatal("accepted an HTTP target")
	}
	if _, _, ok := ParseTarget("<mailto:list@example.com>", "List-Unsubscribe=One-Click"); ok {
		t.Fatal("accepted a mailto target")
	}
	target, headerHash, ok := ParseTarget("<mailto:list@example.com>, <https://example.com/unsub>", "List-Unsubscribe=One-Click")
	if !ok || target != "https://example.com/unsub" || headerHash == "" {
		t.Fatalf("target = %q, hash = %q, ok = %t", target, headerHash, ok)
	}
}

func TestAttemptSendsOnceAndRecordsTheResult(t *testing.T) {
	db, messageID := unsubscribeFixture(t, `List-Unsubscribe: <https://example.com/unsub>
List-Unsubscribe-Post: List-Unsubscribe=One-Click
Subject: Spam
From: spam@example.com

body`)
	var requests int
	var sentBody string
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		requests++
		data, _ := io.ReadAll(request.Body)
		sentBody = string(data)
		if request.Method != http.MethodPost || request.Header.Get("Content-Type") != "application/x-www-form-urlencoded" {
			t.Fatalf("request = %s %s", request.Method, request.Header.Get("Content-Type"))
		}
		return &http.Response{StatusCode: http.StatusNoContent, Body: io.NopCloser(strings.NewReader("")), Header: make(http.Header)}, nil
	})}
	result, err := attempt(context.Background(), db, "default", messageID, client)
	if err != nil || !result.Attempted || result.StatusCode != http.StatusNoContent {
		t.Fatalf("result = %+v, %v", result, err)
	}
	if requests != 1 || sentBody != "List-Unsubscribe=One-Click" {
		t.Fatalf("requests = %d, body = %q", requests, sentBody)
	}
	result, err = attempt(context.Background(), db, "default", messageID, client)
	if err != nil || result.Attempted || requests != 1 {
		t.Fatalf("second attempt = %+v, %v, requests = %d", result, err, requests)
	}
	var status string
	if err := db.Read().QueryRow(`SELECT status FROM ai_unsubscribe_attempts WHERE message_id = ?`, messageID).Scan(&status); err != nil || status != "sent" {
		t.Fatalf("stored status = %q, %v", status, err)
	}
	items, err := db.ListAIUnsubscribeStatus(context.Background(), "default", 10)
	if err != nil || len(items) != 1 || items[0].Status != "sent" || items[0].HTTPStatus != http.StatusNoContent {
		t.Fatalf("listed attempts = %+v, %v", items, err)
	}
}

func TestAttemptRecordsHTTPFailureAndCoolsDown(t *testing.T) {
	db, messageID := unsubscribeFixture(t, `List-Unsubscribe: <https://example.com/unsub>
List-Unsubscribe-Post: List-Unsubscribe=One-Click
Subject: Spam

body`)
	var requests int
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		requests++
		return &http.Response{StatusCode: http.StatusInternalServerError, Body: io.NopCloser(strings.NewReader("broken")), Header: make(http.Header)}, nil
	})}
	result, err := attempt(context.Background(), db, "default", messageID, client)
	if err == nil || !result.Attempted || result.StatusCode != http.StatusInternalServerError {
		t.Fatalf("result = %+v, %v", result, err)
	}
	result, err = attempt(context.Background(), db, "default", messageID, client)
	if err != nil || result.Attempted || requests != 1 {
		t.Fatalf("second result = %+v, %v, requests = %d", result, err, requests)
	}
	var status string
	if err := db.Read().QueryRow(`SELECT status FROM ai_unsubscribe_attempts WHERE message_id = ?`, messageID).Scan(&status); err != nil || status != "failed" {
		t.Fatalf("status = %q, %v", status, err)
	}
}

func TestAttemptSkipsWithoutOneClick(t *testing.T) {
	db, messageID := unsubscribeFixture(t, `List-Unsubscribe: <https://example.com/unsub>
Subject: Spam

body`)
	result, err := attempt(context.Background(), db, "default", messageID, nil)
	if err != nil || result.Attempted || result.Reason != "no one-click target" {
		t.Fatalf("result = %+v, %v", result, err)
	}
}

func unsubscribeFixture(t *testing.T, raw string) (*storage.DB, int64) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "message.eml")
	if err := os.WriteFile(path, []byte(raw), 0600); err != nil {
		t.Fatal(err)
	}
	db, err := storage.New(filepath.Join(dir, "gofer.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.Write().Exec(`
		INSERT INTO users (id, username, username_normalized, name) VALUES ('default', 'default', 'default', 'Default');
		INSERT INTO accounts (id, user_id, email_address, provider) VALUES ('acc', 'default', 'ana@example.com', 'imap');
		INSERT INTO folders (id, account_id, remote_id, name, role) VALUES ('acc:INBOX', 'acc', 'INBOX', 'Inbox', 'inbox');
		INSERT INTO messages (id, account_id, internet_message_id, subject, from_email, raw_path)
			VALUES (1, 'acc', '<spam@example.com>', 'Spam', 'spam@example.com', ?);
		INSERT INTO message_folder_state (message_id, folder_id, is_deleted) VALUES (1, 'acc:INBOX', 0);`, path); err != nil {
		t.Fatal(err)
	}
	return db, 1
}
