package storage

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"
)

func TestAILegitimacyProfilesAreAccountScopedAndUpserted(t *testing.T) {
	ctx := context.Background()
	db := newContactsTestDB(t)
	if _, err := db.Write().Exec(`
		INSERT INTO accounts (id, user_id, email_address, provider) VALUES ('acc', 'default', 'ana@example.com', 'imap');
		INSERT INTO accounts (id, user_id, email_address, provider) VALUES ('other', 'default', 'other@example.com', 'imap');`); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 10, 29, 10, 0, 0, 0, time.UTC)
	profile, err := db.SaveAILegitimacyProfile(ctx, AILegitimacyProfile{
		AccountID: "acc", SenderEmail: "Alice@Example.com", MessageCount: 2,
		FirstSeen: now.Add(-time.Hour), LastSeen: now, Score: 0.8, State: "trusted", VIP: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if profile.ID == "" || profile.SenderEmail != "alice@example.com" || profile.SenderDomain != "example.com" {
		t.Fatalf("profile = %+v", profile)
	}
	profile.MessageCount = 3
	profile.State = "neutral"
	updated, err := db.SaveAILegitimacyProfile(ctx, profile)
	if err != nil {
		t.Fatal(err)
	}
	if updated.MessageCount != 3 || updated.State != "neutral" || !updated.VIP || !updated.FirstSeen.Equal(profile.FirstSeen) {
		t.Fatalf("updated = %+v", updated)
	}
	got, err := db.GetAILegitimacyProfile(ctx, "acc", "alice@example.com")
	if err != nil || got.ID != profile.ID || got.MessageCount != 3 {
		t.Fatalf("got = %+v, %v", got, err)
	}
	if _, err := db.GetAILegitimacyProfile(ctx, "other", "alice@example.com"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("cross-account read = %v", err)
	}
	profiles, err := db.ListAILegitimacyProfiles(ctx, "acc", 10)
	if err != nil || len(profiles) != 1 || profiles[0].ID != profile.ID {
		t.Fatalf("list = %+v, %v", profiles, err)
	}
	if err := db.DeleteAILegitimacyProfile(ctx, "acc", "alice@example.com"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.GetAILegitimacyProfile(ctx, "acc", "alice@example.com"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("after delete = %v", err)
	}
}
