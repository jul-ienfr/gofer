package storage

import (
	"database/sql"
	"fmt"
)

const aiAnalysisJobsTableSQL = `CREATE TABLE IF NOT EXISTS ai_analysis_jobs (
    id TEXT PRIMARY KEY,
    account_id TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    message_id INTEGER NOT NULL REFERENCES messages(id) ON DELETE CASCADE,
    internet_message_id TEXT NOT NULL DEFAULT '',
    folder_id TEXT NOT NULL DEFAULT '',
    analysis_version INTEGER NOT NULL DEFAULT 1,
    state TEXT NOT NULL DEFAULT 'queued' CHECK (state IN ('queued', 'running', 'done', 'failed', 'dead', 'skipped')),
    priority INTEGER NOT NULL DEFAULT 30,
    attempts INTEGER NOT NULL DEFAULT 0,
    next_attempt_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    lease_owner TEXT NOT NULL DEFAULT '',
    lease_expires_at DATETIME,
    last_error TEXT NOT NULL DEFAULT '',
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(message_id, analysis_version)
);
CREATE INDEX IF NOT EXISTS idx_ai_analysis_jobs_claim ON ai_analysis_jobs(state, next_attempt_at, priority);
CREATE INDEX IF NOT EXISTS idx_ai_analysis_jobs_lease ON ai_analysis_jobs(lease_expires_at);
CREATE INDEX IF NOT EXISTS idx_ai_analysis_jobs_message ON ai_analysis_jobs(message_id);`

const aiAnalysisResultsTableSQL = `CREATE TABLE IF NOT EXISTS ai_analysis_results (
    message_id INTEGER PRIMARY KEY REFERENCES messages(id) ON DELETE CASCADE,
    account_id TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    analysis_version INTEGER NOT NULL DEFAULT 1,
    model TEXT NOT NULL DEFAULT '',
    priority_label TEXT NOT NULL DEFAULT '',
    priority_confidence REAL NOT NULL DEFAULT 0,
    risk_label TEXT NOT NULL DEFAULT '',
    risk_confidence REAL NOT NULL DEFAULT 0,
    intent_label TEXT NOT NULL DEFAULT '',
    intent_confidence REAL NOT NULL DEFAULT 0,
    project_label TEXT NOT NULL DEFAULT '',
    project_confidence REAL NOT NULL DEFAULT 0,
    summary TEXT NOT NULL DEFAULT '',
    usage_input_tokens INTEGER NOT NULL DEFAULT 0,
    usage_output_tokens INTEGER NOT NULL DEFAULT 0,
    usage_cost_micros INTEGER NOT NULL DEFAULT 0,
    latency_ms INTEGER NOT NULL DEFAULT 0,
    prompt_version TEXT NOT NULL DEFAULT '',
    policy_version TEXT NOT NULL DEFAULT '',
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_ai_analysis_results_account ON ai_analysis_results(account_id, created_at);`

const aiAnalysisAnswersTableSQL = `CREATE TABLE IF NOT EXISTS ai_analysis_answers (
    message_id INTEGER NOT NULL REFERENCES messages(id) ON DELETE CASCADE,
    question_key TEXT NOT NULL,
    type TEXT NOT NULL DEFAULT '',
    choice TEXT NOT NULL DEFAULT '',
    score REAL NOT NULL DEFAULT 0,
    noul REAL NOT NULL DEFAULT 0,
    confidence REAL NOT NULL DEFAULT 0,
    probabilities_json TEXT NOT NULL DEFAULT '{}',
    PRIMARY KEY (message_id, question_key)
);
CREATE INDEX IF NOT EXISTS idx_ai_analysis_answers_message ON ai_analysis_answers(message_id);`

const aiLegitimacyProfilesTableSQL = `CREATE TABLE IF NOT EXISTS ai_legitimacy_profiles (
    id TEXT PRIMARY KEY,
    account_id TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    sender_email TEXT NOT NULL,
    sender_domain TEXT NOT NULL DEFAULT '',
    first_seen DATETIME,
    last_seen DATETIME,
    message_count INTEGER NOT NULL DEFAULT 0,
    reply_count INTEGER NOT NULL DEFAULT 0,
    sent_count INTEGER NOT NULL DEFAULT 0,
    spam_count INTEGER NOT NULL DEFAULT 0,
    ham_count INTEGER NOT NULL DEFAULT 0,
    user_correction_count INTEGER NOT NULL DEFAULT 0,
    auth_pass_count INTEGER NOT NULL DEFAULT 0,
    auth_fail_count INTEGER NOT NULL DEFAULT 0,
    vip INTEGER NOT NULL DEFAULT 0,
    allowlisted INTEGER NOT NULL DEFAULT 0,
    blocked INTEGER NOT NULL DEFAULT 0,
    score REAL NOT NULL DEFAULT 0,
    state TEXT NOT NULL DEFAULT 'unknown',
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(account_id, sender_email)
);
CREATE INDEX IF NOT EXISTS idx_ai_legitimacy_profiles_domain ON ai_legitimacy_profiles(account_id, sender_domain);`

const aiProjectsTableSQL = `CREATE TABLE IF NOT EXISTS ai_projects (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name TEXT NOT NULL CHECK (name != ''),
    slug TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    instructions TEXT NOT NULL DEFAULT '',
    color TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'active',
    allow_external_routing INTEGER NOT NULL DEFAULT 0 CHECK (allow_external_routing IN (0, 1)),
    allow_llm_draft INTEGER NOT NULL DEFAULT 1 CHECK (allow_llm_draft IN (0, 1)),
    allow_auto_spam INTEGER NOT NULL DEFAULT 0 CHECK (allow_auto_spam IN (0, 1)),
    data_classification TEXT NOT NULL DEFAULT 'internal',
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(user_id, slug)
);`

const aiProjectRoutesTableSQL = `CREATE TABLE IF NOT EXISTS ai_project_routes (
    id TEXT PRIMARY KEY,
    project_id TEXT NOT NULL REFERENCES ai_projects(id) ON DELETE CASCADE,
    name TEXT NOT NULL DEFAULT '',
    enabled INTEGER NOT NULL DEFAULT 1,
    priority INTEGER NOT NULL DEFAULT 30,
    match_json TEXT NOT NULL DEFAULT '{}',
    actions_json TEXT NOT NULL DEFAULT '[]',
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_ai_project_routes_project ON ai_project_routes(project_id, priority);`

const aiMessageProjectsTableSQL = `CREATE TABLE IF NOT EXISTS ai_message_projects (
    message_id INTEGER NOT NULL REFERENCES messages(id) ON DELETE CASCADE,
    project_id TEXT NOT NULL REFERENCES ai_projects(id) ON DELETE CASCADE,
    source TEXT NOT NULL DEFAULT 'jev' CHECK (source IN ('jev', 'rule', 'manual')),
    confidence REAL NOT NULL DEFAULT 0,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (message_id, project_id)
);
CREATE INDEX IF NOT EXISTS idx_ai_message_projects_project ON ai_message_projects(project_id, created_at);`

const aiConnectorsTableSQL = `CREATE TABLE IF NOT EXISTS ai_connectors (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name TEXT NOT NULL DEFAULT '',
    type TEXT NOT NULL DEFAULT 'webhook' CHECK (type IN ('webhook', 'http', 'n8n', 'make', 'zapier')),
    url TEXT NOT NULL DEFAULT '',
    secret_encrypted TEXT NOT NULL DEFAULT '',
    enabled INTEGER NOT NULL DEFAULT 1,
    allow_private INTEGER NOT NULL DEFAULT 0,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);`

const aiRoutingOutboxTableSQL = `CREATE TABLE IF NOT EXISTS ai_routing_outbox (
    id TEXT PRIMARY KEY,
    message_id INTEGER NOT NULL REFERENCES messages(id) ON DELETE CASCADE,
    project_id TEXT REFERENCES ai_projects(id) ON DELETE SET NULL,
    connector_id TEXT REFERENCES ai_connectors(id) ON DELETE SET NULL,
    event_type TEXT NOT NULL DEFAULT '',
    payload_json TEXT NOT NULL DEFAULT '{}',
    idempotency_key TEXT NOT NULL UNIQUE,
    state TEXT NOT NULL DEFAULT 'queued' CHECK (state IN ('queued', 'running', 'done', 'failed', 'dead', 'skipped')),
    attempts INTEGER NOT NULL DEFAULT 0,
    next_attempt_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    lease_owner TEXT NOT NULL DEFAULT '',
    lease_expires_at DATETIME,
    response_status INTEGER NOT NULL DEFAULT 0,
    last_error TEXT NOT NULL DEFAULT '',
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_ai_routing_outbox_claim ON ai_routing_outbox(state, next_attempt_at, created_at);`

const aiLLMProvidersTableSQL = `CREATE TABLE IF NOT EXISTS ai_llm_providers (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name TEXT NOT NULL DEFAULT '',
    base_url TEXT NOT NULL DEFAULT '',
    model TEXT NOT NULL DEFAULT '',
    api_key_encrypted TEXT NOT NULL DEFAULT '',
    temperature REAL NOT NULL DEFAULT 0.2,
    max_tokens INTEGER NOT NULL DEFAULT 0,
    timeout_ms INTEGER NOT NULL DEFAULT 30000,
    enabled INTEGER NOT NULL DEFAULT 0,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);`

const aiLLMGenerationsTableSQL = `CREATE TABLE IF NOT EXISTS ai_llm_generations (
    id TEXT PRIMARY KEY,
    message_id INTEGER NOT NULL REFERENCES messages(id) ON DELETE CASCADE,
    provider_id TEXT REFERENCES ai_llm_providers(id) ON DELETE SET NULL,
    purpose TEXT NOT NULL DEFAULT '',
    prompt_version TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT '',
    draft_message_id INTEGER,
    input_tokens INTEGER NOT NULL DEFAULT 0,
    output_tokens INTEGER NOT NULL DEFAULT 0,
    cost_micros INTEGER NOT NULL DEFAULT 0,
    latency_ms INTEGER NOT NULL DEFAULT 0,
    error TEXT NOT NULL DEFAULT '',
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_ai_llm_generations_message ON ai_llm_generations(message_id, created_at);`

const aiAutomationActionsTableSQL = `CREATE TABLE IF NOT EXISTS ai_automation_actions (
    id TEXT PRIMARY KEY,
    message_id INTEGER NOT NULL REFERENCES messages(id) ON DELETE CASCADE,
    kind TEXT NOT NULL DEFAULT '',
    mode TEXT NOT NULL DEFAULT '',
    state TEXT NOT NULL DEFAULT '',
    gate_json TEXT NOT NULL DEFAULT '{}',
    confidence REAL NOT NULL DEFAULT 0,
    reason TEXT NOT NULL DEFAULT '',
    executed_at DATETIME,
    undo_deadline DATETIME,
    undone_at DATETIME,
    result TEXT NOT NULL DEFAULT '',
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_ai_automation_actions_message ON ai_automation_actions(message_id, created_at);`

const aiUnsubscribeAttemptsTableSQL = `CREATE TABLE IF NOT EXISTS ai_unsubscribe_attempts (
    id TEXT PRIMARY KEY,
    message_id INTEGER NOT NULL REFERENCES messages(id) ON DELETE CASCADE,
    list_unsubscribe_hash TEXT NOT NULL DEFAULT '',
    method TEXT NOT NULL DEFAULT '',
    target_hash TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT '',
    http_status INTEGER NOT NULL DEFAULT 0,
    redirect_count INTEGER NOT NULL DEFAULT 0,
    error TEXT NOT NULL DEFAULT '',
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_ai_unsubscribe_attempts_message ON ai_unsubscribe_attempts(message_id, created_at);`

const aiAPITokensTableSQL = `CREATE TABLE IF NOT EXISTS ai_api_tokens (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name TEXT NOT NULL DEFAULT '',
    token_hash TEXT NOT NULL UNIQUE,
    scopes TEXT NOT NULL DEFAULT '',
    expires_at DATETIME,
    last_used_at DATETIME,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    revoked_at DATETIME
);
CREATE INDEX IF NOT EXISTS idx_ai_api_tokens_user ON ai_api_tokens(user_id, created_at);`

const aiAuditEventsTableSQL = `CREATE TABLE IF NOT EXISTS ai_audit_events (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    actor_type TEXT NOT NULL DEFAULT 'system' CHECK (actor_type IN ('system', 'user', 'api')),
    actor_id TEXT NOT NULL DEFAULT '',
    action TEXT NOT NULL DEFAULT '',
    subject_type TEXT NOT NULL DEFAULT '',
    subject_id TEXT NOT NULL DEFAULT '',
    before_json TEXT NOT NULL DEFAULT '{}',
    after_json TEXT NOT NULL DEFAULT '{}',
    ip TEXT NOT NULL DEFAULT '',
    user_agent TEXT NOT NULL DEFAULT '',
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_ai_audit_events_user ON ai_audit_events(user_id, created_at);`

const aiPromptVersionsTableSQL = `CREATE TABLE IF NOT EXISTS ai_prompt_versions (
    id TEXT PRIMARY KEY,
    key TEXT NOT NULL,
    version TEXT NOT NULL,
    locale TEXT NOT NULL DEFAULT '',
    body TEXT NOT NULL DEFAULT '',
    active INTEGER NOT NULL DEFAULT 0,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(key, version, locale)
);
CREATE INDEX IF NOT EXISTS idx_ai_prompt_versions_active ON ai_prompt_versions(key, locale, active);`

func migrateAISchema(db *sql.DB, version int, statements ...string) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var current int
	if err := tx.QueryRow(`SELECT COALESCE(MAX(version), 0) FROM schema_version`).Scan(&current); err != nil {
		return err
	}
	if current >= version {
		return nil
	}
	for _, statement := range statements {
		if _, err := tx.Exec(statement); err != nil {
			return fmt.Errorf("apply schema %d: %w", version, err)
		}
	}
	if err := markSchemaVersion(tx, version); err != nil {
		return err
	}
	return tx.Commit()
}

func applyAIMigrations(db *sql.DB, currentVersion int) error {
	if currentVersion >= 145 {
		return nil
	}
	for _, migrate := range []func(*sql.DB) error{
		migrateV135ToV136,
		migrateV136ToV137,
		migrateV137ToV138,
		migrateV138ToV139,
		migrateV139ToV140,
		migrateV140ToV141,
		migrateV141ToV142,
		migrateV142ToV143,
		migrateV143ToV144,
		migrateV144ToV145,
	} {
		if err := migrate(db); err != nil {
			return err
		}
	}
	return nil
}

func migrateV135ToV136(db *sql.DB) error {
	return migrateAISchema(db, 136, aiAnalysisJobsTableSQL)
}

func migrateV136ToV137(db *sql.DB) error {
	return migrateAISchema(db, 137, aiAnalysisResultsTableSQL, aiAnalysisAnswersTableSQL)
}

func migrateV137ToV138(db *sql.DB) error {
	return migrateAISchema(db, 138, aiLegitimacyProfilesTableSQL)
}

func migrateV138ToV139(db *sql.DB) error {
	return migrateAISchema(db, 139, aiProjectsTableSQL, aiProjectRoutesTableSQL, aiMessageProjectsTableSQL)
}

func migrateV139ToV140(db *sql.DB) error {
	return migrateAISchema(db, 140, aiConnectorsTableSQL, aiRoutingOutboxTableSQL)
}

func migrateV140ToV141(db *sql.DB) error {
	return migrateAISchema(db, 141, aiLLMProvidersTableSQL, aiLLMGenerationsTableSQL)
}

func migrateV141ToV142(db *sql.DB) error {
	return migrateAISchema(db, 142, aiAutomationActionsTableSQL, aiUnsubscribeAttemptsTableSQL)
}

func migrateV142ToV143(db *sql.DB) error {
	return migrateAISchema(db, 143, aiAPITokensTableSQL, aiAuditEventsTableSQL)
}

func migrateV143ToV144(db *sql.DB) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var current int
	if err := tx.QueryRow(`SELECT COALESCE(MAX(version), 0) FROM schema_version`).Scan(&current); err != nil {
		return err
	}
	if current >= 144 {
		return nil
	}
	if _, err := tx.Exec(aiPromptVersionsTableSQL); err != nil {
		return fmt.Errorf("apply schema 144: %w", err)
	}
	appSettings, err := tableExistsTx(tx, "app_settings")
	if err != nil {
		return err
	}
	users, err := tableExistsTx(tx, "users")
	if err != nil {
		return err
	}
	if appSettings && users {
		managedLayout, err := tableExistsTx(tx, "gofer_storage_layout")
		if err != nil {
			return err
		}
		userFilter := ""
		if managedLayout {
			hasUserType, err := columnExistsTx(tx, "users", "user_type")
			if err != nil {
				return err
			}
			if !hasUserType {
				managedLayout = false
			} else {
				userFilter = ` WHERE user_type = 'management'`
			}
		}
		if managedLayout || userFilter == "" {
			if _, err := tx.Exec(`
				INSERT OR IGNORE INTO app_settings (user_id, key, value)
				SELECT id, 'ai_analysis_enabled', '0' FROM users` + userFilter + `;
				INSERT OR IGNORE INTO app_settings (user_id, key, value)
				SELECT id, 'ai_drafting_enabled', '0' FROM users` + userFilter + `;
				INSERT OR IGNORE INTO app_settings (user_id, key, value)
				SELECT id, 'ai_auto_spam_enabled', '0' FROM users` + userFilter + `;
				INSERT OR IGNORE INTO app_settings (user_id, key, value)
				SELECT id, 'ai_auto_unsubscribe_enabled', '0' FROM users` + userFilter + `;`); err != nil {
				return fmt.Errorf("seed AI settings: %w", err)
			}
		}
	}
	if err := markSchemaVersion(tx, 144); err != nil {
		return err
	}
	return tx.Commit()
}

func migrateV144ToV145(db *sql.DB) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var current int
	if err := tx.QueryRow(`SELECT COALESCE(MAX(version), 0) FROM schema_version`).Scan(&current); err != nil {
		return err
	}
	if current >= 145 {
		return nil
	}
	columns := []struct {
		name string
		sql  string
	}{
		{"allow_external_routing", `ALTER TABLE ai_projects ADD COLUMN allow_external_routing INTEGER NOT NULL DEFAULT 0 CHECK (allow_external_routing IN (0, 1))`},
		{"allow_llm_draft", `ALTER TABLE ai_projects ADD COLUMN allow_llm_draft INTEGER NOT NULL DEFAULT 1 CHECK (allow_llm_draft IN (0, 1))`},
		{"allow_auto_spam", `ALTER TABLE ai_projects ADD COLUMN allow_auto_spam INTEGER NOT NULL DEFAULT 0 CHECK (allow_auto_spam IN (0, 1))`},
		{"data_classification", `ALTER TABLE ai_projects ADD COLUMN data_classification TEXT NOT NULL DEFAULT 'internal'`},
	}
	for _, column := range columns {
		exists, err := columnExistsTx(tx, "ai_projects", column.name)
		if err != nil {
			return err
		}
		if exists {
			continue
		}
		if _, err := tx.Exec(column.sql); err != nil {
			return fmt.Errorf("apply schema 145 (%s): %w", column.name, err)
		}
	}
	if err := markSchemaVersion(tx, 145); err != nil {
		return err
	}
	return tx.Commit()
}
