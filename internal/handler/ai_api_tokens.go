package handler

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"errors"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/cristianadrielbraun/gofer/internal/aiaudit"
	"github.com/cristianadrielbraun/gofer/internal/auth"
	"github.com/cristianadrielbraun/gofer/internal/storage"
)

const aiAPITokenPrefix = "gfr_"

func (h *Handler) RequireAIToken(scope string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		header := strings.TrimSpace(r.Header.Get("Authorization"))
		if !strings.HasPrefix(header, "Bearer ") {
			w.Header().Set("WWW-Authenticate", `Bearer realm="gofer"`)
			writeComposeJSONError(w, r, http.StatusUnauthorized, "missing bearer token")
			return
		}
		plain := strings.TrimSpace(strings.TrimPrefix(header, "Bearer "))
		if !strings.HasPrefix(plain, aiAPITokenPrefix) || len(plain) > 128 {
			w.Header().Set("WWW-Authenticate", `Bearer realm="gofer"`)
			writeComposeJSONError(w, r, http.StatusUnauthorized, "invalid bearer token")
			return
		}
		token, err := h.db.AuthenticateAIAPIToken(r.Context(), storage.HashAIAPIToken(plain), time.Now())
		if errors.Is(err, sql.ErrNoRows) {
			w.Header().Set("WWW-Authenticate", `Bearer realm="gofer"`)
			writeComposeJSONError(w, r, http.StatusUnauthorized, "invalid bearer token")
			return
		}
		if err != nil {
			writeComposeJSONError(w, r, http.StatusInternalServerError, "token lookup failed")
			return
		}
		if h.auth != nil {
			user, err := h.auth.GetUserByID(r.Context(), token.UserID)
			if err != nil || user == nil || !user.Status.AllowsAuthentication() {
				w.Header().Set("WWW-Authenticate", `Bearer realm="gofer"`)
				writeComposeJSONError(w, r, http.StatusUnauthorized, "invalid bearer token")
				return
			}
		}
		if !aiTokenAllows(token.Scopes, scope) {
			writeComposeJSONError(w, r, http.StatusForbidden, "token scope does not allow this operation")
			return
		}
		if !h.allowAPITokenRequest(token.ID, time.Now()) {
			w.Header().Set("Retry-After", "1")
			writeComposeJSONError(w, r, http.StatusTooManyRequests, "too many requests")
			return
		}
		ctx := context.WithValue(r.Context(), aiTokenContextKey{}, token)
		ctx = auth.ContextWithUser(ctx, &auth.User{ID: token.UserID, Username: token.UserID})
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

type aiAPIRateBucket struct {
	tokens float64
	last   time.Time
}

func (h *Handler) allowAPITokenRequest(tokenID string, now time.Time) bool {
	const burst = 30
	const perSecond = 2
	h.aiAPIRateMu.Lock()
	defer h.aiAPIRateMu.Unlock()
	if h.aiAPIRate == nil {
		h.aiAPIRate = map[string]*aiAPIRateBucket{}
	}
	bucket := h.aiAPIRate[tokenID]
	if bucket == nil {
		bucket = &aiAPIRateBucket{tokens: burst, last: now}
		h.aiAPIRate[tokenID] = bucket
	}
	elapsed := now.Sub(bucket.last).Seconds()
	if elapsed > 0 {
		bucket.tokens += elapsed * perSecond
		if bucket.tokens > burst {
			bucket.tokens = burst
		}
		bucket.last = now
	}
	if len(h.aiAPIRate) > 1024 {
		for id, entry := range h.aiAPIRate {
			if now.Sub(entry.last) > 10*time.Minute {
				delete(h.aiAPIRate, id)
			}
		}
	}
	if bucket.tokens < 1 {
		return false
	}
	bucket.tokens--
	return true
}

type aiTokenContextKey struct{}

func aiTokenFromContext(ctx context.Context) (storage.AIAPIToken, bool) {
	token, ok := ctx.Value(aiTokenContextKey{}).(storage.AIAPIToken)
	return token, ok
}

func aiTokenAllows(scopes, required string) bool {
	for _, scope := range strings.Split(scopes, ",") {
		scope = strings.TrimSpace(scope)
		if scope == "write" || scope == required {
			return true
		}
		if required == "read" && scope == "draft" {
			return true
		}
	}
	return false
}

func normalizeAITokenScopes(raw []string) ([]string, error) {
	seen := map[string]bool{}
	var scopes []string
	for _, scope := range raw {
		scope = strings.ToLower(strings.TrimSpace(scope))
		if scope == "" || seen[scope] {
			continue
		}
		switch scope {
		case "read", "draft", "write":
			seen[scope] = true
			scopes = append(scopes, scope)
		default:
			return nil, errors.New("unknown token scope")
		}
	}
	if len(scopes) == 0 {
		return nil, errors.New("at least one token scope is required")
	}
	sort.Strings(scopes)
	return scopes, nil
}

func newAITokenSecret() (string, string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", "", err
	}
	plain := aiAPITokenPrefix + base64.RawURLEncoding.EncodeToString(raw)
	return plain, storage.HashAIAPIToken(plain), nil
}

func (h *Handler) handleAIAPITokens(w http.ResponseWriter, r *http.Request) {
	userID := h.userID(r.Context())
	switch r.Method {
	case http.MethodGet:
		tokens, err := h.db.ListAIAPITokens(r.Context(), userID)
		if err != nil {
			writeComposeJSONError(w, r, http.StatusInternalServerError, "failed to list tokens")
			return
		}
		writeAIJSON(w, http.StatusOK, map[string]any{"tokens": tokens})
	case http.MethodPost:
		var payload struct {
			Name          string   `json:"name"`
			Scopes        []string `json:"scopes"`
			ExpiresInDays int      `json:"expires_in_days"`
		}
		if err := decodeAIBody(w, r, &payload); err != nil {
			writeComposeJSONError(w, r, http.StatusBadRequest, "invalid token payload")
			return
		}
		payload.Name = strings.TrimSpace(payload.Name)
		if payload.Name == "" || len(payload.Name) > 80 {
			writeComposeJSONError(w, r, http.StatusBadRequest, "token name is required")
			return
		}
		scopes, err := normalizeAITokenScopes(payload.Scopes)
		if err != nil {
			writeComposeJSONError(w, r, http.StatusBadRequest, err.Error())
			return
		}
		count, err := h.db.CountAIAPITokens(r.Context(), userID)
		if err != nil {
			writeComposeJSONError(w, r, http.StatusInternalServerError, "failed to count tokens")
			return
		}
		if count >= storage.AIAPITokenLimit {
			writeComposeJSONError(w, r, http.StatusConflict, "revoke an existing token first")
			return
		}
		days := payload.ExpiresInDays
		if days <= 0 {
			days = 90
		}
		if days > 3650 {
			days = 3650
		}
		plain, hash, err := newAITokenSecret()
		if err != nil {
			writeComposeJSONError(w, r, http.StatusInternalServerError, "failed to generate token")
			return
		}
		token, err := h.db.CreateAIAPIToken(r.Context(), storage.AIAPIToken{
			UserID: userID, Name: payload.Name, TokenHash: hash, Scopes: strings.Join(scopes, ","),
			ExpiresAt: time.Now().UTC().AddDate(0, 0, days),
		})
		if err != nil {
			writeComposeJSONError(w, r, http.StatusInternalServerError, "failed to create token")
			return
		}
		if _, err := aiaudit.Record(r.Context(), h.db, aiaudit.Event{
			UserID: userID, ActorType: "user", ActorID: userID, Action: "ai.api_token.create",
			SubjectType: "api_token", SubjectID: token.ID,
			AfterJSON: `{"name":` + quoteJSON(token.Name) + `,"scopes":` + quoteJSON(token.Scopes) + `}`,
		}); err != nil {
			// The token exists even if its audit row could not be written.
			_ = err
		}
		writeAIJSON(w, http.StatusCreated, map[string]any{
			"id": token.ID, "name": token.Name, "scopes": token.Scopes,
			"expires_at": token.ExpiresAt, "token": plain,
		})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (h *Handler) handleAIAPIToken(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	userID := h.userID(r.Context())
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		writeComposeJSONError(w, r, http.StatusBadRequest, "missing token id")
		return
	}
	revoked, err := h.db.RevokeAIAPIToken(r.Context(), userID, id)
	if err != nil {
		writeComposeJSONError(w, r, http.StatusInternalServerError, "failed to revoke token")
		return
	}
	if !revoked {
		writeComposeJSONError(w, r, http.StatusNotFound, "token not found")
		return
	}
	if _, err := aiaudit.Record(r.Context(), h.db, aiaudit.Event{
		UserID: userID, ActorType: "user", ActorID: userID, Action: "ai.api_token.revoke",
		SubjectType: "api_token", SubjectID: id,
	}); err != nil {
		_ = err
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) handleAPIv1Me(w http.ResponseWriter, r *http.Request) {
	token, _ := aiTokenFromContext(r.Context())
	writeAIJSON(w, http.StatusOK, map[string]any{
		"user_id": token.UserID, "token_id": token.ID, "scopes": token.Scopes,
	})
}

func quoteJSON(value string) string {
	replacer := strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`, "\r", `\r`)
	return `"` + replacer.Replace(value) + `"`
}
