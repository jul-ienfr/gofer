package storage

import (
	"context"
	"database/sql"
	"errors"
	"testing"
)

func TestAIProjectsRoutesAndMessageLinksAreOwnerScoped(t *testing.T) {
	ctx := context.Background()
	db := newContactsTestDB(t)
	if _, err := db.Write().Exec(`
		INSERT INTO accounts (id, user_id, email_address, provider) VALUES ('acc', 'default', 'ana@example.com', 'imap');
		INSERT INTO messages (id, account_id, internet_message_id, subject) VALUES (1, 'acc', '<one@example.com>', 'One');
		INSERT INTO users (id, username, username_normalized, name) VALUES ('other', 'other', 'other', 'Other');`); err != nil {
		t.Fatal(err)
	}
	project, err := db.CreateAIProject(ctx, AIProject{UserID: "default", Name: "Employment", Slug: "employment"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.CreateAIProject(ctx, AIProject{UserID: "default", Name: "Employment", Slug: "employment"}); err == nil {
		t.Fatal("duplicate slug accepted")
	}
	route, err := db.CreateAIProjectRoute(ctx, "default", AIProjectRoute{
		ProjectID: project.ID, Name: "Recruiters", Enabled: true, Priority: 10,
		MatchJSON: `{"domains":["jobs.example"]}`, ActionsJSON: `[{"kind":"notify"}]`,
	})
	if err != nil {
		t.Fatal(err)
	}
	routes, err := db.ListAIProjectRoutes(ctx, "default", project.ID)
	if err != nil || len(routes) != 1 || routes[0].ID != route.ID {
		t.Fatalf("routes = %+v, %v", routes, err)
	}
	if _, err := db.CreateAIProjectRoute(ctx, "other", AIProjectRoute{
		ProjectID: project.ID, Name: "Stolen", MatchJSON: `{}`, ActionsJSON: `[]`,
	}); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("cross-user route = %v", err)
	}
	if err := db.LinkAIMessageProject(ctx, "default", AIMessageProject{MessageID: 1, ProjectID: project.ID, Source: "rule", Confidence: 0.9}); err != nil {
		t.Fatal(err)
	}
	if err := db.LinkAIMessageProject(ctx, "other", AIMessageProject{MessageID: 1, ProjectID: project.ID, Source: "manual"}); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("cross-user link = %v", err)
	}
	links, err := db.ListAIMessageProjects(ctx, "default", 1)
	if err != nil || len(links) != 1 || links[0].ProjectID != project.ID {
		t.Fatalf("links = %+v, %v", links, err)
	}
	if err := db.DeleteAIProject(ctx, "other", project.ID); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("cross-user delete = %v", err)
	}
	if err := db.DeleteAIProject(ctx, "default", project.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.GetAIProject(ctx, "default", project.ID); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("after delete = %v", err)
	}
}

func TestEnsureDefaultAIProjectsIsIdempotent(t *testing.T) {
	db := newContactsTestDB(t)
	for i := 0; i < 2; i++ {
		if err := db.EnsureDefaultAIProjects(context.Background(), "default"); err != nil {
			t.Fatal(err)
		}
	}
	projects, err := db.ListAIProjects(context.Background(), "default")
	if err != nil || len(projects) != 3 {
		t.Fatalf("projects = %+v, %v", projects, err)
	}
}
