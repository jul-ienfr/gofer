package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/cristianadrielbraun/gofer/internal/aiaudit"
	"github.com/cristianadrielbraun/gofer/internal/aievents"
	"github.com/cristianadrielbraun/gofer/internal/aisettings"
	"github.com/cristianadrielbraun/gofer/internal/storage"
	"github.com/cristianadrielbraun/gofer/internal/views"
)

func (h *Handler) SetAIEvents(events *aievents.Bus) {
	h.aiEvents = events
}

func (h *Handler) SetAIWorkerReady(ready bool) {
	h.aiWorkerReady = ready
}

func (h *Handler) aiSettingsData(ctx context.Context, userID string) views.AISettingsData {
	data := views.AISettingsData{WorkerReady: h.aiWorkerReady, DraftingReady: h.aiLLMClient != nil}
	_ = h.withUserDB(ctx, userID, func(db *storage.DB) error {
		settings, err := aisettings.Load(ctx, db, userID)
		if err != nil {
			return err
		}
		data.AnalysisEnabled = settings.AnalysisEnabled
		data.DraftingEnabled = settings.DraftingEnabled
		data.AutoSpamEnabled = settings.AutoSpamEnabled
		data.AutoUnsubscribeEnabled = settings.AutoUnsubscribeEnabled
		return nil
	})
	return data
}

type aiSettingsPayload struct {
	AnalysisEnabled        bool `json:"analysis_enabled"`
	DraftingEnabled        bool `json:"drafting_enabled"`
	AutoSpamEnabled        bool `json:"auto_spam_enabled"`
	AutoUnsubscribeEnabled bool `json:"auto_unsubscribe_enabled"`
}

func (p aiSettingsPayload) settings() aisettings.Settings {
	return aisettings.Settings{
		AnalysisEnabled:        p.AnalysisEnabled,
		DraftingEnabled:        p.DraftingEnabled,
		AutoSpamEnabled:        p.AutoSpamEnabled,
		AutoUnsubscribeEnabled: p.AutoUnsubscribeEnabled,
	}
}

func aiSettingsResponse(settings aisettings.Settings) aiSettingsPayload {
	return aiSettingsPayload{
		AnalysisEnabled:        settings.AnalysisEnabled,
		DraftingEnabled:        settings.DraftingEnabled,
		AutoSpamEnabled:        settings.AutoSpamEnabled,
		AutoUnsubscribeEnabled: settings.AutoUnsubscribeEnabled,
	}
}

func (h *Handler) handleAISettingsGet(w http.ResponseWriter, r *http.Request) {
	userID := h.userID(r.Context())
	var settings aisettings.Settings
	if err := h.withUserDB(r.Context(), userID, func(db *storage.DB) error {
		var err error
		settings, err = aisettings.Load(r.Context(), db, userID)
		return err
	}); err != nil {
		http.Error(w, serverText(r.Context(), "failed to load AI settings"), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(aiSettingsResponse(settings))
}

func (h *Handler) handleAISettingsUpdate(w http.ResponseWriter, r *http.Request) {
	userID := h.userID(r.Context())
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10))
	decoder.DisallowUnknownFields()
	var payload aiSettingsPayload
	if err := decoder.Decode(&payload); err != nil {
		http.Error(w, serverText(r.Context(), "invalid AI settings payload"), http.StatusBadRequest)
		return
	}
	desired := payload.settings()
	if desired.AnalysisEnabled && !h.aiWorkerReady {
		http.Error(w, serverText(r.Context(), "AI analysis is unavailable: the Jev worker is not configured"), http.StatusConflict)
		return
	}
	if desired.DraftingEnabled && h.aiLLMClient == nil {
		http.Error(w, serverText(r.Context(), "AI drafting is unavailable: no LLM endpoint is configured"), http.StatusConflict)
		return
	}
	var before aisettings.Settings
	if err := h.withUserDB(r.Context(), userID, func(db *storage.DB) error {
		var err error
		before, err = aisettings.Load(r.Context(), db, userID)
		if err != nil {
			return err
		}
		if err := aisettings.Save(r.Context(), db, userID, desired); err != nil {
			return err
		}
		if desired.AnalysisEnabled {
			if err := db.EnsureDefaultAIProjects(r.Context(), userID); err != nil {
				log.Printf("ai: seed default projects: %v", err)
			}
		}
		beforeJSON, err := json.Marshal(aiSettingsResponse(before))
		if err != nil {
			return err
		}
		afterJSON, err := json.Marshal(aiSettingsResponse(desired))
		if err != nil {
			return err
		}
		if _, err := aiaudit.Record(r.Context(), db, aiaudit.Event{
			UserID:      userID,
			ActorType:   aiaudit.ActorUser,
			ActorID:     userID,
			Action:      "ai.settings.update",
			SubjectType: "user",
			SubjectID:   userID,
			BeforeJSON:  string(beforeJSON),
			AfterJSON:   string(afterJSON),
			IP:          r.RemoteAddr,
			UserAgent:   r.UserAgent(),
		}); err != nil {
			log.Printf("ai: record settings audit event: %v", err)
		}
		if h.aiEvents != nil {
			h.aiEvents.Publish(aievents.Event{Type: "ai.settings", UserID: userID, State: "updated"})
		}
		return nil
	}); err != nil {
		http.Error(w, serverText(r.Context(), "failed to save AI settings"), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(aiSettingsResponse(desired))
}

func (h *Handler) handleAIAuditList(w http.ResponseWriter, r *http.Request) {
	userID := h.userID(r.Context())
	limit := 100
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed <= 0 {
			http.Error(w, serverText(r.Context(), "invalid limit"), http.StatusBadRequest)
			return
		}
		limit = parsed
	}
	var events []aiaudit.Event
	if err := h.withUserDB(r.Context(), userID, func(db *storage.DB) error {
		var err error
		events, err = aiaudit.List(r.Context(), db, userID, limit)
		return err
	}); err != nil {
		http.Error(w, serverText(r.Context(), "failed to load AI audit events"), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"events": events})
}

func (h *Handler) handleAIEvents(w http.ResponseWriter, r *http.Request) {
	if h.aiEvents == nil {
		http.Error(w, serverText(r.Context(), "AI events are unavailable"), http.StatusServiceUnavailable)
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, serverText(r.Context(), "streaming is unsupported"), http.StatusInternalServerError)
		return
	}
	userID := h.userID(r.Context())
	events, cancel := h.aiEvents.Subscribe(userID)
	defer cancel()
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	if _, err := fmt.Fprint(w, "event: ready\ndata: {}\n\n"); err != nil {
		return
	}
	flusher.Flush()
	heartbeat := time.NewTicker(25 * time.Second)
	defer heartbeat.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case event := <-events:
			data, err := json.Marshal(event)
			if err != nil {
				continue
			}
			if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event.Type, data); err != nil {
				return
			}
			flusher.Flush()
		case <-heartbeat.C:
			if _, err := fmt.Fprint(w, ": keepalive\n\n"); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}
