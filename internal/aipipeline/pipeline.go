package aipipeline

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	stdmail "net/mail"
	"strings"
	"time"

	"github.com/cristianadrielbraun/gofer/internal/aiaudit"
	"github.com/cristianadrielbraun/gofer/internal/aisettings"
	"github.com/cristianadrielbraun/gofer/internal/legitimacy"
	"github.com/cristianadrielbraun/gofer/internal/policy"
	"github.com/cristianadrielbraun/gofer/internal/projects"
	"github.com/cristianadrielbraun/gofer/internal/storage"
	"github.com/cristianadrielbraun/gofer/internal/store"
	unsub "github.com/cristianadrielbraun/gofer/internal/unsubscribe"
)

type Processor struct {
	DB     *storage.DB
	Config policy.Config
	Now    func() time.Time
	Logf   func(format string, args ...any)
}

type Result struct {
	Decision           policy.Decision
	Route              projects.Decision
	Legitimacy         legitimacy.Result
	Project            *storage.AIProject
	RoutingQueued      []string
	SpamMoved          bool
	SpamActionID       string
	SpamError          string
	Unsubscribed       bool
	UnsubscribeError   string
	ProjectActions     []string
	ProjectActionError string
}

func (p *Processor) Process(ctx context.Context, analysis storage.AIAnalysisResult) error {
	_, err := p.Run(ctx, analysis)
	return err
}

func (p *Processor) Run(ctx context.Context, analysis storage.AIAnalysisResult) (Result, error) {
	if p == nil || p.DB == nil {
		return Result{}, errors.New("AI pipeline requires a database")
	}
	if analysis.MessageID <= 0 || strings.TrimSpace(analysis.UserID) == "" {
		return Result{}, errors.New("AI pipeline requires an analysis")
	}
	settings, err := aisettings.Load(ctx, p.DB, analysis.UserID)
	if err != nil {
		return Result{}, err
	}
	var accountID, senderEmail, senderDomain, subject string
	err = p.DB.Read().QueryRowContext(ctx, `
		SELECT m.account_id, LOWER(COALESCE(m.from_email, '')), COALESCE(m.subject, '')
		FROM messages m JOIN accounts a ON a.id = m.account_id
		WHERE m.id = ? AND a.user_id = ?`, analysis.MessageID, analysis.UserID).Scan(
		&accountID, &senderEmail, &subject)
	if errors.Is(err, sql.ErrNoRows) {
		return Result{}, nil
	}
	if err != nil {
		return Result{}, err
	}
	if at := strings.LastIndexByte(senderEmail, '@'); at >= 0 {
		senderDomain = senderEmail[at+1:]
	}
	project, err := p.projectForLabel(ctx, analysis.UserID, analysis.ProjectLabel)
	if err != nil {
		return Result{}, err
	}
	if project != nil {
		if err := p.DB.LinkAIMessageProject(ctx, analysis.UserID, storage.AIMessageProject{
			MessageID: analysis.MessageID, ProjectID: project.ID, Source: "jev", Confidence: analysis.ProjectConfidence,
		}); err != nil && !errors.Is(err, sql.ErrNoRows) {
			return Result{}, err
		}
	}
	legitimacyResult, profile, err := p.evaluateLegitimacy(ctx, analysis, accountID, senderEmail, senderDomain)
	if err != nil {
		return Result{}, err
	}
	if profile.AccountID != "" {
		if _, err := p.DB.SaveAILegitimacyProfile(ctx, profile); err != nil {
			return Result{}, err
		}
	}
	projectPermissions := policy.Project{}
	if project != nil {
		projectPermissions = policy.Project{
			AllowExternalRouting: project.AllowExternalRouting,
			AllowLLMDraft:        project.LLMDraftAllowed(),
			AllowAutoSpam:        project.AllowAutoSpam,
		}
	}
	config := p.Config
	if config.AnalysisMinConfidence == 0 {
		config = policy.DefaultConfig()
	}
	config.AnalyzeEnabled = true
	config.Drafting = settings.DraftingEnabled
	config.Routing = settings.AnalysisEnabled
	config.AutoSpam = settings.AutoSpamEnabled
	config.AutoUnsubscribe = settings.AutoUnsubscribeEnabled
	decision := policy.Evaluate(policy.Input{
		Analysis: policy.Analysis{
			Risk: policy.Risk(analysis.RiskLabel), RiskConfidence: analysis.RiskConfidence,
			Priority: policy.Priority(analysis.PriorityLabel), Intent: analysis.IntentLabel,
			ProjectID: projectID(project), ProjectSlug: analysis.ProjectLabel, ProjectName: projectName(project),
			ProjectConfidence: analysis.ProjectConfidence,
			NeedsReply:        analysis.IntentLabel == "reply_required",
			Signals:           policy.Signals{FirstContact: legitimacyResult.FirstContact},
		},
		Legitimacy: policy.Legitimacy{
			State: policy.LegitimacyState(legitimacyResult.State), Score: legitimacyResult.Score,
			VIP: legitimacyResult.IsVIP, Allowlisted: legitimacyResult.Allowlisted,
			Blocked: legitimacyResult.Blocked, RepliedBefore: legitimacyResult.RepliedBefore,
		},
		Project: projectPermissions,
		Config:  config,
	})
	result := Result{Decision: decision, Legitimacy: legitimacyResult, Project: project}
	if !decision.RequiresReview && hasAction(decision, policy.ActionSpam) {
		moved, actionID, moveErr := p.moveToSpam(ctx, analysis)
		result.SpamMoved = moved
		result.SpamActionID = actionID
		if moveErr != nil {
			result.SpamError = moveErr.Error()
			result.Decision.RequiresReview = true
			result.Decision.ReasonCodes = append(result.Decision.ReasonCodes, "auto_spam_failed")
			if p.Logf != nil {
				p.Logf("ai pipeline auto-spam message=%d: %v", analysis.MessageID, moveErr)
			}
		}
	}
	if !decision.RequiresReview && hasAction(decision, policy.ActionUnsubscribe) {
		unsubscribeResult, unsubscribeErr := unsub.Attempt(ctx, p.DB, analysis.UserID, analysis.MessageID)
		result.Unsubscribed = unsubscribeResult.Attempted
		if unsubscribeErr != nil {
			result.UnsubscribeError = unsubscribeErr.Error()
			result.Decision.RequiresReview = true
			result.Decision.ReasonCodes = append(result.Decision.ReasonCodes, "auto_unsubscribe_failed")
			if p.Logf != nil {
				p.Logf("ai pipeline unsubscribe message=%d: %v", analysis.MessageID, unsubscribeErr)
			}
		}
		if unsubscribeResult.Attempted {
			_, auditErr := aiaudit.Record(ctx, p.DB, aiaudit.Event{
				UserID: analysis.UserID, ActorType: "system", Action: "ai.automation.unsubscribe",
				SubjectType: "message", SubjectID: fmt.Sprintf("%d", analysis.MessageID),
				AfterJSON: fmt.Sprintf(`{"status":%d,"target_hash":%q}`, unsubscribeResult.StatusCode, unsubscribeResult.TargetHash),
			})
			if auditErr != nil && p.Logf != nil {
				p.Logf("ai pipeline unsubscribe audit message=%d: %v", analysis.MessageID, auditErr)
			}
		}
	}
	routeDecision := projects.Decision{}
	if project != nil && settings.AnalysisEnabled {
		routes, err := loadRoutes(ctx, p.DB, analysis.UserID, project)
		if err != nil {
			return Result{}, err
		}
		routeDecision = projects.Evaluate(projects.Candidate{
			Sender: senderEmail, SenderDomain: senderDomain, Subject: subject,
			Intent: analysis.IntentLabel, Risk: analysis.RiskLabel,
			Confidence: analysis.ProjectConfidence,
		}, routes)
	}
	result.Route = routeDecision
	if project != nil && routeDecision.Matched && !routeDecision.RequiresReview {
		applied, actionErr := p.applyLocalRouteActions(ctx, analysis, routeDecision)
		result.ProjectActions = applied
		if actionErr != nil {
			result.ProjectActionError = actionErr.Error()
			if p.Logf != nil {
				p.Logf("ai pipeline project action message=%d: %v", analysis.MessageID, actionErr)
			}
		}
	}
	if project != nil && project.ExternalRoutingAllowed() && routeDecision.Matched && !routeDecision.RequiresReview {
		queued, err := p.queueRoute(ctx, analysis, result, senderEmail, senderDomain, subject)
		if err != nil {
			return Result{}, err
		}
		result.RoutingQueued = queued
	}
	p.audit(ctx, analysis, result)
	return result, nil
}

func (p *Processor) applyLocalRouteActions(ctx context.Context, analysis storage.AIAnalysisResult, decision projects.Decision) ([]string, error) {
	var applied []string
	var errs []error
	for _, action := range decision.Route.Actions {
		switch action.Kind {
		case "label":
			name := strings.TrimSpace(action.Value)
			if name == "" || len(name) > 128 || strings.ContainsAny(name, "\r\n\x00") {
				errs = append(errs, fmt.Errorf("invalid label name"))
				continue
			}
			if _, err := p.DB.AddMessageLabel(ctx, analysis.MessageID, analysis.AccountID, storage.LabelInput{
				Name: name, ProviderID: name, ProviderType: storage.LabelProviderLocal,
			}); err != nil {
				errs = append(errs, fmt.Errorf("apply label %q: %w", name, err))
				continue
			}
			applied = append(applied, "label:"+name)
			if _, err := aiaudit.Record(ctx, p.DB, aiaudit.Event{
				UserID: analysis.UserID, ActorType: "system", Action: "ai.projects.label",
				SubjectType: "message", SubjectID: fmt.Sprintf("%d", analysis.MessageID),
				AfterJSON: fmt.Sprintf(`{"label":%q,"route":%q}`, name, decision.Route.ID),
			}); err != nil && p.Logf != nil {
				p.Logf("ai pipeline label audit message=%d: %v", analysis.MessageID, err)
			}
		case "move":
			actionID, destination, err := p.applyRouteMove(ctx, analysis, decision.Route, action.Value)
			if err != nil {
				errs = append(errs, err)
				continue
			}
			applied = append(applied, "move:"+destination)
			if actionID != "" {
				applied = append(applied, "undo:"+actionID)
			}
		}
	}
	return applied, errors.Join(errs...)
}

func (p *Processor) applyRouteMove(ctx context.Context, analysis storage.AIAnalysisResult, route projects.Route, target string) (string, string, error) {
	target = strings.TrimSpace(target)
	if target == "" {
		return "", "", fmt.Errorf("route move target is empty")
	}
	var sourceID, destinationID string
	err := p.DB.Read().QueryRowContext(ctx, `
		SELECT mfs.folder_id, dst.id
		FROM messages m
		JOIN accounts a ON a.id = m.account_id
		JOIN message_folder_state mfs ON mfs.message_id = m.id AND mfs.is_deleted = 0
		JOIN folders dst ON dst.account_id = m.account_id
		WHERE m.id = ? AND a.user_id = ?
		  AND COALESCE(dst.remote_id, '') <> ''
		  AND (lower(dst.name) = lower(?) OR lower(COALESCE(dst.role, '')) = lower(?))
		ORDER BY dst.id
		LIMIT 1`, analysis.MessageID, analysis.UserID, target, target).Scan(&sourceID, &destinationID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", fmt.Errorf("route move target %q not found", target)
	}
	if err != nil {
		return "", "", err
	}
	if sourceID == destinationID {
		return "", destinationID, nil
	}
	if err := p.DB.MoveMessageAndQueueForUser(ctx, analysis.MessageID, sourceID, destinationID, analysis.UserID); err != nil {
		return "", "", fmt.Errorf("queue route move: %w", err)
	}
	now := time.Now().UTC()
	if p.Now != nil {
		now = p.Now()
	}
	gate, _ := json.Marshal(map[string]string{"source": sourceID, "destination": destinationID})
	actionID, recordErr := p.DB.RecordAIAutomationAction(ctx, storage.AIAutomationAction{
		MessageID: analysis.MessageID, Kind: "route_move", GateJSON: string(gate),
		Confidence: analysis.ProjectConfidence, Reason: "route:" + route.ID,
		ExecutedAt: now, UndoDeadline: now.Add(7 * 24 * time.Hour),
		Result: fmt.Sprintf(`{"from":%q,"to":%q}`, sourceID, destinationID),
	})
	if recordErr != nil && p.Logf != nil {
		p.Logf("ai pipeline route move record message=%d: %v", analysis.MessageID, recordErr)
	}
	if _, err := aiaudit.Record(ctx, p.DB, aiaudit.Event{
		UserID: analysis.UserID, ActorType: "system", Action: "ai.projects.move",
		SubjectType: "message", SubjectID: fmt.Sprintf("%d", analysis.MessageID),
		AfterJSON: fmt.Sprintf(`{"from":%q,"to":%q,"route":%q}`, sourceID, destinationID, route.ID),
	}); err != nil && p.Logf != nil {
		p.Logf("ai pipeline route move audit message=%d: %v", analysis.MessageID, err)
	}
	return actionID, destinationID, nil
}

func (p *Processor) queueRoute(ctx context.Context, analysis storage.AIAnalysisResult, result Result, sender, domain, subject string) ([]string, error) {
	var queued []string
	for _, action := range result.Route.Route.Actions {
		if action.Kind != "webhook" || strings.TrimSpace(action.Value) == "" {
			continue
		}
		connector, err := p.DB.GetAIConnector(ctx, analysis.UserID, action.Value)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				continue
			}
			return nil, err
		}
		if !connector.Enabled || connector.SecretEncrypted == "" {
			continue
		}
		payload, err := json.Marshal(map[string]any{
			"version":    "routing-v1.0.0",
			"message_id": analysis.MessageID, "route_id": result.Route.Route.ID,
			"project": result.Project.Slug, "sender": sender, "sender_domain": domain,
			"subject": subject, "risk": analysis.RiskLabel, "priority": analysis.PriorityLabel,
			"intent": analysis.IntentLabel, "confidence": analysis.ProjectConfidence,
		})
		if err != nil {
			return nil, err
		}
		item, err := p.DB.EnqueueAIRouting(ctx, analysis.UserID, storage.AIRoutingItem{
			MessageID: analysis.MessageID, ProjectID: result.Project.ID, ConnectorID: connector.ID,
			EventType:      "message.routed",
			PayloadJSON:    string(payload),
			IdempotencyKey: fmt.Sprintf("jev-route:%d:%s", analysis.MessageID, connector.ID),
		})
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				continue
			}
			return nil, err
		}
		if item.ID != "" {
			queued = append(queued, item.ID)
		}
	}
	return queued, nil
}

func (p *Processor) projectForLabel(ctx context.Context, userID, label string) (*storage.AIProject, error) {
	label = strings.ToLower(strings.TrimSpace(label))
	if label == "" || label == "other" {
		return nil, nil
	}
	list, err := p.DB.ListAIProjects(ctx, userID)
	if err != nil {
		return nil, err
	}
	for i := range list {
		if list[i].Slug == label && list[i].Status == "active" {
			return &list[i], nil
		}
	}
	return nil, nil
}

func (p *Processor) evaluateLegitimacy(ctx context.Context, analysis storage.AIAnalysisResult, accountID, senderEmail, senderDomain string) (legitimacy.Result, storage.AILegitimacyProfile, error) {
	now := time.Now().UTC()
	if p.Now != nil {
		now = p.Now()
	}
	profile, err := p.DB.GetAILegitimacyProfile(ctx, accountID, senderEmail)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return legitimacy.Result{}, storage.AILegitimacyProfile{}, err
	}
	if errors.Is(err, sql.ErrNoRows) {
		profile = storage.AILegitimacyProfile{AccountID: accountID, SenderEmail: senderEmail, SenderDomain: senderDomain, FirstSeen: now}
	}
	headers, err := p.messageHeaders(ctx, analysis.UserID, analysis.MessageID, senderDomain)
	if err != nil {
		return legitimacy.Result{}, storage.AILegitimacyProfile{}, err
	}
	result := legitimacy.Evaluate(legitimacy.Observation{
		MessageCount: profile.MessageCount, ReplyCount: profile.ReplyCount,
		SentCount: profile.SentCount, SpamCount: profile.SpamCount, HamCount: profile.HamCount,
		UserCorrectionCount: profile.UserCorrectionCount,
		AuthPassCount:       profile.AuthPassCount, AuthFailCount: profile.AuthFailCount,
		FirstSeen: profile.FirstSeen, LastSeen: profile.LastSeen,
		VIP: profile.VIP, Allowlisted: profile.Allowlisted, Blocked: profile.Blocked,
		KnownContact: profile.MessageCount > 0,
	}, headers, analysis.RiskLabel, now)
	profile.MessageCount++
	profile.LastSeen = now
	if result.AuthPass {
		profile.AuthPassCount++
	} else if headers.Auth == legitimacy.AuthFail {
		profile.AuthFailCount++
	}
	profile.Score = result.Score
	profile.State = string(result.State)
	return result, profile, nil
}

func (p *Processor) messageHeaders(ctx context.Context, userID string, messageID int64, senderDomain string) (legitimacy.Headers, error) {
	snapshot, err := p.DB.SnapshotRawMessage(ctx, userID, messageID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return legitimacy.Headers{Auth: legitimacy.AuthAbsent, FromDomain: senderDomain}, nil
		}
		return legitimacy.Headers{}, err
	}
	rawPath := snapshot.Info().RawPath
	if rawPath == "" {
		return legitimacy.Headers{Auth: legitimacy.AuthAbsent, FromDomain: senderDomain}, nil
	}
	raw, err := store.ReadMessageFile(rawPath)
	if err != nil {
		return legitimacy.Headers{Auth: legitimacy.AuthAbsent, FromDomain: senderDomain}, nil
	}
	message, err := stdmail.ReadMessage(bytes.NewReader(raw))
	if err != nil {
		return legitimacy.Headers{Auth: legitimacy.AuthAbsent, FromDomain: senderDomain}, nil
	}
	knownDomains := []string{senderDomain}
	if account := snapshot.Info().EmailAddress; account != "" {
		if at := strings.LastIndexByte(account, '@'); at >= 0 {
			knownDomains = append(knownDomains, account[at+1:])
		}
	}
	headers := legitimacy.ParseHeaders(message.Header, knownDomains)
	headers.Auth = legitimacy.ParseAuthentication(message.Header)
	return headers, nil
}

func loadRoutes(ctx context.Context, db *storage.DB, userID string, project *storage.AIProject) ([]projects.Route, error) {
	stored, err := db.ListAIProjectRoutes(ctx, userID, project.ID)
	if err != nil {
		return nil, err
	}
	routes := make([]projects.Route, 0, len(stored))
	for _, route := range stored {
		if !route.Enabled {
			continue
		}
		parsed, err := projects.ParseRoute(route.ID, route.ProjectID, route.Name, route.Priority, route.Enabled, route.MatchJSON, route.ActionsJSON)
		if err != nil {
			continue
		}
		routes = append(routes, parsed)
	}
	return routes, nil
}

func hasAction(decision policy.Decision, kind policy.ActionKind) bool {
	for _, action := range decision.Actions {
		if action.Kind == kind {
			return true
		}
	}
	return false
}

func (p *Processor) moveToSpam(ctx context.Context, analysis storage.AIAnalysisResult) (bool, string, error) {
	var sourceID, destinationID string
	err := p.DB.Read().QueryRowContext(ctx, `
		SELECT src.id, dst.id
		FROM messages m
		JOIN accounts a ON a.id = m.account_id
		JOIN message_folder_state mfs ON mfs.message_id = m.id AND mfs.is_deleted = 0
		JOIN folders src ON src.id = mfs.folder_id AND src.role = 'inbox'
		JOIN folders dst ON dst.account_id = m.account_id
			AND dst.role IN ('junk', 'spam') AND COALESCE(dst.remote_id, '') <> ''
		WHERE m.id = ? AND a.user_id = ?
		ORDER BY CASE dst.role WHEN 'junk' THEN 0 ELSE 1 END, dst.id
		LIMIT 1`, analysis.MessageID, analysis.UserID).Scan(&sourceID, &destinationID)
	if errors.Is(err, sql.ErrNoRows) {
		return false, "", nil
	}
	if err != nil {
		return false, "", err
	}
	if sourceID == destinationID {
		return false, "", nil
	}
	if err := p.DB.MoveMessageAndQueueForUser(ctx, analysis.MessageID, sourceID, destinationID, analysis.UserID); err != nil {
		return false, "", err
	}
	now := time.Now().UTC()
	if p.Now != nil {
		now = p.Now()
	}
	gate, _ := json.Marshal(map[string]string{"source": sourceID, "destination": destinationID})
	actionID, recordErr := p.DB.RecordAIAutomationAction(ctx, storage.AIAutomationAction{
		MessageID: analysis.MessageID, Kind: "move_junk", GateJSON: string(gate),
		Confidence: analysis.RiskConfidence, Reason: "auto_spam",
		ExecutedAt: now, UndoDeadline: now.Add(7 * 24 * time.Hour),
		Result: fmt.Sprintf(`{"from":%q,"to":%q}`, sourceID, destinationID),
	})
	if recordErr != nil && p.Logf != nil {
		p.Logf("ai pipeline auto-spam record message=%d: %v", analysis.MessageID, recordErr)
	}
	if _, err := aiaudit.Record(ctx, p.DB, aiaudit.Event{
		UserID: analysis.UserID, ActorType: "system", Action: "ai.automation.spam",
		SubjectType: "message", SubjectID: fmt.Sprintf("%d", analysis.MessageID),
		AfterJSON: fmt.Sprintf(`{"from":%q,"to":%q}`, sourceID, destinationID),
	}); err != nil && p.Logf != nil {
		p.Logf("ai pipeline auto-spam audit message=%d: %v", analysis.MessageID, err)
	}
	return true, actionID, nil
}

func (p *Processor) audit(ctx context.Context, analysis storage.AIAnalysisResult, result Result) {
	payload, err := json.Marshal(map[string]any{
		"decision": result.Decision, "route": routeAudit(result.Route),
		"legitimacy":     map[string]any{"state": result.Legitimacy.State, "score": result.Legitimacy.Score},
		"routing_queued": append([]string(nil), result.RoutingQueued...),
		"spam_moved":     result.SpamMoved, "spam_action_id": result.SpamActionID, "spam_error": result.SpamError,
		"unsubscribed": result.Unsubscribed, "unsubscribe_error": result.UnsubscribeError,
		"project_actions": result.ProjectActions, "project_action_error": result.ProjectActionError,
	})
	if err != nil {
		return
	}
	if _, err := aiaudit.Record(ctx, p.DB, aiaudit.Event{
		UserID: analysis.UserID, ActorType: "system", Action: "ai.policy.evaluate",
		SubjectType: "message", SubjectID: fmt.Sprintf("%d", analysis.MessageID), AfterJSON: string(payload),
	}); err != nil && p.Logf != nil {
		p.Logf("ai pipeline audit message=%d: %v", analysis.MessageID, err)
	}
}

func routeAudit(decision projects.Decision) map[string]any {
	if !decision.Matched {
		return map[string]any{"matched": false, "requires_review": decision.RequiresReview}
	}
	return map[string]any{
		"matched": true, "route_id": decision.Route.ID, "route_name": decision.Route.Name,
		"actions": decision.Route.Actions, "requires_review": decision.RequiresReview,
	}
}

func projectID(project *storage.AIProject) string {
	if project == nil {
		return ""
	}
	return project.ID
}

func projectName(project *storage.AIProject) string {
	if project == nil {
		return ""
	}
	return project.Name
}
