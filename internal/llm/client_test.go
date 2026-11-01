package llm

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

const validDraft = `{"reply_text":"Bonjour, je suis disponible mardi.","language":"fr","confidence":0.82,"needs_human":false,"reason":"simple scheduling"}`

func TestDraftSendsTheOpenAICompatibleRequest(t *testing.T) {
	var path, auth string
	var payload map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path, auth = r.URL.Path, r.Header.Get("Authorization")
		body, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(body, &payload); err != nil {
			t.Error(err)
		}
		_, _ = io.WriteString(w, `{"model":"test-model","choices":[{"message":{"content":"`+strings.ReplaceAll(validDraft, `"`, `\"`)+`"}}],"usage":{"prompt_tokens":120,"completion_tokens":30}}`)
	}))
	defer server.Close()
	client, err := NewClient(Config{BaseURL: server.URL + "/v1", APIKey: "secret", Model: "test-model", Timeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	draft, completion, err := client.Draft(context.Background(), DraftInput{
		Instruction: "Reply briefly",
		Thread:      []ThreadMessage{{From: "client@example.com", Body: "Can we meet Tuesday?"}},
		Identity:    Identity{Name: "Me", Email: "me@example.com"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if path != "/v1/chat/completions" || auth != "Bearer secret" {
		t.Fatalf("path = %q auth = %q", path, auth)
	}
	if payload["model"] != "test-model" || payload["temperature"] != 0.3 {
		t.Fatalf("payload = %+v", payload)
	}
	if draft.Language != "fr" || draft.Confidence != 0.82 || draft.NeedsHuman {
		t.Fatalf("draft = %+v", draft)
	}
	if completion.Usage.PromptTokens != 120 || completion.Model != "test-model" {
		t.Fatalf("completion = %+v", completion)
	}
}

func TestDraftAcceptsAFencedJSONObject(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		content := "```json\n" + validDraft + "\n```"
		encoded, _ := json.Marshal(map[string]any{
			"model":   "test-model",
			"choices": []map[string]any{{"message": map[string]string{"content": content}}},
		})
		_, _ = w.Write(encoded)
	}))
	defer server.Close()
	client, err := NewClient(Config{BaseURL: server.URL, Model: "test-model"})
	if err != nil {
		t.Fatal(err)
	}
	draft, _, err := client.Draft(context.Background(), DraftInput{Instruction: "Reply", Thread: []ThreadMessage{{Body: "Hello"}}})
	if err != nil || !strings.Contains(draft.ReplyText, "mardi") {
		t.Fatalf("draft = %+v, %v", draft, err)
	}
}

func TestDraftRejectsInvalidModelOutput(t *testing.T) {
	cases := []string{
		`{"reply_text":"","language":"fr","confidence":0.8,"needs_human":false}`,
		`{"reply_text":"ok","confidence":0.8,"needs_human":false}`,
		`{"reply_text":"ok","language":"fr","confidence":1.5,"needs_human":false}`,
		`{"reply_text":"ok","language":"fr","confidence":0.8}`,
		`{"reply_text":"ok","language":"fr","confidence":0.8,"needs_human":false,"extra":true}`,
		`not json`,
	}
	for _, content := range cases {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			encoded, _ := json.Marshal(map[string]any{
				"choices": []map[string]any{{"message": map[string]string{"content": content}}},
			})
			_, _ = w.Write(encoded)
		}))
		client, err := NewClient(Config{BaseURL: server.URL, Model: "test-model"})
		if err != nil {
			server.Close()
			t.Fatal(err)
		}
		_, _, err = client.Draft(context.Background(), DraftInput{Instruction: "Reply", Thread: []ThreadMessage{{Body: "Hello"}}})
		var validation *ValidationError
		if !errors.As(err, &validation) {
			t.Errorf("content %q error = %v, want validation error", content, err)
		}
		server.Close()
	}
}

func TestDraftClassifiesRetryableAPIErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = io.WriteString(w, `{"error":{"message":"slow down"}}`)
	}))
	defer server.Close()
	client, err := NewClient(Config{BaseURL: server.URL, Model: "test-model"})
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = client.Draft(context.Background(), DraftInput{Instruction: "Reply", Thread: []ThreadMessage{{Body: "Hello"}}})
	var api *APIError
	if !errors.As(err, &api) || !api.Retryable() {
		t.Fatalf("error = %v, want retryable API error", err)
	}
}

func TestDraftPropagatesProviderTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(150 * time.Millisecond)
		_, _ = io.WriteString(w, `{"choices":[{"message":{"content":"`+validDraft+`"}}]}`)
	}))
	defer server.Close()
	client, err := NewClient(Config{BaseURL: server.URL, Model: "test-model", Timeout: 25 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = client.Draft(context.Background(), DraftInput{Instruction: "Reply", Thread: []ThreadMessage{{Body: "Hello"}}})
	if err == nil {
		t.Fatal("provider timeout was accepted")
	}
	var api *APIError
	if errors.As(err, &api) {
		t.Fatalf("timeout classified as provider API error: %v", err)
	}
}

func TestNewClientRejectsUnsafeOrIncompleteConfiguration(t *testing.T) {
	cases := []Config{
		{BaseURL: "http://example.com/v1", Model: "m"},
		{BaseURL: "https://example.com/v1"},
		{BaseURL: "not a url", Model: "m"},
		{BaseURL: "https://example.com/v1", Model: "m", Temperature: 3},
	}
	for _, config := range cases {
		if _, err := NewClient(config); err == nil {
			t.Errorf("config %+v accepted", config)
		}
	}
}
