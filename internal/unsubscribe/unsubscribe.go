package unsubscribe

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/mail"
	"net/url"
	"strings"
	"time"

	"github.com/cristianadrielbraun/gofer/internal/netguard"
	"github.com/cristianadrielbraun/gofer/internal/storage"
	"github.com/cristianadrielbraun/gofer/internal/store"
)

const (
	MaxRawMessage = 4 << 20
	MaxResponse   = 64 << 10
)

type Result struct {
	Attempted  bool
	StatusCode int
	TargetHash string
	Reason     string
}

// Attempt performs an RFC 8058 one-click unsubscribe for a spam message. It
// never follows redirects, never sends cookies, and refuses non-HTTPS targets.
func Attempt(ctx context.Context, db *storage.DB, userID string, messageID int64) (Result, error) {
	return attempt(ctx, db, userID, messageID, nil)
}

func attempt(ctx context.Context, db *storage.DB, userID string, messageID int64, client *http.Client) (Result, error) {
	if db == nil || messageID <= 0 || strings.TrimSpace(userID) == "" {
		return Result{}, errors.New("unsubscribe requires a message and owner")
	}
	snapshot, err := db.SnapshotRawMessage(ctx, userID, messageID)
	if err != nil {
		return Result{}, err
	}
	raw, err := store.ReadMessageFile(snapshot.Info().RawPath)
	if err != nil {
		return Result{}, err
	}
	if len(raw) > MaxRawMessage {
		return Result{Reason: "message too large"}, nil
	}
	message, err := mail.ReadMessage(strings.NewReader(string(raw)))
	if err != nil {
		return Result{Reason: "invalid message headers"}, nil
	}
	target, headerHash, ok := ParseTarget(message.Header.Get("List-Unsubscribe"), message.Header.Get("List-Unsubscribe-Post"))
	if !ok {
		return Result{Reason: "no one-click target"}, nil
	}
	targetHash := hash(target)
	attemptID, reserved, err := db.ReserveAIUnsubscribeAttempt(ctx, storage.AIUnsubscribeAttempt{
		MessageID: messageID, ListUnsubscribeHash: headerHash, Method: "one-click", TargetHash: targetHash,
	})
	if err != nil {
		return Result{}, err
	}
	if !reserved {
		return Result{Reason: "already attempted", TargetHash: targetHash}, nil
	}
	httpClient := client
	if httpClient == nil {
		httpClient = guardedClient()
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, target,
		strings.NewReader("List-Unsubscribe=One-Click"))
	if err != nil {
		_ = db.FinishAIUnsubscribeAttempt(ctx, attemptID, "failed", 0, 0, err.Error())
		return Result{Attempted: true, TargetHash: targetHash}, err
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("User-Agent", "Gofer one-click unsubscribe")
	request.Header.Set("List-Unsubscribe", "<"+target+">")
	response, err := httpClient.Do(request)
	if err != nil {
		_ = db.FinishAIUnsubscribeAttempt(ctx, attemptID, "failed", 0, 0, err.Error())
		return Result{Attempted: true, TargetHash: targetHash}, fmt.Errorf("send unsubscribe: %w", err)
	}
	defer response.Body.Close()
	body, readErr := io.ReadAll(io.LimitReader(response.Body, MaxResponse+1))
	if readErr != nil {
		_ = db.FinishAIUnsubscribeAttempt(ctx, attemptID, "failed", response.StatusCode, 0, readErr.Error())
		return Result{Attempted: true, StatusCode: response.StatusCode, TargetHash: targetHash}, readErr
	}
	if len(body) > MaxResponse {
		body = body[:MaxResponse]
	}
	status := "sent"
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		status = "failed"
	}
	if err := db.FinishAIUnsubscribeAttempt(ctx, attemptID, status, response.StatusCode, 0, string(body)); err != nil {
		return Result{Attempted: true, StatusCode: response.StatusCode, TargetHash: targetHash}, err
	}
	if status == "failed" {
		return Result{Attempted: true, StatusCode: response.StatusCode, TargetHash: targetHash},
			fmt.Errorf("unsubscribe returned HTTP %d", response.StatusCode)
	}
	return Result{Attempted: true, StatusCode: response.StatusCode, TargetHash: targetHash}, nil
}

func ParseTarget(listUnsubscribe, listUnsubscribePost string) (string, string, bool) {
	if !strings.Contains(strings.ToLower(listUnsubscribePost), "list-unsubscribe=one-click") {
		return "", "", false
	}
	header := strings.TrimSpace(listUnsubscribe)
	if header == "" {
		return "", "", false
	}
	for _, part := range strings.Split(header, ",") {
		part = strings.TrimSpace(part)
		if !strings.HasPrefix(part, "<") || !strings.HasSuffix(part, ">") {
			continue
		}
		candidate := strings.TrimSpace(part[1 : len(part)-1])
		parsed, err := url.Parse(candidate)
		if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil {
			continue
		}
		return parsed.String(), hash(header), true
	}
	return "", "", false
}

func guardedClient() *http.Client {
	return &http.Client{
		Timeout: 10 * time.Second,
		Transport: netguard.NewHTTPTransport(func(_ context.Context, protocol, host string, port int) bool {
			return false
		}),
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

func hash(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}
