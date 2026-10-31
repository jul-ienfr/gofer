package handler

import (
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"

	"github.com/cristianadrielbraun/gofer/internal/aiaudit"
	"github.com/cristianadrielbraun/gofer/internal/aievents"
	"github.com/cristianadrielbraun/gofer/internal/storage"
	"github.com/cristianadrielbraun/gofer/internal/views"
)

type aiAnswerPayload struct {
	QuestionKey   string          `json:"question_key"`
	Type          string          `json:"type"`
	Choice        string          `json:"choice,omitempty"`
	Score         float64         `json:"score,omitempty"`
	Noul          float64         `json:"noul,omitempty"`
	Confidence    float64         `json:"confidence"`
	Probabilities json.RawMessage `json:"probabilities"`
}

type aiAnalysisPayload struct {
	MessageID          int64             `json:"message_id"`
	AnalysisVersion    int               `json:"analysis_version"`
	Model              string            `json:"model"`
	PriorityLabel      string            `json:"priority_label"`
	PriorityConfidence float64           `json:"priority_confidence"`
	RiskLabel          string            `json:"risk_label"`
	RiskConfidence     float64           `json:"risk_confidence"`
	IntentLabel        string            `json:"intent_label"`
	IntentConfidence   float64           `json:"intent_confidence"`
	ProjectLabel       string            `json:"project_label"`
	ProjectConfidence  float64           `json:"project_confidence"`
	Summary            string            `json:"summary"`
	LatencyMS          int64             `json:"latency_ms"`
	PromptVersion      string            `json:"prompt_version"`
	Answers            []aiAnswerPayload `json:"answers"`
}

func aiAnalysisResponse(result *storage.AIAnalysisResult, answers []storage.AIAnalysisAnswer) aiAnalysisPayload {
	payload := aiAnalysisPayload{
		MessageID:          result.MessageID,
		AnalysisVersion:    result.AnalysisVersion,
		Model:              result.Model,
		PriorityLabel:      result.PriorityLabel,
		PriorityConfidence: result.PriorityConfidence,
		RiskLabel:          result.RiskLabel,
		RiskConfidence:     result.RiskConfidence,
		IntentLabel:        result.IntentLabel,
		IntentConfidence:   result.IntentConfidence,
		ProjectLabel:       result.ProjectLabel,
		ProjectConfidence:  result.ProjectConfidence,
		Summary:            result.Summary,
		LatencyMS:          result.LatencyMS,
		PromptVersion:      result.PromptVersion,
		Answers:            make([]aiAnswerPayload, 0, len(answers)),
	}
	for _, answer := range answers {
		probabilities := json.RawMessage(answer.ProbabilitiesJSON)
		if !json.Valid(probabilities) {
			probabilities = json.RawMessage(`{}`)
		}
		payload.Answers = append(payload.Answers, aiAnswerPayload{
			QuestionKey:   answer.QuestionKey,
			Type:          answer.Type,
			Choice:        answer.Choice,
			Score:         answer.Score,
			Noul:          answer.Noul,
			Confidence:    answer.Confidence,
			Probabilities: probabilities,
		})
	}
	return payload
}

func (h *Handler) handleAIMessageAnalysis(w http.ResponseWriter, r *http.Request) {
	messageID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || messageID <= 0 {
		http.Error(w, "invalid message id", http.StatusBadRequest)
		return
	}
	userID := h.userID(r.Context())
	var result *storage.AIAnalysisResult
	var answers []storage.AIAnalysisAnswer
	if err := h.withUserDB(r.Context(), userID, func(db *storage.DB) error {
		var err error
		result, answers, err = db.GetAIAnalysisForUser(r.Context(), messageID, userID)
		return err
	}); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "analysis not found", http.StatusNotFound)
			return
		}
		http.Error(w, "failed to load analysis", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(aiAnalysisResponse(result, answers))
}

func (h *Handler) handleAIAnalysisPanel(w http.ResponseWriter, r *http.Request) {
	messageID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || messageID <= 0 {
		http.Error(w, "invalid message id", http.StatusBadRequest)
		return
	}
	userID := h.userID(r.Context())
	var result *storage.AIAnalysisResult
	var answers []storage.AIAnalysisAnswer
	var profile storage.AILegitimacyProfile
	var senderEmail string
	var automation storage.AIAutomationAction
	if err := h.withUserDB(r.Context(), userID, func(db *storage.DB) error {
		var err error
		result, answers, err = db.GetAIAnalysisForUser(r.Context(), messageID, userID)
		if err != nil {
			return err
		}
		profile, senderEmail, err = db.GetAILegitimacyProfileForMessage(r.Context(), userID, messageID)
		if errors.Is(err, sql.ErrNoRows) {
			err = nil
		}
		if err != nil {
			return err
		}
		automation, err = db.LatestAIAutomationForMessage(r.Context(), userID, messageID)
		if errors.Is(err, sql.ErrNoRows) {
			err = nil
		}
		return err
	}); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		http.Error(w, "failed to load analysis", http.StatusInternalServerError)
		return
	}
	legitimacyState := profile.State
	if legitimacyState == "" {
		legitimacyState = "unknown"
	}
	data := views.AIAnalysisPanelData{
		LegitimacyState:    legitimacyState,
		LegitimacyScore:    profile.Score,
		SenderEmail:        senderEmail,
		SenderVIP:          profile.VIP,
		SenderAllowlisted:  profile.Allowlisted,
		SenderBlocked:      profile.Blocked,
		AutomationKind:     automation.Kind,
		AutomationState:    automation.State,
		AutomationActionID: automation.ID,
		AutomationUndoBy:   automation.UndoDeadline,
		MessageID:          result.MessageID,
		Model:              result.Model,
		PriorityLabel:      result.PriorityLabel,
		PriorityConfidence: result.PriorityConfidence,
		RiskLabel:          result.RiskLabel,
		RiskConfidence:     result.RiskConfidence,
		IntentLabel:        result.IntentLabel,
		IntentConfidence:   result.IntentConfidence,
		ProjectLabel:       result.ProjectLabel,
		ProjectConfidence:  result.ProjectConfidence,
		Summary:            result.Summary,
		LatencyMS:          result.LatencyMS,
		PromptVersion:      result.PromptVersion,
		WorkerReady:        h.aiWorkerReady,
		Answers:            make([]views.AIAnalysisPanelAnswer, 0, len(answers)),
	}
	for _, answer := range answers {
		data.Answers = append(data.Answers, views.AIAnalysisPanelAnswer{
			QuestionKey: answer.QuestionKey,
			Type:        answer.Type,
			Choice:      answer.Choice,
			Score:       answer.Score,
			Noul:        answer.Noul,
			Confidence:  answer.Confidence,
		})
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = views.AIAnalysisPanel(data).Render(r.Context(), w)
}

func (h *Handler) handleAIReanalyze(w http.ResponseWriter, r *http.Request) {
	if !h.aiWorkerReady {
		http.Error(w, "AI analysis is unavailable: the Jev worker is not configured", http.StatusConflict)
		return
	}
	messageID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || messageID <= 0 {
		http.Error(w, "invalid message id", http.StatusBadRequest)
		return
	}
	userID := h.userID(r.Context())
	var jobID string
	if err := h.withUserDB(r.Context(), userID, func(db *storage.DB) error {
		var err error
		jobID, err = db.EnqueueAnalysisForUser(r.Context(), userID, messageID)
		if err != nil {
			return err
		}
		after, _ := json.Marshal(map[string]string{"job_id": jobID})
		if _, err := aiaudit.Record(r.Context(), db, aiaudit.Event{
			UserID:      userID,
			ActorType:   aiaudit.ActorUser,
			ActorID:     userID,
			Action:      "ai.analysis.requeue",
			SubjectType: "message",
			SubjectID:   strconv.FormatInt(messageID, 10),
			AfterJSON:   string(after),
			IP:          r.RemoteAddr,
			UserAgent:   r.UserAgent(),
		}); err != nil {
			log.Printf("ai: record reanalysis audit event: %v", err)
		}
		return nil
	}); err != nil {
		switch {
		case errors.Is(err, sql.ErrNoRows):
			http.Error(w, "message not found", http.StatusNotFound)
		case errors.Is(err, storage.ErrAnalysisDisabled), errors.Is(err, storage.ErrAnalysisFolderNotAllowed):
			http.Error(w, err.Error(), http.StatusConflict)
		default:
			http.Error(w, "failed to queue analysis", http.StatusInternalServerError)
		}
		return
	}
	if h.aiEvents != nil {
		h.aiEvents.Publish(aievents.Event{
			Type:      "analysis.job",
			UserID:    userID,
			JobID:     jobID,
			MessageID: messageID,
			State:     storage.AnalysisStateQueued,
		})
	}
	if r.Header.Get("HX-Request") == "true" {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusAccepted)
		_ = views.AIAnalysisQueued(messageID).Render(r.Context(), w)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(map[string]string{"job_id": jobID})
}
