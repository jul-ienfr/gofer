package routing

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/cristianadrielbraun/gofer/internal/netguard"
)

const (
	WebhookTimeout      = 10 * time.Second
	WebhookMaxResponse  = 64 << 10
	WebhookTimestampTol = 5 * time.Minute
)

type WebhookRequest struct {
	URL            string
	Secret         string
	Payload        []byte
	IdempotencyKey string
	AllowPrivate   bool
	Now            time.Time
}

type WebhookResult struct {
	StatusCode int
	Body       string
	Duration   time.Duration
}

type WebhookError struct {
	StatusCode int
	Retryable  bool
	Dead       bool
	Body       string
}

func (e *WebhookError) Error() string {
	if e.Body == "" {
		return fmt.Sprintf("webhook returned HTTP %d", e.StatusCode)
	}
	return fmt.Sprintf("webhook returned HTTP %d: %s", e.StatusCode, e.Body)
}

func SendWebhook(ctx context.Context, request WebhookRequest) (WebhookResult, error) {
	target, err := url.Parse(strings.TrimSpace(request.URL))
	if err != nil || target.Host == "" {
		return WebhookResult{}, errors.New("webhook URL must be absolute")
	}
	if target.Scheme != "https" && !(request.AllowPrivate && target.Scheme == "http") {
		return WebhookResult{}, errors.New("webhook URL must use HTTPS unless private targets are explicitly allowed")
	}
	if request.Secret == "" {
		return WebhookResult{}, errors.New("webhook secret is required")
	}
	if len(request.Payload) == 0 {
		return WebhookResult{}, errors.New("webhook payload is empty")
	}
	if len(request.Payload) > 1<<20 {
		return WebhookResult{}, errors.New("webhook payload is too large")
	}
	now := request.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}
	timestamp := now.UTC().Format(time.RFC3339)
	mac := hmac.New(sha256.New, []byte(request.Secret))
	mac.Write([]byte(timestamp))
	mac.Write([]byte("."))
	mac.Write(request.Payload)
	signature := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	client := &http.Client{
		Timeout: WebhookTimeout,
		Transport: netguard.NewHTTPTransport(func(_ context.Context, protocol, host string, port int) bool {
			return request.AllowPrivate
		}),
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, target.String(), bytes.NewReader(request.Payload))
	if err != nil {
		return WebhookResult{}, err
	}
	httpRequest.Header.Set("Content-Type", "application/json")
	httpRequest.Header.Set("Accept", "application/json")
	httpRequest.Header.Set("User-Agent", "Gofer")
	httpRequest.Header.Set("X-Gofer-Signature", signature)
	httpRequest.Header.Set("X-Gofer-Timestamp", timestamp)
	key := strings.TrimSpace(request.IdempotencyKey)
	if key == "" {
		key = idempotencyFromPayload(request.Payload)
	}
	httpRequest.Header.Set("Idempotency-Key", key)
	started := time.Now()
	response, err := client.Do(httpRequest)
	if err != nil {
		return WebhookResult{}, fmt.Errorf("send webhook: %w", err)
	}
	defer response.Body.Close()
	body, readErr := io.ReadAll(io.LimitReader(response.Body, WebhookMaxResponse+1))
	if readErr != nil {
		return WebhookResult{}, fmt.Errorf("read webhook response: %w", readErr)
	}
	if len(body) > WebhookMaxResponse {
		return WebhookResult{}, errors.New("webhook response is too large")
	}
	result := WebhookResult{StatusCode: response.StatusCode, Body: truncateText(string(body), 300), Duration: time.Since(started)}
	if response.StatusCode >= 200 && response.StatusCode < 300 {
		return result, nil
	}
	webhookErr := &WebhookError{StatusCode: response.StatusCode, Body: result.Body}
	switch {
	case response.StatusCode == http.StatusGone:
		webhookErr.Dead = true
	case response.StatusCode == http.StatusRequestTimeout || response.StatusCode == http.StatusTooManyRequests || response.StatusCode >= 500:
		webhookErr.Retryable = true
	default:
		webhookErr.Dead = true
	}
	return result, webhookErr
}

func VerifyWebhookSignature(secret, timestamp string, payload []byte, signature string) bool {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(timestamp))
	mac.Write([]byte("."))
	mac.Write(payload)
	expected := "sha256=" + hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(expected), []byte(signature))
}

func idempotencyFromPayload(payload []byte) string {
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:16])
}

func truncateText(value string, limit int) string {
	value = strings.TrimSpace(value)
	runes := []rune(value)
	if len(runes) > limit {
		return string(runes[:limit])
	}
	return value
}
