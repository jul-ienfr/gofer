package aisettings

import (
	"context"
	"errors"
	"strings"

	"github.com/cristianadrielbraun/gofer/internal/storage"
)

const (
	AnalysisEnabledKey        = "ai_analysis_enabled"
	DraftingEnabledKey        = "ai_drafting_enabled"
	AutoSpamEnabledKey        = "ai_auto_spam_enabled"
	AutoUnsubscribeEnabledKey = "ai_auto_unsubscribe_enabled"
)

type Settings struct {
	AnalysisEnabled        bool
	DraftingEnabled        bool
	AutoSpamEnabled        bool
	AutoUnsubscribeEnabled bool
}

func Default() Settings {
	return Settings{}
}

func Load(ctx context.Context, db *storage.DB, userID string) (Settings, error) {
	if db == nil {
		return Settings{}, errors.New("ai settings require a database")
	}
	if strings.TrimSpace(userID) == "" {
		return Settings{}, errors.New("ai settings require a user id")
	}
	rows, err := db.Read().QueryContext(ctx, `
		SELECT key, value FROM app_settings
		WHERE user_id = ? AND key IN (?, ?, ?, ?)`,
		userID, AnalysisEnabledKey, DraftingEnabledKey, AutoSpamEnabledKey, AutoUnsubscribeEnabledKey)
	if err != nil {
		return Settings{}, err
	}
	defer rows.Close()
	var settings Settings
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			return Settings{}, err
		}
		enabled := value == "1"
		switch key {
		case AnalysisEnabledKey:
			settings.AnalysisEnabled = enabled
		case DraftingEnabledKey:
			settings.DraftingEnabled = enabled
		case AutoSpamEnabledKey:
			settings.AutoSpamEnabled = enabled
		case AutoUnsubscribeEnabledKey:
			settings.AutoUnsubscribeEnabled = enabled
		}
	}
	return settings, rows.Err()
}

func Save(ctx context.Context, db *storage.DB, userID string, settings Settings) error {
	if db == nil {
		return errors.New("ai settings require a database")
	}
	if strings.TrimSpace(userID) == "" {
		return errors.New("ai settings require a user id")
	}
	tx, err := db.Write().BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for key, enabled := range map[string]bool{
		AnalysisEnabledKey:        settings.AnalysisEnabled,
		DraftingEnabledKey:        settings.DraftingEnabled,
		AutoSpamEnabledKey:        settings.AutoSpamEnabled,
		AutoUnsubscribeEnabledKey: settings.AutoUnsubscribeEnabled,
	} {
		value := "0"
		if enabled {
			value = "1"
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO app_settings (user_id, key, value, updated_at)
			VALUES (?, ?, ?, CURRENT_TIMESTAMP)
			ON CONFLICT(user_id, key) DO UPDATE SET value = excluded.value, updated_at = CURRENT_TIMESTAMP`,
			userID, key, value); err != nil {
			return err
		}
	}
	return tx.Commit()
}
