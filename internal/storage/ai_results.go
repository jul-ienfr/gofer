package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

var ErrAnalysisSkipped = errors.New("analysis skipped")

type AIAnalysisResult struct {
	MessageID          int64
	UserID             string
	AccountID          string
	AnalysisVersion    int
	Model              string
	PriorityLabel      string
	PriorityConfidence float64
	RiskLabel          string
	RiskConfidence     float64
	IntentLabel        string
	IntentConfidence   float64
	ProjectLabel       string
	ProjectConfidence  float64
	Summary            string
	InputTokens        int64
	OutputTokens       int64
	CostMicros         int64
	LatencyMS          int64
	PromptVersion      string
	PolicyVersion      string
}

type AIAnalysisAnswer struct {
	MessageID         int64
	QuestionKey       string
	Type              string
	Choice            string
	Score             float64
	Noul              float64
	Confidence        float64
	ProbabilitiesJSON string
}

func (db *DB) SaveAIAnalysis(ctx context.Context, result AIAnalysisResult, answers []AIAnalysisAnswer) error {
	if result.MessageID <= 0 || strings.TrimSpace(result.AccountID) == "" || strings.TrimSpace(result.UserID) == "" {
		return errors.New("AI result requires a message, account and user")
	}
	if result.AnalysisVersion <= 0 {
		result.AnalysisVersion = CurrentAnalysisVersion
	}
	for _, answer := range answers {
		if answer.MessageID != result.MessageID {
			return errors.New("AI answers must belong to the result message")
		}
	}
	tx, err := db.Write().BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var owned int
	if err := tx.QueryRowContext(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM messages m JOIN accounts a ON a.id = m.account_id
			WHERE m.id = ? AND m.account_id = ? AND a.user_id = ? AND COALESCE(a.is_deleting, 0) = 0
		)`, result.MessageID, result.AccountID, result.UserID).Scan(&owned); err != nil {
		return err
	}
	if owned != 1 {
		return sql.ErrNoRows
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO ai_analysis_results
			(message_id, account_id, analysis_version, model, priority_label, priority_confidence,
			 risk_label, risk_confidence, intent_label, intent_confidence, project_label, project_confidence,
			 summary, usage_input_tokens, usage_output_tokens, usage_cost_micros, latency_ms, prompt_version, policy_version)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(message_id) DO UPDATE SET
			account_id=excluded.account_id, analysis_version=excluded.analysis_version, model=excluded.model,
			priority_label=excluded.priority_label, priority_confidence=excluded.priority_confidence,
			risk_label=excluded.risk_label, risk_confidence=excluded.risk_confidence,
			intent_label=excluded.intent_label, intent_confidence=excluded.intent_confidence,
			project_label=excluded.project_label, project_confidence=excluded.project_confidence,
			summary=excluded.summary, usage_input_tokens=excluded.usage_input_tokens,
			usage_output_tokens=excluded.usage_output_tokens, usage_cost_micros=excluded.usage_cost_micros,
			latency_ms=excluded.latency_ms, prompt_version=excluded.prompt_version,
			policy_version=excluded.policy_version, created_at=CURRENT_TIMESTAMP`,
		result.MessageID, result.AccountID, result.AnalysisVersion, result.Model,
		result.PriorityLabel, clampConfidence(result.PriorityConfidence),
		result.RiskLabel, clampConfidence(result.RiskConfidence),
		result.IntentLabel, clampConfidence(result.IntentConfidence),
		result.ProjectLabel, clampConfidence(result.ProjectConfidence),
		result.Summary, result.InputTokens, result.OutputTokens, result.CostMicros,
		result.LatencyMS, result.PromptVersion, result.PolicyVersion); err != nil {
		return fmt.Errorf("save AI result: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM ai_analysis_answers WHERE message_id = ?`, result.MessageID); err != nil {
		return err
	}
	for _, answer := range answers {
		if strings.TrimSpace(answer.QuestionKey) == "" {
			return errors.New("AI answer requires a question key")
		}
		probabilities := strings.TrimSpace(answer.ProbabilitiesJSON)
		if probabilities == "" || !json.Valid([]byte(probabilities)) {
			probabilities = "{}"
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO ai_analysis_answers
				(message_id, question_key, type, choice, score, noul, confidence, probabilities_json)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			result.MessageID, answer.QuestionKey, answer.Type, answer.Choice, answer.Score,
			answer.Noul, clampConfidence(answer.Confidence), probabilities); err != nil {
			return fmt.Errorf("save AI answer: %w", err)
		}
	}
	return tx.Commit()
}

func (db *DB) GetAIAnalysisForUser(ctx context.Context, messageID int64, userID string) (*AIAnalysisResult, []AIAnalysisAnswer, error) {
	if messageID <= 0 || strings.TrimSpace(userID) == "" {
		return nil, nil, sql.ErrNoRows
	}
	var result AIAnalysisResult
	err := db.Read().QueryRowContext(ctx, `
		SELECT r.message_id, a.user_id, r.account_id, r.analysis_version, r.model, r.priority_label,
			r.priority_confidence, r.risk_label, r.risk_confidence, r.intent_label,
			r.intent_confidence, r.project_label, r.project_confidence, r.summary,
			r.usage_input_tokens, r.usage_output_tokens, r.usage_cost_micros, r.latency_ms,
			r.prompt_version, r.policy_version
		FROM ai_analysis_results r
		JOIN messages m ON m.id = r.message_id
		JOIN accounts a ON a.id = m.account_id
		WHERE r.message_id = ? AND a.user_id = ?`, messageID, userID).
		Scan(&result.MessageID, &result.UserID, &result.AccountID, &result.AnalysisVersion, &result.Model,
			&result.PriorityLabel, &result.PriorityConfidence, &result.RiskLabel, &result.RiskConfidence,
			&result.IntentLabel, &result.IntentConfidence, &result.ProjectLabel, &result.ProjectConfidence,
			&result.Summary, &result.InputTokens, &result.OutputTokens, &result.CostMicros,
			&result.LatencyMS, &result.PromptVersion, &result.PolicyVersion)
	if err != nil {
		return nil, nil, err
	}
	rows, err := db.Read().QueryContext(ctx, `
		SELECT message_id, question_key, type, choice, score, noul, confidence, probabilities_json
		FROM ai_analysis_answers WHERE message_id = ? ORDER BY question_key`, messageID)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	answers := make([]AIAnalysisAnswer, 0, 4)
	for rows.Next() {
		var answer AIAnalysisAnswer
		if err := rows.Scan(&answer.MessageID, &answer.QuestionKey, &answer.Type, &answer.Choice,
			&answer.Score, &answer.Noul, &answer.Confidence, &answer.ProbabilitiesJSON); err != nil {
			return nil, nil, err
		}
		answers = append(answers, answer)
	}
	return &result, answers, rows.Err()
}

func clampConfidence(value float64) float64 {
	if value < 0 {
		return 0
	}
	if value > 1 {
		return 1
	}
	return value
}
