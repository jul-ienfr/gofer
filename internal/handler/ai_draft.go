package handler

import (
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"strings"

	"github.com/cristianadrielbraun/gofer/internal/aiaudit"
	"github.com/cristianadrielbraun/gofer/internal/aisettings"
	"github.com/cristianadrielbraun/gofer/internal/drafting"
	"github.com/cristianadrielbraun/gofer/internal/llm"
	"github.com/cristianadrielbraun/gofer/internal/storage"
)

const maxDraftRequestBytes = 32 << 10

func (h *Handler) SetAIDraftingClient(client *llm.Client) {
	h.aiLLMClient = client
}

func (h *Handler) handleAIDraft(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if h.aiLLMClient == nil {
		writeComposeJSONError(w, http.StatusServiceUnavailable, "LLM drafting is not configured")
		return
	}
	messageID, err := h.aiMessageID(r)
	if err != nil {
		writeComposeJSONError(w, http.StatusBadRequest, "invalid message id")
		return
	}
	var payload struct {
		Instruction string `json:"instruction"`
		Tone        string `json:"tone"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, maxDraftRequestBytes)).Decode(&payload); err != nil && !errors.Is(err, io.EOF) {
		writeComposeJSONError(w, http.StatusBadRequest, "invalid draft request")
		return
	}
	ctx := r.Context()
	userID := h.userID(ctx)
	var result drafting.Result
	err = h.withUserDB(ctx, userID, func(db *storage.DB) error {
		var owned int
		if err := db.Read().QueryRowContext(ctx, `
			SELECT 1 FROM messages m JOIN accounts a ON a.id = m.account_id
			WHERE m.id = ? AND a.user_id = ? AND COALESCE(a.is_deleting, 0) = 0`,
			messageID, userID).Scan(&owned); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return drafting.ErrMessageNotFound
			}
			return err
		}
		settings, err := aisettings.Load(ctx, db, userID)
		if err != nil {
			return err
		}
		if !settings.DraftingEnabled {
			return errAIDraftingDisabled
		}
		service := &drafting.Service{DB: db, Generator: h.aiLLMClient}
		result, err = service.Generate(ctx, drafting.Request{
			UserID: userID, MessageID: messageID,
			Instruction: strings.TrimSpace(payload.Instruction), Tone: strings.TrimSpace(payload.Tone),
		})
		if err != nil {
			return err
		}
		after, _ := json.Marshal(map[string]any{"model": result.Model, "confidence": result.Draft.Confidence, "needs_human": result.Draft.NeedsHuman})
		_, _ = aiaudit.Record(ctx, db, aiaudit.Event{
			UserID: userID, ActorType: "user", Action: "ai.draft.generate",
			SubjectType: "message", SubjectID: r.PathValue("id"), AfterJSON: string(after),
		})
		return nil
	})
	if err != nil {
		switch {
		case errors.Is(err, errAIDraftingDisabled):
			writeComposeJSONError(w, http.StatusConflict, "LLM drafting is disabled in settings")
		case errors.Is(err, drafting.ErrMessageNotFound):
			writeComposeJSONError(w, http.StatusNotFound, "message not found")
		case errors.Is(err, drafting.ErrAnalysisMissing):
			writeComposeJSONError(w, http.StatusConflict, "analysis is required before drafting")
		case errors.Is(err, drafting.ErrUnsafeAnalysis), errors.Is(err, drafting.ErrProjectDraftDenied):
			writeComposeJSONError(w, http.StatusConflict, "drafting is disabled for this message")
		default:
			var apiErr *llm.APIError
			if errors.As(err, &apiErr) {
				writeComposeJSONError(w, http.StatusBadGateway, "LLM provider rejected the request")
				return
			}
			log.Printf("ai draft message=%d: %v", messageID, err)
			writeComposeJSONError(w, http.StatusBadGateway, "draft generation failed")
		}
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"reply_text":  result.Draft.ReplyText,
		"language":    result.Draft.Language,
		"confidence":  result.Draft.Confidence,
		"needs_human": result.Draft.NeedsHuman,
		"reason":      result.Draft.Reason,
		"warnings":    result.Warnings,
		"model":       result.Model,
		"usage":       result.Usage,
		"latency_ms":  result.Latency.Milliseconds(),
		"compose":     result.Compose,
	})
}

var errAIDraftingDisabled = errors.New("AI drafting is disabled")

func (h *Handler) aiMessageID(r *http.Request) (int64, error) {
	raw := strings.TrimSpace(r.PathValue("id"))
	if raw == "" {
		return 0, errors.New("missing message id")
	}
	var id int64
	for i := 0; i < len(raw); i++ {
		if raw[i] < '0' || raw[i] > '9' {
			return 0, errors.New("invalid message id")
		}
		id = id*10 + int64(raw[i]-'0')
	}
	if id <= 0 {
		return 0, errors.New("invalid message id")
	}
	return id, nil
}
