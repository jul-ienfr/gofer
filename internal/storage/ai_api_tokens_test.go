package storage

import (
	"context"
	"testing"
	"time"
)

func TestAIAPITokensAreHashedScopedAndRevocable(t *testing.T) {
	db := newContactsTestDB(t)
	ctx := context.Background()
	hash := HashAIAPIToken("gfr_test-token")
	token, err := db.CreateAIAPIToken(ctx, AIAPIToken{
		UserID: "default", Name: "Shortcut", TokenHash: hash, Scopes: "read",
		ExpiresAt: time.Now().UTC().Add(time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}
	if token.ID == "" || token.Scopes != "read" || token.ExpiresAt.IsZero() {
		t.Fatalf("token = %+v", token)
	}
	authenticated, err := db.AuthenticateAIAPIToken(ctx, hash, time.Now())
	if err != nil || authenticated.ID != token.ID {
		t.Fatalf("authenticate = %+v, %v", authenticated, err)
	}
	list, err := db.ListAIAPITokens(ctx, "default")
	if err != nil || len(list) != 1 || list[0].TokenHash != "" {
		t.Fatalf("list = %+v, %v", list, err)
	}
	if _, err := db.AuthenticateAIAPIToken(ctx, HashAIAPIToken("gfr_wrong"), time.Now()); err == nil {
		t.Fatal("wrong token authenticated")
	}
	revoked, err := db.RevokeAIAPIToken(ctx, "default", token.ID)
	if err != nil || !revoked {
		t.Fatalf("revoke = %v, %v", revoked, err)
	}
	if _, err := db.AuthenticateAIAPIToken(ctx, hash, time.Now()); err == nil {
		t.Fatal("revoked token authenticated")
	}
	if _, err := db.AuthenticateAIAPIToken(ctx, hash, token.ExpiresAt.Add(time.Second)); err == nil {
		t.Fatal("expired token authenticated")
	}
}
