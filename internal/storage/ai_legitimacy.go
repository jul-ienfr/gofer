package storage

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

type AILegitimacyProfile struct {
	ID                  string
	AccountID           string
	SenderEmail         string
	SenderDomain        string
	FirstSeen           time.Time
	LastSeen            time.Time
	MessageCount        int
	ReplyCount          int
	SentCount           int
	SpamCount           int
	HamCount            int
	UserCorrectionCount int
	AuthPassCount       int
	AuthFailCount       int
	VIP                 bool
	Allowlisted         bool
	Blocked             bool
	Score               float64
	State               string
	UpdatedAt           time.Time
}

func (db *DB) GetAILegitimacyProfile(ctx context.Context, accountID, senderEmail string) (AILegitimacyProfile, error) {
	var profile AILegitimacyProfile
	var firstSeen, lastSeen sql.NullString
	var updatedAt sql.NullString
	err := db.Read().QueryRowContext(ctx, `
		SELECT id, account_id, sender_email, sender_domain, first_seen, last_seen,
		       message_count, reply_count, sent_count, spam_count, ham_count,
		       user_correction_count, auth_pass_count, auth_fail_count,
		       vip, allowlisted, blocked, score, state, updated_at
		FROM ai_legitimacy_profiles
		WHERE account_id = ? AND sender_email = ?`,
		accountID, normalizeLegitimacyEmail(senderEmail)).Scan(
		&profile.ID, &profile.AccountID, &profile.SenderEmail, &profile.SenderDomain,
		&firstSeen, &lastSeen, &profile.MessageCount, &profile.ReplyCount,
		&profile.SentCount, &profile.SpamCount, &profile.HamCount,
		&profile.UserCorrectionCount, &profile.AuthPassCount, &profile.AuthFailCount,
		&profile.VIP, &profile.Allowlisted, &profile.Blocked, &profile.Score,
		&profile.State, &updatedAt,
	)
	if err != nil {
		return AILegitimacyProfile{}, err
	}
	profile.FirstSeen = parseLegitimacyTime(firstSeen)
	profile.LastSeen = parseLegitimacyTime(lastSeen)
	profile.UpdatedAt = parseLegitimacyTime(updatedAt)
	return profile, nil
}

// GetAILegitimacyProfileForMessage returns the sender profile for a message
// only when the message belongs to the requested user.
func (db *DB) GetAILegitimacyProfileForMessage(ctx context.Context, userID string, messageID int64) (AILegitimacyProfile, string, error) {
	var accountID, senderEmail string
	if err := db.Read().QueryRowContext(ctx, `
		SELECT m.account_id, COALESCE(m.from_email, '')
		FROM messages m
		JOIN accounts a ON a.id = m.account_id
		WHERE m.id = ? AND a.user_id = ?`, messageID, userID).Scan(&accountID, &senderEmail); err != nil {
		return AILegitimacyProfile{}, "", err
	}
	if strings.TrimSpace(senderEmail) == "" {
		return AILegitimacyProfile{}, senderEmail, sql.ErrNoRows
	}
	profile, err := db.GetAILegitimacyProfile(ctx, accountID, senderEmail)
	if err != nil {
		return AILegitimacyProfile{}, senderEmail, err
	}
	return profile, senderEmail, nil
}

func (db *DB) SaveAILegitimacyProfile(ctx context.Context, profile AILegitimacyProfile) (AILegitimacyProfile, error) {
	profile.SenderEmail = normalizeLegitimacyEmail(profile.SenderEmail)
	if profile.AccountID == "" || profile.SenderEmail == "" {
		return AILegitimacyProfile{}, fmt.Errorf("legitimacy profile requires account and sender")
	}
	if profile.ID == "" {
		profile.ID = uuid.NewString()
	}
	if profile.SenderDomain == "" {
		if at := lastIndexByte(profile.SenderEmail, '@'); at >= 0 {
			profile.SenderDomain = profile.SenderEmail[at+1:]
		}
	}
	profile.State = string(normalizeLegitimacyState(profile.State))
	now := time.Now().UTC()
	if profile.FirstSeen.IsZero() {
		profile.FirstSeen = now
	}
	if profile.LastSeen.IsZero() {
		profile.LastSeen = now
	}
	_, err := db.Write().ExecContext(ctx, `
		INSERT INTO ai_legitimacy_profiles (
			id, account_id, sender_email, sender_domain, first_seen, last_seen,
			message_count, reply_count, sent_count, spam_count, ham_count,
			user_correction_count, auth_pass_count, auth_fail_count,
			vip, allowlisted, blocked, score, state, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(account_id, sender_email) DO UPDATE SET
			sender_domain = excluded.sender_domain,
			first_seen = CASE WHEN ai_legitimacy_profiles.first_seen IS NULL THEN excluded.first_seen ELSE ai_legitimacy_profiles.first_seen END,
			last_seen = excluded.last_seen,
			message_count = excluded.message_count,
			reply_count = excluded.reply_count,
			sent_count = excluded.sent_count,
			spam_count = excluded.spam_count,
			ham_count = excluded.ham_count,
			user_correction_count = excluded.user_correction_count,
			auth_pass_count = excluded.auth_pass_count,
			auth_fail_count = excluded.auth_fail_count,
			vip = excluded.vip,
			allowlisted = excluded.allowlisted,
			blocked = excluded.blocked,
			score = excluded.score,
			state = excluded.state,
			updated_at = CURRENT_TIMESTAMP`,
		profile.ID, profile.AccountID, profile.SenderEmail, profile.SenderDomain,
		formatLegitimacyTime(profile.FirstSeen), formatLegitimacyTime(profile.LastSeen),
		profile.MessageCount, profile.ReplyCount, profile.SentCount, profile.SpamCount, profile.HamCount,
		profile.UserCorrectionCount, profile.AuthPassCount, profile.AuthFailCount,
		profile.VIP, profile.Allowlisted, profile.Blocked, profile.Score, profile.State,
	)
	if err != nil {
		return AILegitimacyProfile{}, err
	}
	return db.GetAILegitimacyProfile(ctx, profile.AccountID, profile.SenderEmail)
}

func (db *DB) ListAILegitimacyProfiles(ctx context.Context, accountID string, limit int) ([]AILegitimacyProfile, error) {
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	rows, err := db.Read().QueryContext(ctx, `
		SELECT id, account_id, sender_email, sender_domain, first_seen, last_seen,
		       message_count, reply_count, sent_count, spam_count, ham_count,
		       user_correction_count, auth_pass_count, auth_fail_count,
		       vip, allowlisted, blocked, score, state, updated_at
		FROM ai_legitimacy_profiles
		WHERE account_id = ?
		ORDER BY last_seen DESC, sender_email ASC
		LIMIT ?`, accountID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var profiles []AILegitimacyProfile
	for rows.Next() {
		var profile AILegitimacyProfile
		var firstSeen, lastSeen, updatedAt sql.NullString
		if err := rows.Scan(
			&profile.ID, &profile.AccountID, &profile.SenderEmail, &profile.SenderDomain,
			&firstSeen, &lastSeen, &profile.MessageCount, &profile.ReplyCount,
			&profile.SentCount, &profile.SpamCount, &profile.HamCount,
			&profile.UserCorrectionCount, &profile.AuthPassCount, &profile.AuthFailCount,
			&profile.VIP, &profile.Allowlisted, &profile.Blocked, &profile.Score,
			&profile.State, &updatedAt,
		); err != nil {
			return nil, err
		}
		profile.FirstSeen = parseLegitimacyTime(firstSeen)
		profile.LastSeen = parseLegitimacyTime(lastSeen)
		profile.UpdatedAt = parseLegitimacyTime(updatedAt)
		profiles = append(profiles, profile)
	}
	return profiles, rows.Err()
}

func (db *DB) DeleteAILegitimacyProfile(ctx context.Context, accountID, senderEmail string) error {
	_, err := db.Write().ExecContext(ctx, `DELETE FROM ai_legitimacy_profiles WHERE account_id = ? AND sender_email = ?`, accountID, normalizeLegitimacyEmail(senderEmail))
	return err
}

func normalizeLegitimacyEmail(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func normalizeLegitimacyState(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	switch value {
	case "trusted", "neutral", "suspicious", "malicious":
		return value
	default:
		return "unknown"
	}
}

func formatLegitimacyTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format("2006-01-02 15:04:05")
}

func parseLegitimacyTime(value sql.NullString) time.Time {
	if !value.Valid || value.String == "" {
		return time.Time{}
	}
	for _, layout := range []string{"2006-01-02 15:04:05.999999999-07:00", "2006-01-02 15:04:05", time.RFC3339Nano} {
		if parsed, err := time.Parse(layout, value.String); err == nil {
			return parsed.UTC()
		}
	}
	return time.Time{}
}

func lastIndexByte(value string, needle byte) int {
	for i := len(value) - 1; i >= 0; i-- {
		if value[i] == needle {
			return i
		}
	}
	return -1
}
