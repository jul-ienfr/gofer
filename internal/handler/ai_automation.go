package handler

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/cristianadrielbraun/gofer/internal/aiaudit"
	"github.com/cristianadrielbraun/gofer/internal/storage"
)

func (h *Handler) handleAIAutomationUndo(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, serverText(r.Context(), "method not allowed"), http.StatusMethodNotAllowed)
		return
	}
	userID := h.userID(r.Context())
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		writeComposeJSONError(w, r, http.StatusBadRequest, "missing automation id")
		return
	}
	var action storage.AIAutomationAction
	if err := h.withUserDB(r.Context(), userID, func(db *storage.DB) error {
		var err error
		action, err = db.GetAIAutomationAction(r.Context(), userID, id)
		return err
	}); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeComposeJSONError(w, r, http.StatusNotFound, "automation not found")
			return
		}
		writeComposeJSONError(w, r, http.StatusInternalServerError, "failed to load automation")
		return
	}
	if (action.Kind != "move_junk" && action.Kind != "route_move") || action.State != "executed" {
		writeComposeJSONError(w, r, http.StatusConflict, "automation cannot be undone")
		return
	}
	if action.UndoDeadline.IsZero() || time.Now().UTC().After(action.UndoDeadline) {
		writeComposeJSONError(w, r, http.StatusConflict, "undo window expired")
		return
	}
	var folders struct {
		Source      string `json:"source"`
		Destination string `json:"destination"`
	}
	if err := json.Unmarshal([]byte(action.GateJSON), &folders); err != nil || folders.Source == "" || folders.Destination == "" {
		writeComposeJSONError(w, r, http.StatusConflict, "automation has no undo target")
		return
	}
	if err := h.withUserDB(r.Context(), userID, func(db *storage.DB) error {
		if err := db.MoveMessageAndQueueForUser(r.Context(), action.MessageID, folders.Destination, folders.Source, userID); err != nil {
			return err
		}
		if err := db.MarkAIAutomationUndone(r.Context(), action.ID, `{"restored":true}`); err != nil {
			return err
		}
		_, auditErr := aiaudit.Record(r.Context(), db, aiaudit.Event{
			UserID: userID, ActorType: aiaudit.ActorUser, ActorID: userID,
			Action: "ai.automation.undo", SubjectType: "message",
			SubjectID: fmt.Sprintf("%d", action.MessageID),
			AfterJSON: `{"restored":true}`,
		})
		if auditErr != nil {
			return auditErr
		}
		return nil
	}); err != nil {
		writeComposeJSONError(w, r, http.StatusConflict, "could not undo automation")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
