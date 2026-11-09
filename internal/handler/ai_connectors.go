package handler

import (
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"net/url"
	"strings"

	"github.com/google/uuid"

	"github.com/cristianadrielbraun/gofer/internal/aiaudit"
	"github.com/cristianadrielbraun/gofer/internal/storage"
)

type aiConnectorPayload struct {
	Name         string `json:"name"`
	Type         string `json:"type"`
	URL          string `json:"url"`
	Secret       string `json:"secret"`
	Enabled      bool   `json:"enabled"`
	AllowPrivate bool   `json:"allow_private"`
}

type aiConnectorView struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Type         string `json:"type"`
	URL          string `json:"url"`
	Enabled      bool   `json:"enabled"`
	AllowPrivate bool   `json:"allow_private"`
	HasSecret    bool   `json:"has_secret"`
}

func (h *Handler) handleAIConnectors(w http.ResponseWriter, r *http.Request) {
	userID := h.userID(r.Context())
	switch r.Method {
	case http.MethodGet:
		var list []storage.AIConnector
		if err := h.withUserDB(r.Context(), userID, func(db *storage.DB) error {
			var err error
			list, err = db.ListAIConnectors(r.Context(), userID)
			return err
		}); err != nil {
			writeComposeJSONError(w, r, http.StatusInternalServerError, "failed to list connectors")
			return
		}
		views := make([]aiConnectorView, 0, len(list))
		for _, connector := range list {
			views = append(views, aiConnectorView{
				ID: connector.ID, Name: connector.Name, Type: connector.Type, URL: connector.URL,
				Enabled: connector.Enabled, AllowPrivate: connector.AllowPrivate,
				HasSecret: connector.SecretEncrypted != "",
			})
		}
		writeAIJSON(w, http.StatusOK, map[string]any{"connectors": views})
	case http.MethodPost:
		var payload aiConnectorPayload
		if err := decodeAIBody(w, r, &payload); err != nil {
			writeComposeJSONError(w, r, http.StatusBadRequest, "invalid connector payload")
			return
		}
		if err := validateAIConnectorURL(payload.URL, payload.AllowPrivate); err != nil {
			writeComposeJSONError(w, r, http.StatusBadRequest, err.Error())
			return
		}
		if payload.Enabled && strings.TrimSpace(payload.Secret) == "" {
			writeComposeJSONError(w, r, http.StatusBadRequest, "an enabled connector requires a signing secret")
			return
		}
		if h.accountStore == nil && strings.TrimSpace(payload.Secret) != "" {
			writeComposeJSONError(w, r, http.StatusServiceUnavailable, "connector secret storage is unavailable")
			return
		}
		connector := storage.AIConnector{
			ID: newAIConnectorID(), UserID: userID, Name: payload.Name, Type: payload.Type, URL: payload.URL,
			Enabled: payload.Enabled, AllowPrivate: payload.AllowPrivate,
		}
		var encrypted string
		if h.accountStore != nil {
			var err error
			encrypted, err = h.accountStore.EncryptAIConnectorSecret(userID, connector.ID, payload.Secret)
			if err != nil {
				writeComposeJSONError(w, r, http.StatusInternalServerError, "failed to protect connector secret")
				return
			}
		}
		connector.SecretEncrypted = encrypted
		var created storage.AIConnector
		if err := h.withUserDB(r.Context(), userID, func(db *storage.DB) error {
			var err error
			created, err = db.CreateAIConnector(r.Context(), connector)
			if err == nil {
				h.auditAIConnector(r, db, "ai.connector.create", created)
			}
			return err
		}); err != nil {
			writeComposeJSONError(w, r, http.StatusBadRequest, "failed to create connector")
			return
		}
		writeAIJSON(w, http.StatusCreated, aiConnectorView{
			ID: created.ID, Name: created.Name, Type: created.Type, URL: created.URL,
			Enabled: created.Enabled, AllowPrivate: created.AllowPrivate, HasSecret: created.SecretEncrypted != "",
		})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (h *Handler) handleAIRoutingStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	userID := h.userID(r.Context())
	var items []storage.AIRoutingStatus
	var unsubscribes []storage.AIUnsubscribeStatus
	var actions []storage.AIAutomationAction
	if err := h.withUserDB(r.Context(), userID, func(db *storage.DB) error {
		var err error
		items, err = db.ListAIRoutingStatus(r.Context(), userID, 20)
		if err != nil {
			return err
		}
		unsubscribes, err = db.ListAIUnsubscribeStatus(r.Context(), userID, 20)
		if err != nil {
			return err
		}
		actions, err = db.ListAIAutomationActions(r.Context(), userID, 20)
		return err
	}); err != nil {
		writeComposeJSONError(w, r, http.StatusInternalServerError, "could not list automation activity")
		return
	}
	if items == nil {
		items = []storage.AIRoutingStatus{}
	}
	if unsubscribes == nil {
		unsubscribes = []storage.AIUnsubscribeStatus{}
	}
	if actions == nil {
		actions = []storage.AIAutomationAction{}
	}
	writeAIJSON(w, http.StatusOK, map[string]any{
		"deliveries": items, "unsubscribes": unsubscribes, "actions": actions,
	})
}

func (h *Handler) handleAIConnector(w http.ResponseWriter, r *http.Request) {
	userID := h.userID(r.Context())
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		writeComposeJSONError(w, r, http.StatusBadRequest, "missing connector id")
		return
	}
	switch r.Method {
	case http.MethodDelete:
		if err := h.withUserDB(r.Context(), userID, func(db *storage.DB) error {
			connector, err := db.GetAIConnector(r.Context(), userID, id)
			if err != nil {
				return err
			}
			if err := db.DeleteAIConnector(r.Context(), userID, id); err != nil {
				return err
			}
			h.auditAIConnector(r, db, "ai.connector.delete", connector)
			return nil
		}); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				writeComposeJSONError(w, r, http.StatusNotFound, "connector not found")
				return
			}
			writeComposeJSONError(w, r, http.StatusInternalServerError, "failed to delete connector")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	case http.MethodPatch:
		var payload struct {
			Enabled *bool `json:"enabled"`
		}
		if err := decodeAIBody(w, r, &payload); err != nil || payload.Enabled == nil {
			writeComposeJSONError(w, r, http.StatusBadRequest, "invalid connector update")
			return
		}
		if err := h.withUserDB(r.Context(), userID, func(db *storage.DB) error {
			connector, err := db.GetAIConnector(r.Context(), userID, id)
			if err != nil {
				return err
			}
			if *payload.Enabled && connector.SecretEncrypted == "" {
				return errors.New("connector has no signing secret")
			}
			if err := db.SetAIConnectorEnabled(r.Context(), userID, id, *payload.Enabled); err != nil {
				return err
			}
			connector.Enabled = *payload.Enabled
			h.auditAIConnector(r, db, "ai.connector.update", connector)
			return nil
		}); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				writeComposeJSONError(w, r, http.StatusNotFound, "connector not found")
				return
			}
			writeComposeJSONError(w, r, http.StatusBadRequest, "failed to update connector")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func validateAIConnectorURL(raw string, allowPrivate bool) error {
	target, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || target.Host == "" || target.User != nil {
		return errors.New("connector URL must be an absolute URL without credentials")
	}
	if target.Scheme != "https" && !(allowPrivate && target.Scheme == "http") {
		return errors.New("connector URL must use HTTPS unless private targets are allowed")
	}
	return nil
}

func (h *Handler) auditAIConnector(r *http.Request, db *storage.DB, action string, connector storage.AIConnector) {
	after, _ := json.Marshal(map[string]any{
		"name": connector.Name, "type": connector.Type, "enabled": connector.Enabled,
	})
	if _, err := aiaudit.Record(r.Context(), db, aiaudit.Event{
		UserID: h.userID(r.Context()), ActorType: aiaudit.ActorUser, ActorID: h.userID(r.Context()),
		Action: action, SubjectType: "connector", SubjectID: connector.ID, AfterJSON: string(after),
	}); err != nil {
		log.Printf("ai: connector audit: %v", err)
	}
}

func newAIConnectorID() string {
	return uuid.NewString()
}
