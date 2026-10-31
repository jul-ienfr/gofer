package jev

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/cristianadrielbraun/gofer/internal/storage"
)

const DefaultPromptVersion = "jev-v1.0.0"

type Classifier interface {
	Analyze(ctx context.Context, state State) (Response, error)
}

type PostProcessor interface {
	Process(ctx context.Context, result storage.AIAnalysisResult) error
}

type Analyzer struct {
	DB            *storage.DB
	Client        Classifier
	PromptVersion string
	Now           func() time.Time
	PostProcessor PostProcessor
	OnPostError   func(error)
}

func (a *Analyzer) Analyze(ctx context.Context, job storage.AnalysisJob) error {
	if a == nil || a.DB == nil {
		return errors.New("jev analyzer requires a database")
	}
	if a.Client == nil {
		return errors.New("jev analyzer requires a client")
	}
	if job.MessageID <= 0 || strings.TrimSpace(job.UserID) == "" {
		return errors.New("jev analysis job is incomplete")
	}
	state, err := LoadState(ctx, a.DB, job.UserID, job.MessageID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) || errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("%w: message source unavailable", storage.ErrAnalysisSkipped)
		}
		return err
	}
	now := time.Now
	if a.Now != nil {
		now = a.Now
	}
	started := now()
	response, err := a.Client.Analyze(ctx, state)
	if err != nil {
		return err
	}
	latency := now().Sub(started).Milliseconds()
	if latency < 0 {
		latency = 0
	}
	promptVersion := strings.TrimSpace(a.PromptVersion)
	if promptVersion == "" {
		promptVersion = DefaultPromptVersion
	}
	snapshot, err := a.DB.SnapshotRawMessage(ctx, job.UserID, job.MessageID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("%w: message disappeared during analysis", storage.ErrAnalysisSkipped)
		}
		return err
	}
	info := snapshot.Info()
	if job.AccountID != "" && info.AccountID != job.AccountID {
		return fmt.Errorf("%w: message moved to another account", storage.ErrAnalysisSkipped)
	}
	answers, err := storageAnswers(job.MessageID, response.Answers)
	if err != nil {
		return err
	}
	result := storage.AIAnalysisResult{
		MessageID:          job.MessageID,
		UserID:             job.UserID,
		AccountID:          info.AccountID,
		AnalysisVersion:    job.AnalysisVersion,
		Model:              response.Model,
		PriorityLabel:      priorityLabel(response.Answers.Priority.Score),
		PriorityConfidence: confidence(response.Answers.Priority.Confidence),
		RiskLabel:          response.Answers.Risk.Choice,
		RiskConfidence:     confidence(response.Answers.Risk.Confidence),
		IntentLabel:        intentLabel(response.Answers.NeedsReply.Noul),
		IntentConfidence:   confidence(response.Answers.NeedsReply.Confidence),
		ProjectLabel:       response.Answers.Domain.Choice,
		ProjectConfidence:  confidence(response.Answers.Domain.Confidence),
		InputTokens:        response.Usage.InputTokens,
		OutputTokens:       response.Usage.OutputTokens,
		LatencyMS:          latency,
		PromptVersion:      promptVersion,
	}
	if result.Model == "" {
		result.Model = DefaultModel
	}
	if err := a.DB.SaveAIAnalysis(ctx, result, answers); err != nil {
		return err
	}
	if a.PostProcessor != nil {
		if err := a.PostProcessor.Process(ctx, result); err != nil {
			if a.OnPostError != nil {
				a.OnPostError(err)
			}
		}
	}
	return nil
}

func storageAnswers(messageID int64, answers Answers) ([]storage.AIAnalysisAnswer, error) {
	priority, err := json.Marshal(answers.Priority.Probabilities)
	if err != nil {
		return nil, err
	}
	risk, err := json.Marshal(answers.Risk.Probabilities)
	if err != nil {
		return nil, err
	}
	domain, err := json.Marshal(answers.Domain.Probabilities)
	if err != nil {
		return nil, err
	}
	return []storage.AIAnalysisAnswer{
		{
			MessageID: messageID, QuestionKey: "priority", Type: "score",
			Score: float64(answers.Priority.Score), Confidence: confidence(answers.Priority.Confidence),
			ProbabilitiesJSON: string(priority),
		},
		{
			MessageID: messageID, QuestionKey: "needs_reply", Type: "noul",
			Noul: answers.NeedsReply.Noul, Confidence: confidence(answers.NeedsReply.Confidence),
			ProbabilitiesJSON: "{}",
		},
		{
			MessageID: messageID, QuestionKey: "risk", Type: "choice", Choice: answers.Risk.Choice,
			Confidence: confidence(answers.Risk.Confidence), ProbabilitiesJSON: string(risk),
		},
		{
			MessageID: messageID, QuestionKey: "domain", Type: "choice", Choice: answers.Domain.Choice,
			Confidence: confidence(answers.Domain.Confidence), ProbabilitiesJSON: string(domain),
		},
	}, nil
}

func confidence(value *float64) float64 {
	if value == nil {
		return 0
	}
	return *value
}

func priorityLabel(score int) string {
	switch score {
	case 0:
		return "low"
	case 1:
		return "normal"
	case 2:
		return "high"
	default:
		return "unknown"
	}
}

func intentLabel(noul float64) string {
	if noul >= 0.5 {
		return "reply_required"
	}
	return "no_reply"
}
