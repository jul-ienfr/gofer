package aipipeline

import (
	"context"
	"strings"
	"testing"

	"github.com/cristianadrielbraun/gofer/internal/aisettings"
	"github.com/cristianadrielbraun/gofer/internal/policy"
	"github.com/cristianadrielbraun/gofer/internal/storage"
)

func pipelineFixture(t *testing.T) (*storage.DB, *Processor) {
	t.Helper()
	db, err := storage.New(t.TempDir() + "/gofer.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.Write().Exec(`
		INSERT INTO users (id, username, username_normalized, name) VALUES ('default', 'default', 'default', 'Default');
		INSERT INTO accounts (id, user_id, email_address, provider) VALUES ('acc', 'default', 'ana@example.com', 'imap');
		INSERT INTO folders (id, account_id, remote_id, name, role) VALUES ('acc:INBOX', 'acc', 'INBOX', 'Inbox', 'inbox');
		INSERT INTO folders (id, account_id, remote_id, name, role) VALUES ('acc:Junk', 'acc', 'Junk', 'Junk', 'junk');
		INSERT INTO messages (id, account_id, internet_message_id, subject, from_email)
			VALUES (1, 'acc', '<one@example.com>', 'Interview', 'alice@example.com');
		INSERT INTO message_folder_state (message_id, folder_id, is_deleted) VALUES (1, 'acc:INBOX', 0);`); err != nil {
		t.Fatal(err)
	}
	if err := db.EnsureDefaultAIProjects(context.Background(), "default"); err != nil {
		t.Fatal(err)
	}
	if err := aisettings.Save(context.Background(), db, "default", aisettings.Settings{AnalysisEnabled: true, DraftingEnabled: true}); err != nil {
		t.Fatal(err)
	}
	projects, err := db.ListAIProjects(context.Background(), "default")
	if err != nil {
		t.Fatal(err)
	}
	for _, project := range projects {
		if project.Slug != "employment" {
			continue
		}
		if _, err := db.CreateAIProjectRoute(context.Background(), "default", storage.AIProjectRoute{
			ProjectID: project.ID, Name: "Recruiters", Enabled: true, Priority: 10,
			MatchJSON: `{"senders":["alice@example.com"]}`, ActionsJSON: `[{"kind":"notify"}]`,
		}); err != nil {
			t.Fatal(err)
		}
	}
	return db, &Processor{DB: db, Config: policy.DefaultConfig()}
}

func TestPipelineLinksProjectAndAuditsWithoutChanges(t *testing.T) {
	db, processor := pipelineFixture(t)
	result, err := processor.Run(context.Background(), storage.AIAnalysisResult{
		MessageID: 1, UserID: "default", AccountID: "acc", AnalysisVersion: 1,
		RiskLabel: "legit", RiskConfidence: 0.95, PriorityLabel: "high",
		IntentLabel: "reply_required", ProjectLabel: "employment", ProjectConfidence: 0.9,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Project == nil || result.Project.Slug != "employment" {
		t.Fatalf("project = %+v", result.Project)
	}
	if result.Decision.Risk != policy.RiskLegit || result.Decision.Priority != policy.PriorityHigh {
		t.Fatalf("decision = %+v", result.Decision)
	}
	if !result.Route.Matched || len(result.Route.Route.Actions) != 1 {
		t.Fatalf("route = %+v", result.Route)
	}
	links, err := db.ListAIMessageProjects(context.Background(), "default", 1)
	if err != nil || len(links) != 1 {
		t.Fatalf("links = %+v, %v", links, err)
	}
	profile, err := db.GetAILegitimacyProfile(context.Background(), "acc", "alice@example.com")
	if err != nil || profile.MessageCount != 1 {
		t.Fatalf("profile = %+v, %v", profile, err)
	}
	var audits int
	if err := db.Read().QueryRow(`SELECT COUNT(*) FROM ai_audit_events WHERE action = 'ai.policy.evaluate'`).Scan(&audits); err != nil || audits != 1 {
		t.Fatalf("audits = %d, %v", audits, err)
	}
	var state int
	if err := db.Read().QueryRow(`SELECT COUNT(*) FROM messages WHERE id = 1`).Scan(&state); err != nil || state != 1 {
		t.Fatalf("message changed: %d, %v", state, err)
	}
}

func TestPipelineQueuesOnlyExplicitExternalRoutes(t *testing.T) {
	db, processor := pipelineFixture(t)
	list, err := db.ListAIProjects(t.Context(), "default")
	if err != nil {
		t.Fatal(err)
	}
	var employment storage.AIProject
	for _, project := range list {
		if project.Slug == "employment" {
			employment = project
		}
	}
	if employment.ID == "" {
		t.Fatal("employment project missing")
	}
	connector, err := db.CreateAIConnector(t.Context(), storage.AIConnector{
		UserID: "default", Name: "n8n", Type: "n8n", URL: "https://example.com/hook",
		SecretEncrypted: "encrypted", Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.CreateAIProjectRoute(t.Context(), "default", storage.AIProjectRoute{
		ProjectID: employment.ID, Name: "Webhook", Enabled: true, Priority: 1,
		MatchJSON:   `{"senders":["alice@example.com"]}`,
		ActionsJSON: `[{"kind":"webhook","value":"` + connector.ID + `"}]`,
	}); err != nil {
		t.Fatal(err)
	}
	analysis := storage.AIAnalysisResult{
		MessageID: 1, UserID: "default", AccountID: "acc", AnalysisVersion: 1,
		RiskLabel: "legit", RiskConfidence: 0.95, PriorityLabel: "high",
		IntentLabel: "reply_required", ProjectLabel: "employment", ProjectConfidence: 0.9,
	}
	result, err := processor.Run(t.Context(), analysis)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.RoutingQueued) != 0 {
		t.Fatal("external routing queued without project permission")
	}
	employment.AllowExternalRouting = true
	if _, err := db.UpdateAIProject(t.Context(), employment); err != nil {
		t.Fatal(err)
	}
	result, err = processor.Run(t.Context(), analysis)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.RoutingQueued) != 1 {
		t.Fatalf("routing queued = %+v", result.RoutingQueued)
	}
	var payload, state string
	if err := db.Read().QueryRow(`SELECT payload_json, state FROM ai_routing_outbox WHERE id = ?`, result.RoutingQueued[0]).Scan(&payload, &state); err != nil {
		t.Fatal(err)
	}
	if state != "queued" || !strings.Contains(payload, `"project":"employment"`) || !strings.Contains(payload, `"subject":"Interview"`) {
		t.Fatalf("outbox state=%q payload=%s", state, payload)
	}
	employment.DataClassification = "restricted"
	if _, err := db.UpdateAIProject(t.Context(), employment); err != nil {
		t.Fatal(err)
	}
	result, err = processor.Run(t.Context(), analysis)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.RoutingQueued) != 0 {
		t.Fatal("restricted project queued external routing")
	}
	employment.DataClassification = "confidential"
	if _, err := db.UpdateAIProject(t.Context(), employment); err != nil {
		t.Fatal(err)
	}
	result, err = processor.Run(t.Context(), analysis)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.RoutingQueued) != 0 {
		t.Fatal("confidential project queued external routing")
	}
}

func TestPipelineMovesSpamOnlyWhenEveryGateIsOpen(t *testing.T) {
	db, processor := pipelineFixture(t)
	analysis := storage.AIAnalysisResult{
		MessageID: 1, UserID: "default", AccountID: "acc", AnalysisVersion: 1,
		RiskLabel: "spam", RiskConfidence: 0.97, PriorityLabel: "low",
		IntentLabel: "no_reply", ProjectLabel: "employment", ProjectConfidence: 0.9,
	}
	result, err := processor.Run(t.Context(), analysis)
	if err != nil {
		t.Fatal(err)
	}
	if result.SpamMoved {
		t.Fatal("spam moved without opt-in")
	}
	if err := aisettings.Save(t.Context(), db, "default", aisettings.Settings{
		AnalysisEnabled: true, DraftingEnabled: true, AutoSpamEnabled: true,
	}); err != nil {
		t.Fatal(err)
	}
	list, err := db.ListAIProjects(t.Context(), "default")
	if err != nil {
		t.Fatal(err)
	}
	for _, project := range list {
		if project.Slug != "employment" {
			continue
		}
		project.AllowAutoSpam = true
		if _, err := db.UpdateAIProject(t.Context(), project); err != nil {
			t.Fatal(err)
		}
	}
	result, err = processor.Run(t.Context(), analysis)
	if err != nil {
		t.Fatal(err)
	}
	if !result.SpamMoved || result.SpamError != "" || result.SpamActionID == "" {
		t.Fatalf("spam result = %+v", result)
	}
	var actionState, gate string
	if err := db.Read().QueryRow(`SELECT state, gate_json FROM ai_automation_actions WHERE id = ?`, result.SpamActionID).Scan(&actionState, &gate); err != nil || actionState != "executed" || !strings.Contains(gate, `"source":"acc:INBOX"`) {
		t.Fatalf("action = %q, gate = %q, %v", actionState, gate, err)
	}
	var folderID string
	if err := db.Read().QueryRow(`
		SELECT mfs.folder_id FROM message_folder_state mfs
		WHERE mfs.message_id = 1 AND mfs.is_deleted = 0`).Scan(&folderID); err != nil {
		t.Fatal(err)
	}
	if folderID != "acc:Junk" {
		t.Fatalf("folder after auto-spam = %q", folderID)
	}
	var audits int
	if err := db.Read().QueryRow(`SELECT COUNT(*) FROM ai_audit_events WHERE action = 'ai.automation.spam'`).Scan(&audits); err != nil || audits != 1 {
		t.Fatalf("spam audits = %d, %v", audits, err)
	}
}

func TestPipelineAppliesProjectLabelAndMoveActions(t *testing.T) {
	db, processor := pipelineFixture(t)
	if _, err := db.Write().Exec(`INSERT INTO folders (id, account_id, remote_id, name, role) VALUES ('acc:Archive', 'acc', 'Archive', 'Archive', 'archive')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Write().Exec(`UPDATE ai_project_routes SET actions_json = ? WHERE name = 'Recruiters'`,
		`[{"kind":"label","value":"Recruiters"},{"kind":"move","value":"Archive"}]`); err != nil {
		t.Fatal(err)
	}
	result, err := processor.Run(t.Context(), storage.AIAnalysisResult{
		MessageID: 1, UserID: "default", AccountID: "acc", AnalysisVersion: 1,
		RiskLabel: "legit", RiskConfidence: 0.97, PriorityLabel: "high",
		IntentLabel: "reply_required", ProjectLabel: "employment", ProjectConfidence: 0.92,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.ProjectActionError != "" || len(result.ProjectActions) < 3 {
		t.Fatalf("project actions = %+v, error = %q", result.ProjectActions, result.ProjectActionError)
	}
	var labels int
	if err := db.Read().QueryRow(`SELECT COUNT(*) FROM message_labels ml JOIN labels l ON l.id = ml.label_id WHERE ml.message_id = 1 AND l.name = 'Recruiters'`).Scan(&labels); err != nil || labels != 1 {
		t.Fatalf("labels = %d, %v", labels, err)
	}
	var folderID string
	if err := db.Read().QueryRow(`SELECT folder_id FROM message_folder_state WHERE message_id = 1`).Scan(&folderID); err != nil || folderID != "acc:Archive" {
		t.Fatalf("folder = %q, %v", folderID, err)
	}
	var actions int
	if err := db.Read().QueryRow(`SELECT COUNT(*) FROM ai_automation_actions WHERE message_id = 1 AND kind = 'route_move'`).Scan(&actions); err != nil || actions != 1 {
		t.Fatalf("move actions = %d, %v", actions, err)
	}
	var audits int
	if err := db.Read().QueryRow(`SELECT COUNT(*) FROM ai_audit_events WHERE action IN ('ai.projects.label','ai.projects.move')`).Scan(&audits); err != nil || audits != 2 {
		t.Fatalf("audits = %d, %v", audits, err)
	}
	result, err = processor.Run(t.Context(), storage.AIAnalysisResult{
		MessageID: 1, UserID: "default", AccountID: "acc", AnalysisVersion: 1,
		RiskLabel: "legit", RiskConfidence: 0.97, PriorityLabel: "high",
		IntentLabel: "reply_required", ProjectLabel: "employment", ProjectConfidence: 0.92,
	})
	if err != nil || result.ProjectActionError != "" {
		t.Fatalf("second run = %+v, %v", result.ProjectActions, err)
	}
	if err := db.Read().QueryRow(`SELECT COUNT(*) FROM ai_automation_actions WHERE message_id = 1 AND kind = 'route_move'`).Scan(&actions); err != nil || actions != 1 {
		t.Fatalf("move actions after second run = %d, %v", actions, err)
	}
}

func TestPipelineNeverDraftsOrRoutesPhishing(t *testing.T) {
	_, processor := pipelineFixture(t)
	result, err := processor.Run(context.Background(), storage.AIAnalysisResult{
		MessageID: 1, UserID: "default", AccountID: "acc", AnalysisVersion: 1,
		RiskLabel: "phishing", RiskConfidence: 0.99, PriorityLabel: "high",
		IntentLabel: "reply_required", ProjectLabel: "employment", ProjectConfidence: 0.99,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Route.Matched {
		t.Fatalf("phishing routed: %+v", result.Route)
	}
	for _, action := range result.Decision.Actions {
		if action.Kind == policy.ActionDraft || action.Kind == policy.ActionWebhook || action.Kind == policy.ActionSpam {
			t.Fatalf("unsafe action on phishing: %+v", result.Decision.Actions)
		}
	}
}
