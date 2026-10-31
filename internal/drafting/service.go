package drafting

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/cristianadrielbraun/gofer/internal/llm"
	"github.com/cristianadrielbraun/gofer/internal/models"
	"github.com/cristianadrielbraun/gofer/internal/storage"
	"github.com/google/uuid"
)

const (
	PromptVersion = "draft-v1.0.0"
	MaxThread     = 3
)

var (
	ErrMessageNotFound    = errors.New("message not found")
	ErrAnalysisMissing    = errors.New("analysis is required before drafting")
	ErrUnsafeAnalysis     = errors.New("drafting is disabled for this message")
	ErrProjectDraftDenied = errors.New("the project does not allow LLM drafting")
)

type Generator interface {
	Draft(ctx context.Context, input llm.DraftInput) (llm.Draft, llm.Completion, error)
}

type Service struct {
	DB        *storage.DB
	Generator Generator
}

type Request struct {
	UserID      string
	MessageID   int64
	Instruction string
	Tone        string
}

type Result struct {
	Draft    llm.Draft
	Model    string
	Usage    llm.Usage
	Latency  time.Duration
	Warnings []string
	Compose  Compose
}

type Compose struct {
	AccountID  string `json:"account_id"`
	To         string `json:"to"`
	Subject    string `json:"subject"`
	Body       string `json:"body"`
	InReplyTo  string `json:"in_reply_to"`
	References string `json:"references"`
}

func (s *Service) Generate(ctx context.Context, request Request) (Result, error) {
	if s == nil || s.DB == nil || s.Generator == nil {
		return Result{}, errors.New("drafting service is not configured")
	}
	if strings.TrimSpace(request.UserID) == "" || request.MessageID <= 0 {
		return Result{}, ErrMessageNotFound
	}
	var accountID, threadID, accountEmail, accountName, subject, fromEmail, preview string
	var internetMessageID, references string
	err := s.DB.Read().QueryRowContext(ctx, `
		SELECT m.account_id, COALESCE(m.thread_id, ''), a.email_address, COALESCE(a.display_name, ''),
		       COALESCE(m.subject, ''), COALESCE(m.from_email, ''),
		       COALESCE(m.preview_text, ''), COALESCE(m.internet_message_id, ''),
		       COALESCE(m."references", '')
		FROM messages m
		JOIN accounts a ON a.id = m.account_id
		WHERE m.id = ? AND a.user_id = ? AND COALESCE(a.is_deleting, 0) = 0
		  AND EXISTS (
			SELECT 1 FROM message_folder_state ms
			JOIN folders f ON f.id = ms.folder_id
			WHERE ms.message_id = m.id AND ms.is_deleted = 0
			  AND COALESCE(f.role, '') NOT IN ('drafts', 'sent', 'trash')
		  )`, request.MessageID, request.UserID).Scan(
		&accountID, &threadID, &accountEmail, &accountName, &subject, &fromEmail, &preview,
		&internetMessageID, &references)
	if errors.Is(err, sql.ErrNoRows) {
		return Result{}, ErrMessageNotFound
	}
	if err != nil {
		return Result{}, err
	}
	analysis, _, err := s.DB.GetAIAnalysisForUser(ctx, request.MessageID, request.UserID)
	if errors.Is(err, sql.ErrNoRows) {
		return Result{}, ErrAnalysisMissing
	}
	if err != nil {
		return Result{}, err
	}
	if analysis.RiskLabel != "legit" && analysis.RiskLabel != "unsure" {
		return Result{}, ErrUnsafeAnalysis
	}
	if project, err := projectBySlug(ctx, s.DB, request.UserID, analysis.ProjectLabel); err != nil {
		return Result{}, err
	} else if project != nil && !project.LLMDraftAllowed() {
		return Result{}, ErrProjectDraftDenied
	}
	thread := make([]llm.ThreadMessage, 0, MaxThread)
	if threadID != "" {
		items, err := s.DB.GetThreadMessagesForUser(ctx, accountID, threadID, request.UserID)
		if err == nil {
			start := 0
			if len(items) > MaxThread {
				start = len(items) - MaxThread
			}
			for _, item := range items[start:] {
				thread = append(thread, llm.ThreadMessage{
					From: item.From.Email, To: contactsText(item.To), Subject: item.Subject,
					Body: truncateRunes(item.TextBody, 12000), Date: item.Date,
				})
			}
		}
	}
	if len(thread) == 0 {
		body := preview
		if strings.TrimSpace(body) == "" {
			body = analysis.Summary
		}
		thread = append(thread, llm.ThreadMessage{From: fromEmail, To: accountEmail, Subject: subject, Body: truncateRunes(body, 12000)})
	}
	instruction := strings.TrimSpace(request.Instruction)
	if instruction == "" {
		instruction = "Write a concise reply that answers the sender and asks for missing information when needed."
	}
	input := llm.DraftInput{
		Instruction: instruction,
		Thread:      thread,
		Identity:    llm.Identity{Name: accountName, Email: accountEmail, Tone: request.Tone},
		Tone:        request.Tone,
	}
	draft, completion, err := s.Generator.Draft(ctx, input)
	if err != nil {
		return Result{}, err
	}
	if strings.TrimSpace(draft.ReplyText) == "" {
		return Result{}, errors.New("draft is empty")
	}
	replySubject := subject
	if !strings.HasPrefix(strings.ToLower(strings.TrimSpace(subject)), "re:") {
		replySubject = "Re: " + subject
	}
	replyReferences := strings.TrimSpace(references)
	if internetMessageID != "" && !strings.Contains(replyReferences, internetMessageID) {
		replyReferences = strings.TrimSpace(replyReferences + " " + internetMessageID)
	}
	result := Result{
		Draft: draft, Model: completion.Model, Usage: completion.Usage, Latency: completion.Latency,
		Compose: Compose{
			AccountID: accountID, To: fromEmail, Subject: replySubject, Body: draft.ReplyText,
			InReplyTo: internetMessageID, References: replyReferences,
		},
	}
	if draft.NeedsHuman {
		result.Warnings = append(result.Warnings, "The model requested human review before sending.")
	}
	if containsPlaceholder(draft.ReplyText) {
		result.Warnings = append(result.Warnings, "The draft contains a placeholder to complete before sending.")
	}
	if err := s.recordGeneration(ctx, request, result); err != nil {
		return Result{}, err
	}
	return result, nil
}

func (s *Service) recordGeneration(ctx context.Context, request Request, result Result) error {
	_, err := s.DB.Write().ExecContext(ctx, `
		INSERT INTO ai_llm_generations (
			id, message_id, purpose, prompt_version, status,
			input_tokens, output_tokens, latency_ms
		) VALUES (?, ?, 'draft', ?, 'done', ?, ?, ?)`,
		uuid.NewString(), request.MessageID, PromptVersion,
		result.Usage.PromptTokens, result.Usage.CompletionTokens, result.Latency.Milliseconds())
	return err
}

func projectBySlug(ctx context.Context, db *storage.DB, userID, slug string) (*storage.AIProject, error) {
	slug = strings.ToLower(strings.TrimSpace(slug))
	if slug == "" || slug == "other" {
		return nil, nil
	}
	projects, err := db.ListAIProjects(ctx, userID)
	if err != nil {
		return nil, err
	}
	for i := range projects {
		if projects[i].Slug == slug {
			return &projects[i], nil
		}
	}
	return nil, nil
}

func contactsText(contacts []models.Contact) string {
	emails := make([]string, 0, len(contacts))
	for _, contact := range contacts {
		if contact.Email != "" {
			emails = append(emails, contact.Email)
		}
	}
	return strings.Join(emails, ", ")
}

func truncateRunes(value string, limit int) string {
	value = strings.TrimSpace(value)
	runes := []rune(value)
	if len(runes) > limit {
		return string(runes[:limit])
	}
	return value
}

func containsPlaceholder(value string) bool {
	lower := strings.ToLower(value)
	return strings.Contains(lower, "[à compléter]") ||
		strings.Contains(lower, "[to be completed]") ||
		strings.Contains(lower, "{{") ||
		strings.Contains(lower, "todo")
}
