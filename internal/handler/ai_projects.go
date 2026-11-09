package handler

import (
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"

	"github.com/cristianadrielbraun/gofer/internal/aiaudit"
	"github.com/cristianadrielbraun/gofer/internal/projects"
	"github.com/cristianadrielbraun/gofer/internal/storage"
)

type aiProjectPayload struct {
	Name                 string `json:"name"`
	Slug                 string `json:"slug"`
	Description          string `json:"description"`
	Instructions         string `json:"instructions"`
	Color                string `json:"color"`
	Status               string `json:"status"`
	AllowExternalRouting *bool  `json:"allow_external_routing"`
	AllowLLMDraft        *bool  `json:"allow_llm_draft"`
	AllowAutoSpam        *bool  `json:"allow_auto_spam"`
	DataClassification   string `json:"data_classification"`
}

type aiRoutePayload struct {
	Name        string `json:"name"`
	Enabled     bool   `json:"enabled"`
	Priority    int    `json:"priority"`
	MatchJSON   string `json:"match_json"`
	ActionsJSON string `json:"actions_json"`
}

func (h *Handler) handleAIProjects(w http.ResponseWriter, r *http.Request) {
	userID := h.userID(r.Context())
	switch r.Method {
	case http.MethodGet:
		var list []storage.AIProject
		if err := h.withUserDB(r.Context(), userID, func(db *storage.DB) error {
			var err error
			list, err = db.ListAIProjects(r.Context(), userID)
			return err
		}); err != nil {
			writeComposeJSONError(w, r, http.StatusInternalServerError, "failed to list projects")
			return
		}
		writeAIJSON(w, http.StatusOK, map[string]any{"projects": projectsOrEmpty(list)})
	case http.MethodPost:
		var payload aiProjectPayload
		if err := decodeAIBody(w, r, &payload); err != nil {
			writeComposeJSONError(w, r, http.StatusBadRequest, "invalid project payload")
			return
		}
		classification := strings.ToLower(strings.TrimSpace(payload.DataClassification))
		if !validAIClassification(classification) {
			writeComposeJSONError(w, r, http.StatusBadRequest, "invalid data classification")
			return
		}
		if !classificationAllowsExternal(classification) && boolValue(payload.AllowExternalRouting) {
			writeComposeJSONError(w, r, http.StatusBadRequest, "this data classification cannot route externally")
			return
		}
		var created storage.AIProject
		if err := h.withUserDB(r.Context(), userID, func(db *storage.DB) error {
			var err error
			created, err = db.CreateAIProject(r.Context(), storage.AIProject{
				UserID: userID, Name: payload.Name, Slug: payload.Slug, Description: payload.Description,
				Instructions: payload.Instructions, Color: payload.Color, Status: payload.Status,
				AllowExternalRouting: boolValue(payload.AllowExternalRouting) && classificationAllowsExternal(classification),
				AllowLLMDraft:        payload.AllowLLMDraft,
				AllowAutoSpam:        boolValue(payload.AllowAutoSpam), DataClassification: classification,
			})
			return err
		}); err != nil {
			writeComposeJSONError(w, r, http.StatusBadRequest, "failed to create project")
			return
		}
		writeAIJSON(w, http.StatusCreated, created)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (h *Handler) handleAIProjectDefaults(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	userID := h.userID(r.Context())
	var list []storage.AIProject
	if err := h.withUserDB(r.Context(), userID, func(db *storage.DB) error {
		if err := db.EnsureDefaultAIProjects(r.Context(), userID); err != nil {
			return err
		}
		var err error
		list, err = db.ListAIProjects(r.Context(), userID)
		return err
	}); err != nil {
		writeComposeJSONError(w, r, http.StatusInternalServerError, "failed to create default projects")
		return
	}
	writeAIJSON(w, http.StatusOK, map[string]any{"projects": projectsOrEmpty(list)})
}

func (h *Handler) handleAIProject(w http.ResponseWriter, r *http.Request) {
	userID := h.userID(r.Context())
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		writeComposeJSONError(w, r, http.StatusBadRequest, "missing project id")
		return
	}
	switch r.Method {
	case http.MethodGet:
		var project storage.AIProject
		if err := h.withUserDB(r.Context(), userID, func(db *storage.DB) error {
			var err error
			project, err = db.GetAIProject(r.Context(), userID, id)
			return err
		}); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				writeComposeJSONError(w, r, http.StatusNotFound, "project not found")
				return
			}
			writeComposeJSONError(w, r, http.StatusInternalServerError, "failed to read project")
			return
		}
		writeAIJSON(w, http.StatusOK, project)
	case http.MethodPut, http.MethodPatch:
		var payload aiProjectPayload
		if err := decodeAIBody(w, r, &payload); err != nil {
			writeComposeJSONError(w, r, http.StatusBadRequest, "invalid project payload")
			return
		}
		classification := strings.ToLower(strings.TrimSpace(payload.DataClassification))
		if !validAIClassification(classification) {
			writeComposeJSONError(w, r, http.StatusBadRequest, "invalid data classification")
			return
		}
		if !classificationAllowsExternal(classification) && payload.AllowExternalRouting != nil && *payload.AllowExternalRouting {
			writeComposeJSONError(w, r, http.StatusBadRequest, "this data classification cannot route externally")
			return
		}
		var updated storage.AIProject
		if err := h.withUserDB(r.Context(), userID, func(db *storage.DB) error {
			current, err := db.GetAIProject(r.Context(), userID, id)
			if err != nil {
				return err
			}
			project := current
			if payload.Name != "" {
				project.Name = payload.Name
			}
			if payload.Slug != "" {
				project.Slug = payload.Slug
			}
			if payload.Description != "" {
				project.Description = payload.Description
			}
			if payload.Instructions != "" {
				project.Instructions = payload.Instructions
			}
			if payload.Color != "" {
				project.Color = payload.Color
			}
			if payload.Status != "" {
				project.Status = payload.Status
			}
			if classification != "" {
				project.DataClassification = classification
			}
			if payload.AllowExternalRouting != nil {
				project.AllowExternalRouting = *payload.AllowExternalRouting
			}
			if payload.AllowAutoSpam != nil {
				project.AllowAutoSpam = *payload.AllowAutoSpam
			}
			if payload.AllowLLMDraft != nil {
				project.AllowLLMDraft = payload.AllowLLMDraft
			}
			if !classificationAllowsExternal(project.DataClassification) {
				project.AllowExternalRouting = false
			}
			updated, err = db.UpdateAIProject(r.Context(), project)
			return err
		}); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				writeComposeJSONError(w, r, http.StatusNotFound, "project not found")
				return
			}
			writeComposeJSONError(w, r, http.StatusBadRequest, "failed to update project")
			return
		}
		writeAIJSON(w, http.StatusOK, updated)
	case http.MethodDelete:
		if err := h.withUserDB(r.Context(), userID, func(db *storage.DB) error {
			return db.DeleteAIProject(r.Context(), userID, id)
		}); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				writeComposeJSONError(w, r, http.StatusNotFound, "project not found")
				return
			}
			writeComposeJSONError(w, r, http.StatusInternalServerError, "failed to delete project")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (h *Handler) handleAIProjectRoutes(w http.ResponseWriter, r *http.Request) {
	userID := h.userID(r.Context())
	projectID := strings.TrimSpace(r.PathValue("id"))
	if projectID == "" {
		writeComposeJSONError(w, r, http.StatusBadRequest, "missing project id")
		return
	}
	switch r.Method {
	case http.MethodGet:
		var routes []storage.AIProjectRoute
		if err := h.withUserDB(r.Context(), userID, func(db *storage.DB) error {
			var err error
			routes, err = db.ListAIProjectRoutes(r.Context(), userID, projectID)
			return err
		}); err != nil {
			writeComposeJSONError(w, r, http.StatusNotFound, "project not found")
			return
		}
		writeAIJSON(w, http.StatusOK, map[string]any{"routes": routesOrEmpty(routes)})
	case http.MethodPost:
		var payload aiRoutePayload
		if err := decodeAIBody(w, r, &payload); err != nil {
			writeComposeJSONError(w, r, http.StatusBadRequest, "invalid route payload")
			return
		}
		if _, err := projects.ParseMatch(payload.MatchJSON); err != nil {
			writeComposeJSONError(w, r, http.StatusBadRequest, err.Error())
			return
		}
		if _, err := projects.ParseActions(payload.ActionsJSON); err != nil {
			writeComposeJSONError(w, r, http.StatusBadRequest, err.Error())
			return
		}
		var created storage.AIProjectRoute
		if err := h.withUserDB(r.Context(), userID, func(db *storage.DB) error {
			var err error
			created, err = db.CreateAIProjectRoute(r.Context(), userID, storage.AIProjectRoute{
				ProjectID: projectID, Name: payload.Name, Enabled: payload.Enabled, Priority: payload.Priority,
				MatchJSON: payload.MatchJSON, ActionsJSON: payload.ActionsJSON,
			})
			if err == nil {
				after, _ := json.Marshal(map[string]any{
					"name": created.Name, "enabled": created.Enabled, "priority": created.Priority,
					"match_json": created.MatchJSON, "actions_json": created.ActionsJSON,
				})
				_, auditErr := aiaudit.Record(r.Context(), db, aiaudit.Event{
					UserID: userID, ActorType: aiaudit.ActorUser, ActorID: userID,
					Action: "ai.route.create", SubjectType: "route", SubjectID: created.ID, AfterJSON: string(after),
				})
				if auditErr != nil {
					log.Printf("ai: route audit: %v", auditErr)
				}
			}
			return err
		}); err != nil {
			writeComposeJSONError(w, r, http.StatusBadRequest, "failed to create route")
			return
		}
		writeAIJSON(w, http.StatusCreated, created)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (h *Handler) handleAIRoute(w http.ResponseWriter, r *http.Request) {
	userID := h.userID(r.Context())
	id := strings.TrimSpace(r.PathValue("id"))
	switch r.Method {
	case http.MethodDelete:
		if err := h.withUserDB(r.Context(), userID, func(db *storage.DB) error {
			if err := db.DeleteAIProjectRoute(r.Context(), userID, id); err != nil {
				return err
			}
			_, auditErr := aiaudit.Record(r.Context(), db, aiaudit.Event{
				UserID: userID, ActorType: aiaudit.ActorUser, ActorID: userID,
				Action: "ai.route.delete", SubjectType: "route", SubjectID: id,
			})
			if auditErr != nil {
				log.Printf("ai: route audit: %v", auditErr)
			}
			return nil
		}); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				writeComposeJSONError(w, r, http.StatusNotFound, "route not found")
				return
			}
			writeComposeJSONError(w, r, http.StatusInternalServerError, "failed to delete route")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (h *Handler) handleAILegitimacyProfiles(w http.ResponseWriter, r *http.Request) {
	userID := h.userID(r.Context())
	var accountID string
	if err := h.withUserDB(r.Context(), userID, func(db *storage.DB) error {
		return db.Read().QueryRowContext(r.Context(), `
			SELECT id FROM accounts WHERE user_id = ? AND COALESCE(is_deleting, 0) = 0
			ORDER BY id LIMIT 1`, userID).Scan(&accountID)
	}); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeAIJSON(w, http.StatusOK, map[string]any{"profiles": []any{}})
			return
		}
		writeComposeJSONError(w, r, http.StatusInternalServerError, "failed to read account")
		return
	}
	var profiles []storage.AILegitimacyProfile
	if err := h.withUserDB(r.Context(), userID, func(db *storage.DB) error {
		var err error
		profiles, err = db.ListAILegitimacyProfiles(r.Context(), accountID, 200)
		return err
	}); err != nil {
		writeComposeJSONError(w, r, http.StatusInternalServerError, "failed to list legitimacy profiles")
		return
	}
	if profiles == nil {
		profiles = []storage.AILegitimacyProfile{}
	}
	writeAIJSON(w, http.StatusOK, map[string]any{"profiles": profiles})
}

func (h *Handler) handleAILegitimacyProfile(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut && r.Method != http.MethodPatch {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	userID := h.userID(r.Context())
	sender := strings.ToLower(strings.TrimSpace(r.PathValue("email")))
	var payload struct {
		VIP         *bool `json:"vip"`
		Allowlisted *bool `json:"allowlisted"`
		Blocked     *bool `json:"blocked"`
	}
	if err := decodeAIBody(w, r, &payload); err != nil {
		writeComposeJSONError(w, r, http.StatusBadRequest, "invalid legitimacy payload")
		return
	}
	var updated storage.AILegitimacyProfile
	err := h.withUserDB(r.Context(), userID, func(db *storage.DB) error {
		var accountID string
		if err := db.Read().QueryRowContext(r.Context(), `
			SELECT id FROM accounts WHERE user_id = ? AND COALESCE(is_deleting, 0) = 0
			ORDER BY id LIMIT 1`, userID).Scan(&accountID); err != nil {
			return err
		}
		profile, err := db.GetAILegitimacyProfile(r.Context(), accountID, sender)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				profile = storage.AILegitimacyProfile{AccountID: accountID, SenderEmail: sender}
			} else {
				return err
			}
		}
		if payload.VIP != nil {
			profile.VIP = *payload.VIP
		}
		if payload.Allowlisted != nil {
			profile.Allowlisted = *payload.Allowlisted
		}
		if payload.Blocked != nil {
			profile.Blocked = *payload.Blocked
		}
		if profile.Blocked {
			profile.Allowlisted = false
		} else if profile.Allowlisted {
			profile.Blocked = false
		}
		updated, err = db.SaveAILegitimacyProfile(r.Context(), profile)
		if err == nil {
			after, _ := json.Marshal(map[string]any{"vip": updated.VIP, "allowlisted": updated.Allowlisted, "blocked": updated.Blocked})
			_, auditErr := aiaudit.Record(r.Context(), db, aiaudit.Event{
				UserID: userID, ActorType: aiaudit.ActorUser, ActorID: userID,
				Action: "ai.legitimacy.update", SubjectType: "sender", SubjectID: sender, AfterJSON: string(after),
			})
			if auditErr != nil {
				log.Printf("ai: legitimacy audit: %v", auditErr)
			}
		}
		return err
	})
	if err != nil {
		writeComposeJSONError(w, r, http.StatusBadRequest, "failed to update legitimacy profile")
		return
	}
	writeAIJSON(w, http.StatusOK, updated)
}

func writeAIJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func boolValue(value *bool) bool {
	return value != nil && *value
}

func decodeAIBody(w http.ResponseWriter, r *http.Request, target any) error {
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10))
	decoder.DisallowUnknownFields()
	return decoder.Decode(target)
}

func validAIClassification(value string) bool {
	switch strings.TrimSpace(value) {
	case "", "public", "internal", "confidential", "restricted":
		return true
	default:
		return false
	}
}

func classificationAllowsExternal(value string) bool {
	switch strings.TrimSpace(value) {
	case "", "public", "internal":
		return true
	default:
		return false
	}
}

func projectsOrEmpty(list []storage.AIProject) []storage.AIProject {
	if list == nil {
		return []storage.AIProject{}
	}
	return list
}

func routesOrEmpty(list []storage.AIProjectRoute) []storage.AIProjectRoute {
	if list == nil {
		return []storage.AIProjectRoute{}
	}
	return list
}
