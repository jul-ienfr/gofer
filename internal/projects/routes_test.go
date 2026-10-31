package projects

import (
	"testing"
)

func route(t *testing.T, id string, priority int, match string, actions string) Route {
	t.Helper()
	parsed, err := ParseRoute(id, "prj", id, priority, true, match, actions)
	if err != nil {
		t.Fatal(err)
	}
	return parsed
}

func TestHighestPriorityMatchingRouteWins(t *testing.T) {
	routes := []Route{
		route(t, "fallback", 30, `{"domains":["example.com"]}`, `[{"kind":"label","value":"Project/Employment"}]`),
		route(t, "recruiter", 10, `{"domains":["jobs.example.com"],"intents":["job"]}`, `[{"kind":"notify"}]`),
	}
	decision := Evaluate(Candidate{Sender: "alice@jobs.example.com", SenderDomain: "jobs.example.com", Subject: "Interview", Intent: "job", Risk: "legit", Confidence: 0.9}, routes)
	if !decision.Matched || decision.Route.ID != "recruiter" {
		t.Fatalf("decision = %+v", decision)
	}
}

func TestSecurityGuardBlocksPhishingSpamAndErrors(t *testing.T) {
	routes := []Route{route(t, "all", 10, `{}`, `[{"kind":"notify"}]`)}
	for _, risk := range []string{"phishing", "spam", "error", ""} {
		decision := Evaluate(Candidate{Risk: risk, Confidence: 1}, routes)
		if decision.Matched {
			t.Errorf("risk %q routed to %+v", risk, decision.Route)
		}
		if len(decision.ReasonCodes) == 0 || decision.ReasonCodes[0] != "security_guard" {
			t.Errorf("risk %q reasons = %+v", risk, decision.ReasonCodes)
		}
	}
}

func TestUnsureNeverRoutes(t *testing.T) {
	routes := []Route{route(t, "all", 10, `{"risk":["unsure"]}`, `[{"kind":"notify"}]`)}
	decision := Evaluate(Candidate{Risk: "unsure", Confidence: 1}, routes)
	if decision.Matched || !decision.RequiresReview {
		t.Fatalf("decision = %+v", decision)
	}
}

func TestMatchFieldsAndWildcards(t *testing.T) {
	routes := []Route{
		route(t, "wild", 10, `{"domains":["*.jobs.example"],"subject_regex":"(?i)entretien","has_attachments":true,"list_ids":["jobs.example"]}`, `[]`),
	}
	candidate := Candidate{
		SenderDomain: "eu.jobs.example", Subject: "Entretien jeudi", Risk: "legit",
		Confidence: 0.9, HasAttachments: true, ListID: "jobs.example",
	}
	if decision := Evaluate(candidate, routes); !decision.Matched {
		t.Fatalf("decision = %+v", decision)
	}
	candidate.HasAttachments = false
	if decision := Evaluate(candidate, routes); decision.Matched {
		t.Fatalf("decision = %+v", decision)
	}
}

func TestInvalidMatchAndActionsAreRejected(t *testing.T) {
	if _, err := ParseMatch(`{"unknown":true}`); err == nil {
		t.Fatal("unknown match field accepted")
	}
	if _, err := ParseMatch(`{"subject_regex":"("}`); err == nil {
		t.Fatal("invalid regex accepted")
	}
	if _, err := ParseMatch(`{"min_confidence":1.5}`); err == nil {
		t.Fatal("invalid confidence accepted")
	}
	if _, err := ParseActions(`[{"kind":"spam"}]`); err == nil {
		t.Fatal("spam action accepted")
	}
	if _, err := ParseActions(`[{"kind":"send"}]`); err == nil {
		t.Fatal("send action accepted")
	}
}
