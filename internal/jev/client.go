package jev

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const maxResponseBytes = 2 << 20

type Config struct {
	Endpoint   string
	APIKey     string
	Model      string
	HTTPClient *http.Client
	Timeout    time.Duration
}

type Client struct {
	endpoint string
	apiKey   string
	model    string
	http     *http.Client
}

func NewClient(config Config) (*Client, error) {
	endpoint := strings.TrimSpace(config.Endpoint)
	if endpoint == "" {
		endpoint = DefaultEndpoint
	}
	parsed, err := url.Parse(endpoint)
	if err != nil || (parsed.Scheme != "https" && parsed.Scheme != "http") || parsed.Host == "" {
		return nil, errors.New("jev endpoint must be an absolute http(s) URL")
	}
	if parsed.Scheme == "http" && !isLoopback(parsed.Hostname()) {
		return nil, errors.New("jev endpoint must use HTTPS outside localhost")
	}
	model := strings.TrimSpace(config.Model)
	if model == "" {
		model = DefaultModel
	}
	httpClient := config.HTTPClient
	if httpClient == nil {
		timeout := config.Timeout
		if timeout <= 0 {
			timeout = 20 * time.Second
		}
		httpClient = &http.Client{Timeout: timeout}
	}
	return &Client{endpoint: endpoint, apiKey: config.APIKey, model: model, http: httpClient}, nil
}

func (c *Client) Analyze(ctx context.Context, state State) (Response, error) {
	return c.Ask(ctx, state, DefaultQuestions())
}

func (c *Client) Ask(ctx context.Context, state State, questions map[string]Question) (Response, error) {
	if c == nil {
		return Response{}, errors.New("jev client is nil")
	}
	if len(questions) == 0 {
		return Response{}, errors.New("jev questions are required")
	}
	body, err := json.Marshal(Request{Model: c.model, State: state, Questions: questions})
	if err != nil {
		return Response{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(body))
	if err != nil {
		return Response{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return Response{}, fmt.Errorf("call jev: %w", err)
	}
	defer resp.Body.Close()
	payload, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
	if err != nil {
		return Response{}, fmt.Errorf("read jev response: %w", err)
	}
	if len(payload) > maxResponseBytes {
		return Response{}, errors.New("jev response is too large")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return Response{}, &APIError{StatusCode: resp.StatusCode, Body: truncate(string(payload), 300)}
	}
	return parseResponse(payload)
}

func isLoopback(host string) bool {
	switch strings.ToLower(host) {
	case "localhost", "127.0.0.1", "::1":
		return true
	default:
		return false
	}
}

func truncate(value string, limit int) string {
	value = strings.TrimSpace(strings.ReplaceAll(value, "\n", " "))
	if len(value) > limit {
		return value[:limit]
	}
	return value
}
