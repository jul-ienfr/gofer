package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	CurrentAnalysisVersion = 1

	AnalysisStateQueued  = "queued"
	AnalysisStateRunning = "running"
	AnalysisStateDone    = "done"
	AnalysisStateFailed  = "failed"
	AnalysisStateDead    = "dead"
	AnalysisStateSkipped = "skipped"

	analysisMaxAttempts = 5
	analysisPriority    = 30
)

var (
	ErrAnalysisJobLeaseLost     = errors.New("analysis job lease is no longer owned by this worker")
	ErrAnalysisDisabled         = errors.New("AI analysis is disabled")
	ErrAnalysisFolderNotAllowed = errors.New("AI analysis is not allowed for this folder")
)

type AnalysisJob struct {
	ID                string
	UserID            string
	AccountID         string
	MessageID         int64
	InternetMessageID string
	FolderID          string
	AnalysisVersion   int
	Priority          int
	Attempts          int
}

func analysisEnabledTx(ctx context.Context, tx *sql.Tx, accountID string) (bool, error) {
	var enabled int
	err := tx.QueryRowContext(ctx, `
		SELECT COALESCE((
			SELECT CASE WHEN lower(s.value) IN ('1', 'true', 'yes', 'on') THEN 1 ELSE 0 END
			FROM app_settings s
			JOIN accounts a ON a.user_id = s.user_id
			WHERE a.id = ? AND s.key = 'ai_analysis_enabled'
		), 0)`, accountID).Scan(&enabled)
	if err != nil {
		return false, err
	}
	return enabled == 1, nil
}

func analysisFolderAllowedTx(ctx context.Context, tx *sql.Tx, folderID string) (bool, error) {
	if folderID == "" {
		return true, nil
	}
	var role string
	err := tx.QueryRowContext(ctx, `SELECT COALESCE(role, '') FROM folders WHERE id = ?`, folderID).Scan(&role)
	if errors.Is(err, sql.ErrNoRows) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	switch strings.ToLower(role) {
	case "drafts", "draft", "sent", "trash":
		return false, nil
	default:
		return true, nil
	}
}

func enqueueAnalysisTx(ctx context.Context, tx *sql.Tx, messageID int64, accountID, folderID, internetMessageID string, analysisVersion int) error {
	if messageID == 0 || accountID == "" {
		return nil
	}
	if analysisVersion <= 0 {
		analysisVersion = CurrentAnalysisVersion
	}
	enabled, err := analysisEnabledTx(ctx, tx, accountID)
	if err != nil {
		return fmt.Errorf("check analysis setting: %w", err)
	}
	if !enabled {
		return nil
	}
	allowed, err := analysisFolderAllowedTx(ctx, tx, folderID)
	if err != nil {
		return fmt.Errorf("check analysis folder: %w", err)
	}
	if !allowed {
		return nil
	}
	_, err = tx.ExecContext(ctx, `
		INSERT OR IGNORE INTO ai_analysis_jobs
			(id, account_id, message_id, internet_message_id, folder_id, analysis_version, state, priority, next_attempt_at)
		VALUES (?, ?, ?, ?, ?, ?, 'queued', ?, CURRENT_TIMESTAMP)`,
		uuid.NewString(), accountID, messageID, internetMessageID, folderID, analysisVersion, analysisPriority)
	if err != nil {
		return fmt.Errorf("enqueue analysis: %w", err)
	}
	return nil
}

// EnqueueAnalysisForUser queues or requeues one visible message for its owner.
// A job that is already queued or running is returned unchanged.
func (db *DB) EnqueueAnalysisForUser(ctx context.Context, userID string, messageID int64) (string, error) {
	if strings.TrimSpace(userID) == "" || messageID <= 0 {
		return "", sql.ErrNoRows
	}
	tx, err := db.Write().BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	var accountID, internetMessageID, folderID, folderRole string
	err = tx.QueryRowContext(ctx, `
		SELECT m.account_id, COALESCE(m.internet_message_id, ''), f.id, COALESCE(f.role, '')
		FROM messages m
		JOIN accounts a ON a.id = m.account_id
		JOIN message_folder_state ms ON ms.message_id = m.id AND ms.is_deleted = 0
		JOIN folders f ON f.id = ms.folder_id AND f.account_id = m.account_id
		WHERE m.id = ? AND a.user_id = ? AND COALESCE(a.is_deleting, 0) = 0
		ORDER BY f.id LIMIT 1`, messageID, userID).Scan(&accountID, &internetMessageID, &folderID, &folderRole)
	if err != nil {
		return "", err
	}
	switch strings.ToLower(folderRole) {
	case "drafts", "draft", "sent", "trash":
		return "", ErrAnalysisFolderNotAllowed
	}
	enabled, err := analysisEnabledTx(ctx, tx, accountID)
	if err != nil {
		return "", err
	}
	if !enabled {
		return "", ErrAnalysisDisabled
	}
	var jobID, state string
	err = tx.QueryRowContext(ctx, `
		SELECT id, state FROM ai_analysis_jobs
		WHERE message_id = ? AND analysis_version = ?`,
		messageID, CurrentAnalysisVersion).Scan(&jobID, &state)
	if errors.Is(err, sql.ErrNoRows) {
		jobID = uuid.NewString()
		_, err = tx.ExecContext(ctx, `
			INSERT INTO ai_analysis_jobs
				(id, account_id, message_id, internet_message_id, folder_id, analysis_version, state, priority, next_attempt_at)
			VALUES (?, ?, ?, ?, ?, ?, 'queued', ?, CURRENT_TIMESTAMP)`,
			jobID, accountID, messageID, internetMessageID, folderID, CurrentAnalysisVersion, analysisPriority)
		if err != nil {
			return "", fmt.Errorf("enqueue analysis: %w", err)
		}
	} else if err != nil {
		return "", err
	} else if state != AnalysisStateQueued && state != AnalysisStateRunning {
		_, err = tx.ExecContext(ctx, `
			UPDATE ai_analysis_jobs
			SET account_id = ?, internet_message_id = ?, folder_id = ?, state = 'queued',
				attempts = 0, priority = ?, next_attempt_at = CURRENT_TIMESTAMP,
				lease_owner = '', lease_expires_at = NULL, last_error = '',
				updated_at = CURRENT_TIMESTAMP
			WHERE id = ?`,
			accountID, internetMessageID, folderID, analysisPriority, jobID)
		if err != nil {
			return "", fmt.Errorf("requeue analysis: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return "", err
	}
	return jobID, nil
}

func (db *DB) ClaimAnalysisJob(ctx context.Context, workerID string, lease time.Duration) (*AnalysisJob, error) {
	if strings.TrimSpace(workerID) == "" {
		return nil, errors.New("analysis worker id is required")
	}
	if lease <= 0 {
		lease = 2 * time.Minute
	}
	leaseUntil := formatDBTime(time.Now().UTC().Add(lease))
	var job AnalysisJob
	err := db.Write().QueryRowContext(ctx, `
		UPDATE ai_analysis_jobs
		SET state = 'running',
			lease_owner = ?,
			lease_expires_at = ?,
			attempts = attempts + 1,
			updated_at = CURRENT_TIMESTAMP
		WHERE id = (
			SELECT id FROM ai_analysis_jobs
			WHERE state = 'queued' AND next_attempt_at <= CURRENT_TIMESTAMP
			ORDER BY priority ASC, created_at ASC
			LIMIT 1
		)
		RETURNING id, (SELECT user_id FROM accounts WHERE id = account_id),
			account_id, message_id, internet_message_id, folder_id, analysis_version, priority, attempts`,
		workerID, leaseUntil).Scan(&job.ID, &job.UserID, &job.AccountID, &job.MessageID, &job.InternetMessageID,
		&job.FolderID, &job.AnalysisVersion, &job.Priority, &job.Attempts)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("claim analysis job: %w", err)
	}
	return &job, nil
}

func (db *DB) RecoverExpiredAnalysisJobs(ctx context.Context) (int64, error) {
	result, err := db.Write().ExecContext(ctx, `
		UPDATE ai_analysis_jobs
		SET state = CASE WHEN attempts + 1 >= ? THEN 'dead' ELSE 'queued' END,
			attempts = attempts + 1,
			lease_owner = '',
			lease_expires_at = NULL,
			next_attempt_at = CURRENT_TIMESTAMP,
			updated_at = CURRENT_TIMESTAMP
		WHERE state = 'running'
		  AND lease_expires_at IS NOT NULL
		  AND lease_expires_at <= CURRENT_TIMESTAMP`,
		analysisMaxAttempts)
	if err != nil {
		return 0, fmt.Errorf("recover expired analysis jobs: %w", err)
	}
	return result.RowsAffected()
}

func (db *DB) CompleteAnalysisJob(ctx context.Context, jobID, workerID string) error {
	result, err := db.Write().ExecContext(ctx, `
		UPDATE ai_analysis_jobs
		SET state = 'done',
			lease_owner = '',
			lease_expires_at = NULL,
			last_error = '',
			updated_at = CURRENT_TIMESTAMP
		WHERE id = ? AND state = 'running' AND lease_owner = ?`,
		jobID, workerID)
	if err != nil {
		return fmt.Errorf("complete analysis job: %w", err)
	}
	return requireAnalysisLease(result)
}

func (db *DB) RetryAnalysisJob(ctx context.Context, jobID, workerID, lastError string, nextAttempt time.Time) error {
	if nextAttempt.IsZero() {
		nextAttempt = time.Now().UTC()
	}
	result, err := db.Write().ExecContext(ctx, `
		UPDATE ai_analysis_jobs
		SET state = CASE WHEN attempts >= ? THEN 'dead' ELSE 'queued' END,
			next_attempt_at = ?,
			lease_owner = '',
			lease_expires_at = NULL,
			last_error = ?,
			updated_at = CURRENT_TIMESTAMP
		WHERE id = ? AND state = 'running' AND lease_owner = ?`,
		analysisMaxAttempts, formatDBTime(nextAttempt), redactAnalysisError(lastError), jobID, workerID)
	if err != nil {
		return fmt.Errorf("retry analysis job: %w", err)
	}
	return requireAnalysisLease(result)
}

func (db *DB) FailAnalysisJob(ctx context.Context, jobID, workerID, lastError string) error {
	result, err := db.Write().ExecContext(ctx, `
		UPDATE ai_analysis_jobs
		SET state = 'failed',
			lease_owner = '',
			lease_expires_at = NULL,
			last_error = ?,
			updated_at = CURRENT_TIMESTAMP
		WHERE id = ? AND state = 'running' AND lease_owner = ?`,
		redactAnalysisError(lastError), jobID, workerID)
	if err != nil {
		return fmt.Errorf("fail analysis job: %w", err)
	}
	return requireAnalysisLease(result)
}

func (db *DB) SkipAnalysisJob(ctx context.Context, jobID, workerID, reason string) error {
	result, err := db.Write().ExecContext(ctx, `
		UPDATE ai_analysis_jobs
		SET state = 'skipped',
			lease_owner = '',
			lease_expires_at = NULL,
			last_error = ?,
			updated_at = CURRENT_TIMESTAMP
		WHERE id = ? AND state = 'running' AND lease_owner = ?`,
		redactAnalysisError(reason), jobID, workerID)
	if err != nil {
		return fmt.Errorf("skip analysis job: %w", err)
	}
	return requireAnalysisLease(result)
}

func requireAnalysisLease(result sql.Result) error {
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return ErrAnalysisJobLeaseLost
	}
	return nil
}

func redactAnalysisError(value string) string {
	value = strings.TrimSpace(strings.ReplaceAll(value, "\n", " "))
	if len(value) > 500 {
		value = value[:500]
	}
	return value
}
