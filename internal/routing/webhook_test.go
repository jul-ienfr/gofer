package routing

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/cristianadrielbraun/gofer/internal/netguard"
)

func TestSendWebhookSignsTimestampAndIdempotencyKey(t *testing.T) {
	var signature, timestamp, idempotency, body string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		signature = r.Header.Get("X-Gofer-Signature")
		timestamp = r.Header.Get("X-Gofer-Timestamp")
		idempotency = r.Header.Get("Idempotency-Key")
		raw, _ := io.ReadAll(r.Body)
		body = string(raw)
		if !VerifyWebhookSignature("secret", timestamp, raw, signature) {
			t.Error("invalid signature")
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	restore := netguard.UseTransportForTests(http.DefaultTransport)
	defer restore()
	payload := []byte(`{"event_id":"evt-1"}`)
	result, err := SendWebhook(context.Background(), WebhookRequest{
		URL: server.URL, Secret: "secret", Payload: payload, IdempotencyKey: "evt-1",
		AllowPrivate: true,
		Now:          time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.StatusCode != http.StatusOK || body != string(payload) {
		t.Fatalf("result = %+v body = %q", result, body)
	}
	if timestamp != "2026-10-29T12:00:00Z" || idempotency != "evt-1" || !strings.HasPrefix(signature, "sha256=") {
		t.Fatalf("headers = %q %q %q", timestamp, idempotency, signature)
	}
	if _, err := SendWebhook(context.Background(), WebhookRequest{URL: "http://example.com/hook", Secret: "secret", Payload: payload}); err == nil {
		t.Fatal("plain HTTP remote target accepted")
	}
}

func TestSendWebhookClassifiesRetriesAndDeadLetters(t *testing.T) {
	cases := []struct {
		status    int
		retryable bool
		dead      bool
	}{
		{http.StatusInternalServerError, true, false},
		{http.StatusTooManyRequests, true, false},
		{http.StatusRequestTimeout, true, false},
		{http.StatusBadRequest, false, true},
		{http.StatusUnauthorized, false, true},
		{http.StatusGone, false, true},
		{http.StatusFound, false, true},
	}
	for _, test := range cases {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if test.status == http.StatusFound {
				http.Redirect(w, r, "/elsewhere", http.StatusFound)
				return
			}
			w.WriteHeader(test.status)
		}))
		restore := netguard.UseTransportForTests(http.DefaultTransport)
		_, err := SendWebhook(context.Background(), WebhookRequest{URL: server.URL, Secret: "secret", Payload: []byte(`{}`), AllowPrivate: true})
		restore()
		server.Close()
		var webhookErr *WebhookError
		if !errors.As(err, &webhookErr) {
			t.Errorf("status %d error = %v", test.status, err)
			continue
		}
		if webhookErr.Retryable != test.retryable || webhookErr.Dead != test.dead {
			t.Errorf("status %d error = %+v", test.status, webhookErr)
		}
	}
}

func TestWebhookAllowsPrivateOnlyWithExplicitPermission(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	if _, err := SendWebhook(context.Background(), WebhookRequest{URL: server.URL, Secret: "secret", Payload: []byte(`{}`)}); err == nil {
		t.Fatal("private request without permission was accepted")
	}
	if _, err := SendWebhook(context.Background(), WebhookRequest{URL: server.URL, Secret: "secret", Payload: []byte(`{}`), AllowPrivate: true}); err != nil {
		t.Fatalf("private request = %v", err)
	}
}

func TestVerifyWebhookSignatureRejectsTampering(t *testing.T) {
	payload := []byte(`{"a":1}`)
	timestamp := "2026-10-29T12:00:00Z"
	valid := "sha256=0b9f6e3a3f5c0e6f0f1e0f0e0f0e0f0e0f0e0f0e0f0e0f0e0f0e0f0e0f0e0f0e"
	if VerifyWebhookSignature("secret", timestamp, payload, valid) {
		t.Fatal("forged signature accepted")
	}
}
