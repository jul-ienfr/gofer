package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/google/uuid"
)

type AIProject struct {
	ID                   string
	UserID               string
	Name                 string
	Slug                 string
	Description          string
	Instructions         string
	Color                string
	Status               string
	AllowExternalRouting bool
	AllowLLMDraft        *bool
	AllowAutoSpam        bool
	DataClassification   string
}

func (p AIProject) LLMDraftAllowed() bool {
	if p.AllowLLMDraft == nil {
		return true
	}
	return *p.AllowLLMDraft
}

func (p AIProject) ExternalRoutingAllowed() bool {
	if !p.AllowExternalRouting {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(p.DataClassification)) {
	case "public", "internal":
		return true
	default:
		return false
	}
}

type AIProjectRoute struct {
	ID          string
	ProjectID   string
	Name        string
	Enabled     bool
	Priority    int
	MatchJSON   string
	ActionsJSON string
}

type AIMessageProject struct {
	MessageID  int64
	ProjectID  string
	Source     string
	Confidence float64
}

var projectSlugPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,63}$`)

func (db *DB) CreateAIProject(ctx context.Context, project AIProject) (AIProject, error) {
	project.Name = strings.TrimSpace(project.Name)
	project.Slug = strings.ToLower(strings.TrimSpace(project.Slug))
	if project.UserID == "" {
		return AIProject{}, fmt.Errorf("project requires a user")
	}
	if project.Name == "" || !projectSlugPattern.MatchString(project.Slug) {
		return AIProject{}, fmt.Errorf("project requires a name and a valid slug")
	}
	if project.ID == "" {
		project.ID = uuid.NewString()
	}
	if project.Status == "" {
		project.Status = "active"
	}
	classification, err := normalizeProjectClassification(project.DataClassification)
	if err != nil {
		return AIProject{}, err
	}
	llmDraft := true
	if project.AllowLLMDraft != nil {
		llmDraft = *project.AllowLLMDraft
	}
	_, err = db.Write().ExecContext(ctx, `
		INSERT INTO ai_projects (id, user_id, name, slug, description, instructions, color, status,
			allow_external_routing, allow_llm_draft, allow_auto_spam, data_classification)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		project.ID, project.UserID, project.Name, project.Slug, project.Description,
		project.Instructions, project.Color, project.Status,
		boolInt(project.AllowExternalRouting), boolInt(llmDraft), boolInt(project.AllowAutoSpam), classification)
	if err != nil {
		return AIProject{}, err
	}
	return db.GetAIProject(ctx, project.UserID, project.ID)
}

func (db *DB) GetAIProject(ctx context.Context, userID, id string) (AIProject, error) {
	var project AIProject
	var allowExternal, allowLLM, allowSpam int
	err := db.Read().QueryRowContext(ctx, `
		SELECT id, user_id, name, slug, description, instructions, color, status,
		       allow_external_routing, allow_llm_draft, allow_auto_spam, data_classification
		FROM ai_projects WHERE id = ? AND user_id = ?`, id, userID).Scan(
		&project.ID, &project.UserID, &project.Name, &project.Slug,
		&project.Description, &project.Instructions, &project.Color, &project.Status,
		&allowExternal, &allowLLM, &allowSpam, &project.DataClassification)
	project.AllowExternalRouting = allowExternal != 0
	llmDraft := allowLLM != 0
	project.AllowLLMDraft = &llmDraft
	project.AllowAutoSpam = allowSpam != 0
	return project, err
}

func (db *DB) ListAIProjects(ctx context.Context, userID string) ([]AIProject, error) {
	rows, err := db.Read().QueryContext(ctx, `
		SELECT id, user_id, name, slug, description, instructions, color, status,
		       allow_external_routing, allow_llm_draft, allow_auto_spam, data_classification
		FROM ai_projects WHERE user_id = ? ORDER BY name ASC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var projects []AIProject
	for rows.Next() {
		var project AIProject
		var allowExternal, allowLLM, allowSpam int
		if err := rows.Scan(&project.ID, &project.UserID, &project.Name, &project.Slug,
			&project.Description, &project.Instructions, &project.Color, &project.Status,
			&allowExternal, &allowLLM, &allowSpam, &project.DataClassification); err != nil {
			return nil, err
		}
		project.AllowExternalRouting = allowExternal != 0
		llmDraft := allowLLM != 0
		project.AllowLLMDraft = &llmDraft
		project.AllowAutoSpam = allowSpam != 0
		projects = append(projects, project)
	}
	return projects, rows.Err()
}

func (db *DB) UpdateAIProject(ctx context.Context, project AIProject) (AIProject, error) {
	project.Name = strings.TrimSpace(project.Name)
	project.Slug = strings.ToLower(strings.TrimSpace(project.Slug))
	if project.Name == "" || !projectSlugPattern.MatchString(project.Slug) {
		return AIProject{}, fmt.Errorf("project requires a name and a valid slug")
	}
	classification, err := normalizeProjectClassification(project.DataClassification)
	if err != nil {
		return AIProject{}, err
	}
	llmDraft := true
	if project.AllowLLMDraft != nil {
		llmDraft = *project.AllowLLMDraft
	}
	result, err := db.Write().ExecContext(ctx, `
		UPDATE ai_projects SET name = ?, slug = ?, description = ?, instructions = ?,
			color = ?, status = ?, allow_external_routing = ?, allow_llm_draft = ?,
			allow_auto_spam = ?, data_classification = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = ? AND user_id = ?`,
		project.Name, project.Slug, project.Description, project.Instructions,
		project.Color, project.Status, boolInt(project.AllowExternalRouting), boolInt(llmDraft),
		boolInt(project.AllowAutoSpam), classification, project.ID, project.UserID)
	if err != nil {
		return AIProject{}, err
	}
	if changed, _ := result.RowsAffected(); changed == 0 {
		return AIProject{}, sql.ErrNoRows
	}
	return db.GetAIProject(ctx, project.UserID, project.ID)
}

func (db *DB) DeleteAIProject(ctx context.Context, userID, id string) error {
	result, err := db.Write().ExecContext(ctx, `DELETE FROM ai_projects WHERE id = ? AND user_id = ?`, id, userID)
	if err != nil {
		return err
	}
	if changed, _ := result.RowsAffected(); changed == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func normalizeProjectClassification(value string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "":
		return "internal", nil
	case "public", "internal", "confidential", "restricted":
		return strings.ToLower(strings.TrimSpace(value)), nil
	default:
		return "", fmt.Errorf("invalid project data classification")
	}
}

func (db *DB) EnsureDefaultAIProjects(ctx context.Context, userID string) error {
	defaults := []AIProject{
		{Name: "Employment", Slug: "employment", Description: "Recruiters, applications, interviews and work contracts.", Color: "#2563eb"},
		{Name: "Legal", Slug: "legal", Description: "Lawyers, contracts, formal notices and compliance.", Color: "#7c3aed"},
		{Name: "Personal", Slug: "personal", Description: "Family, administration, health and hobbies.", Color: "#059669"},
	}
	for _, project := range defaults {
		project.UserID = userID
		if _, err := db.CreateAIProject(ctx, project); err != nil && !isUniqueConstraint(err) {
			return err
		}
	}
	return nil
}

func (db *DB) CreateAIProjectRoute(ctx context.Context, userID string, route AIProjectRoute) (AIProjectRoute, error) {
	if _, err := db.GetAIProject(ctx, userID, route.ProjectID); err != nil {
		return AIProjectRoute{}, err
	}
	if route.Name == "" {
		return AIProjectRoute{}, fmt.Errorf("route requires a name")
	}
	if route.MatchJSON == "" {
		route.MatchJSON = "{}"
	}
	if route.ActionsJSON == "" {
		route.ActionsJSON = "[]"
	}
	if !json.Valid([]byte(route.MatchJSON)) || !json.Valid([]byte(route.ActionsJSON)) {
		return AIProjectRoute{}, fmt.Errorf("route match and actions must be valid JSON")
	}
	if route.ID == "" {
		route.ID = uuid.NewString()
	}
	if route.Priority == 0 {
		route.Priority = 30
	}
	_, err := db.Write().ExecContext(ctx, `
		INSERT INTO ai_project_routes (id, project_id, name, enabled, priority, match_json, actions_json)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		route.ID, route.ProjectID, route.Name, route.Enabled, route.Priority, route.MatchJSON, route.ActionsJSON)
	if err != nil {
		return AIProjectRoute{}, err
	}
	return route, nil
}

func (db *DB) ListAIProjectRoutes(ctx context.Context, userID string, projectID string) ([]AIProjectRoute, error) {
	query := `
		SELECT r.id, r.project_id, r.name, r.enabled, r.priority, r.match_json, r.actions_json
		FROM ai_project_routes r
		JOIN ai_projects p ON p.id = r.project_id
		WHERE p.user_id = ?`
	args := []any{userID}
	if projectID != "" {
		query += ` AND r.project_id = ?`
		args = append(args, projectID)
	}
	query += ` ORDER BY r.priority ASC, r.created_at ASC`
	rows, err := db.Read().QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var routes []AIProjectRoute
	for rows.Next() {
		var route AIProjectRoute
		if err := rows.Scan(&route.ID, &route.ProjectID, &route.Name, &route.Enabled,
			&route.Priority, &route.MatchJSON, &route.ActionsJSON); err != nil {
			return nil, err
		}
		routes = append(routes, route)
	}
	return routes, rows.Err()
}

func (db *DB) UpdateAIProjectRoute(ctx context.Context, userID string, route AIProjectRoute) (AIProjectRoute, error) {
	if route.Name == "" {
		return AIProjectRoute{}, fmt.Errorf("route requires a name")
	}
	if !json.Valid([]byte(route.MatchJSON)) || !json.Valid([]byte(route.ActionsJSON)) {
		return AIProjectRoute{}, fmt.Errorf("route match and actions must be valid JSON")
	}
	result, err := db.Write().ExecContext(ctx, `
		UPDATE ai_project_routes SET name = ?, enabled = ?, priority = ?, match_json = ?,
			actions_json = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = ? AND project_id IN (SELECT id FROM ai_projects WHERE user_id = ?)`,
		route.Name, route.Enabled, route.Priority, route.MatchJSON, route.ActionsJSON, route.ID, userID)
	if err != nil {
		return AIProjectRoute{}, err
	}
	if changed, _ := result.RowsAffected(); changed == 0 {
		return AIProjectRoute{}, sql.ErrNoRows
	}
	return route, nil
}

func (db *DB) DeleteAIProjectRoute(ctx context.Context, userID, id string) error {
	result, err := db.Write().ExecContext(ctx, `
		DELETE FROM ai_project_routes
		WHERE id = ? AND project_id IN (SELECT id FROM ai_projects WHERE user_id = ?)`, id, userID)
	if err != nil {
		return err
	}
	if changed, _ := result.RowsAffected(); changed == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (db *DB) LinkAIMessageProject(ctx context.Context, userID string, link AIMessageProject) error {
	if link.Source == "" {
		link.Source = "rule"
	}
	if link.Source != "jev" && link.Source != "rule" && link.Source != "manual" {
		return fmt.Errorf("invalid project link source")
	}
	result, err := db.Write().ExecContext(ctx, `
		INSERT INTO ai_message_projects (message_id, project_id, source, confidence)
		SELECT m.id, p.id, ?, ?
		FROM messages m
		JOIN accounts a ON a.id = m.account_id
		JOIN ai_projects p ON p.id = ?
		WHERE m.id = ? AND a.user_id = ? AND p.user_id = ?
		ON CONFLICT(message_id, project_id) DO UPDATE SET
			source = excluded.source, confidence = excluded.confidence`,
		link.Source, link.Confidence, link.ProjectID, link.MessageID, userID, userID)
	if err != nil {
		return err
	}
	if changed, _ := result.RowsAffected(); changed == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (db *DB) ListAIMessageProjects(ctx context.Context, userID string, messageID int64) ([]AIMessageProject, error) {
	rows, err := db.Read().QueryContext(ctx, `
		SELECT mp.message_id, mp.project_id, mp.source, mp.confidence
		FROM ai_message_projects mp
		JOIN messages m ON m.id = mp.message_id
		JOIN accounts a ON a.id = m.account_id
		WHERE mp.message_id = ? AND a.user_id = ?
		ORDER BY mp.created_at ASC`, messageID, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var links []AIMessageProject
	for rows.Next() {
		var link AIMessageProject
		if err := rows.Scan(&link.MessageID, &link.ProjectID, &link.Source, &link.Confidence); err != nil {
			return nil, err
		}
		links = append(links, link)
	}
	return links, rows.Err()
}

func isUniqueConstraint(err error) bool {
	return err != nil && strings.Contains(strings.ToLower(err.Error()), "unique")
}
