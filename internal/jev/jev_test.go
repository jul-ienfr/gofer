package jev

import (
	"bufio"
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

const validResponse = `{
  "model": "jev-1.14.0",
  "answers": {
    "priority": {"score": 2, "probabilities": [0.02, 0.18, 0.80], "confidence": 0.86},
    "needs_reply": {"noul": 0.94},
    "risk": {"choice": "legit", "probabilities": {"legit": 0.97, "spam": 0.03}, "confidence": 0.95},
    "domain": {"choice": "employment", "probabilities": {"employment": 0.91, "other": 0.09}, "confidence": 0.91}
  },
  "usage": {"input_tokens": 812, "output_tokens": 0}
}`

func TestParseResponseAcceptsTheContract(t *testing.T) {
	result, err := parseResponse([]byte(validResponse))
	if err != nil {
		t.Fatal(err)
	}
	if result.Model != "jev-1.14.0" || result.Answers.Priority.Score != 2 || result.Answers.NeedsReply.Noul != 0.94 {
		t.Fatalf("result = %+v", result)
	}
	if result.Answers.Risk.Choice != "legit" || result.Answers.Domain.Choice != "employment" {
		t.Fatalf("labels = %+v", result.Answers)
	}
	if result.Usage.InputTokens != 812 {
		t.Fatalf("usage = %+v", result.Usage)
	}
}

func TestParseResponseRejectsInvalidAnswers(t *testing.T) {
	cases := []struct {
		name    string
		payload string
	}{
		{"missing", `{"answers":{"priority":{"score":1,"probabilities":[0.1,0.8,0.1],"confidence":0.8},"needs_reply":{"noul":0.5},"risk":{"choice":"legit","confidence":0.8}}}`},
		{"score range", `{"answers":{"priority":{"score":3,"probabilities":[0.1,0.8,0.1],"confidence":0.8},"needs_reply":{"noul":0.5},"risk":{"choice":"legit","confidence":0.8},"domain":{"choice":"other","confidence":0.8}}}`},
		{"probabilities", `{"answers":{"priority":{"score":1,"probabilities":[1.2,0.8,-0.3],"confidence":0.8},"needs_reply":{"noul":0.5},"risk":{"choice":"legit","confidence":0.8},"domain":{"choice":"other","confidence":0.8}}}`},
		{"noul", `{"answers":{"priority":{"score":1,"probabilities":[0.1,0.8,0.1],"confidence":0.8},"needs_reply":{"noul":1.5},"risk":{"choice":"legit","confidence":0.8},"domain":{"choice":"other","confidence":0.8}}}`},
		{"risk label", `{"answers":{"priority":{"score":1,"probabilities":[0.1,0.8,0.1],"confidence":0.8},"needs_reply":{"noul":0.5},"risk":{"choice":"trusted","confidence":0.8},"domain":{"choice":"other","confidence":0.8}}}`},
		{"confidence", `{"answers":{"priority":{"score":1,"probabilities":[0.1,0.8,0.1],"confidence":1.5},"needs_reply":{"noul":0.5},"risk":{"choice":"legit","confidence":0.8},"domain":{"choice":"other","confidence":0.8}}}`},
		{"syntax", `{`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parseResponse([]byte(tc.payload))
			if err == nil {
				t.Fatal("invalid response accepted")
			}
			if tc.name == "syntax" {
				return
			}
			var validation *ValidationError
			if !errors.As(err, &validation) {
				t.Fatalf("error = %v, want validation error", err)
			}
		})
	}
}

func TestClientSendsTheDocumentedRequest(t *testing.T) {
	var received Request
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Errorf("authorization = %q", got)
		}
		payload, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
		}
		if err := json.Unmarshal(payload, &received); err != nil {
			t.Error(err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, validResponse)
	}))
	defer server.Close()
	client, err := NewClient(Config{Endpoint: server.URL, APIKey: "test-key", Timeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	state := BuildState("me@example.com", Message{FromName: "Client", FromEmail: "client@example.com", Subject: "Interview", Body: "Hello"})
	result, err := client.Analyze(context.Background(), state)
	if err != nil {
		t.Fatal(err)
	}
	if result.Answers.Priority.Score != 2 {
		t.Fatalf("result = %+v", result)
	}
	if received.Model != DefaultModel || received.State.Me != "me@example.com" || len(received.Questions) != 4 {
		t.Fatalf("request = %+v", received)
	}
	if _, ok := received.Questions["priority"]; !ok {
		t.Fatal("priority question missing")
	}
}

func TestClientClassifiesRetryableErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = io.WriteString(w, `{"error":"upstream"}`)
	}))
	defer server.Close()
	client, err := NewClient(Config{Endpoint: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Analyze(context.Background(), State{Me: "me@example.com"})
	var api *APIError
	if !errors.As(err, &api) || !api.Retryable() {
		t.Fatalf("error = %v, want retryable API error", err)
	}

	plain := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer plain.Close()
	client, err = NewClient(Config{Endpoint: plain.URL})
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Analyze(context.Background(), State{Me: "me@example.com"})
	if !errors.As(err, &api) || api.Retryable() {
		t.Fatalf("error = %v, want non-retryable API error", err)
	}
}

func TestClientRejectsInsecureRemoteEndpoints(t *testing.T) {
	if _, err := NewClient(Config{Endpoint: "http://example.com/v1/systemone"}); err == nil {
		t.Fatal("plain HTTP remote endpoint accepted")
	}
}

func TestReadRawMessageBuildsAPrivacyBoundedState(t *testing.T) {
	raw := strings.Join([]string{
		"From: Alice Martin <alice@example.com>",
		"To: Me <me@example.com>, Other <other@example.com>",
		"Cc: Copy <copy@example.com>",
		"Subject: Interview",
		"Authentication-Results: mx.example.com; dmarc=pass",
		"List-Unsubscribe: <https://example.com/unsubscribe>",
		"Reply-To: Alice <alice@example.com>",
		"MIME-Version: 1.0",
		`Content-Type: multipart/mixed; boundary="b"`,
		"",
		"--b",
		"Content-Type: text/plain; charset=utf-8",
		"",
		"Bonjour, voici https://jobs.example.com/post et https://jobs.example.com/post.",
		"",
		"--b",
		`Content-Type: application/pdf; name="cv.pdf"`,
		`Content-Disposition: attachment; filename="cv.pdf"`,
		"Content-Transfer-Encoding: base64",
		"",
		"JVBERi0xLjQ=",
		"--b--",
		"",
	}, "\r\n")
	message, err := readRawMessage(bufio.NewReader(strings.NewReader(raw)))
	if err != nil {
		t.Fatal(err)
	}
	if message.FromName != "Alice Martin" || message.FromEmail != "alice@example.com" {
		t.Fatalf("from = %q <%q>", message.FromName, message.FromEmail)
	}
	if len(message.To) != 2 || len(message.CC) != 1 {
		t.Fatalf("recipients = %+v %+v", message.To, message.CC)
	}
	if !strings.Contains(message.Body, "Bonjour") {
		t.Fatalf("body = %q", message.Body)
	}
	if len(message.Attachments) != 1 || !strings.Contains(message.Attachments[0], "cv.pdf") {
		t.Fatalf("attachments = %+v", message.Attachments)
	}
	if message.Headers["Authentication-Results"] == "" || message.Headers["List-Unsubscribe"] == "" {
		t.Fatalf("headers = %+v", message.Headers)
	}
	links := extractLinks(message.Body)
	if len(links) != 1 || links[0] != "jobs.example.com" {
		t.Fatalf("links = %+v", links)
	}
	state := BuildState("me@example.com", message)
	if state.Me != "me@example.com" || state.Email.Subject != "Interview" || len(state.Email.Links) != 1 {
		t.Fatalf("state = %+v", state)
	}
}

func TestBuildStateKeepsEmptyArraysAndBoundsBody(t *testing.T) {
	state := BuildState(" me@example.com ", Message{Body: strings.Repeat("x", maxBodyCharacters+5)})
	if state.Email.To == nil || state.Email.CC == nil || state.Email.Links == nil || state.Email.Attachments == nil {
		t.Fatalf("arrays must be non-nil: %+v", state.Email)
	}
	if len([]rune(state.Email.Body)) != maxBodyCharacters {
		t.Fatalf("body length = %d", len([]rune(state.Email.Body)))
	}
}
