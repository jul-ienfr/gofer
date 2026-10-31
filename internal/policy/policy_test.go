package policy

import (
	"testing"
)

func baseInput() Input {
	return Input{
		Analysis: Analysis{
			Risk: RiskLegit, RiskConfidence: 0.9, Priority: PriorityNormal,
			Intent: "job", ProjectID: "prj_emploi", ProjectSlug: "employment", ProjectName: "Employment",
			ProjectConfidence: 0.9, NeedsReply: true,
		},
		Legitimacy: Legitimacy{State: LegitimacyTrusted},
		Project:    Project{AllowLLMDraft: true, AllowExternalRouting: true},
		Config: Config{
			AnalysisMinConfidence: 0.55, ProjectMinConfidence: 0.70,
			AutoSpamMinConfidence: 0.90, DraftMinConfidence: 0.70, RoutingMinConfidence: 0.75,
			Drafting: true, Routing: true,
		},
	}
}

func hasAction(decision Decision, kind ActionKind) bool {
	for _, action := range decision.Actions {
		if action.Kind == kind {
			return true
		}
	}
	return false
}

func hasReason(decision Decision, reason string) bool {
	for _, code := range decision.ReasonCodes {
		if code == reason {
			return true
		}
	}
	return false
}

func TestLegitMailRoutesLabelsAndDrafts(t *testing.T) {
	decision := Evaluate(baseInput())
	if decision.ProjectID != "prj_emploi" {
		t.Fatalf("project = %q", decision.ProjectID)
	}
	if !hasAction(decision, ActionDraft) || !hasAction(decision, ActionWebhook) || !hasAction(decision, ActionLabel) {
		t.Fatalf("actions = %+v", decision.Actions)
	}
	if decision.RequiresReview {
		t.Fatalf("review = true, reasons %v", decision.ReasonCodes)
	}
}

func TestPhishingBlocksDraftAndRoutingEvenForTrustedSender(t *testing.T) {
	input := baseInput()
	input.Analysis.Risk = RiskPhishing
	input.Analysis.RiskConfidence = 0.99
	decision := Evaluate(input)
	if hasAction(decision, ActionDraft) || hasAction(decision, ActionWebhook) || hasAction(decision, ActionSpam) {
		t.Fatalf("actions = %+v", decision.Actions)
	}
	if !hasAction(decision, ActionLabel) || !hasReason(decision, "phishing") {
		t.Fatalf("decision = %+v", decision)
	}
}

func TestPhishingIsNeverAutoSpammed(t *testing.T) {
	input := baseInput()
	input.Analysis.Risk = RiskPhishing
	input.Analysis.RiskConfidence = 0.99
	input.Config.AutoSpam = true
	input.Project.AllowAutoSpam = true
	decision := Evaluate(input)
	if hasAction(decision, ActionSpam) || hasAction(decision, ActionUnsubscribe) || !decision.RequiresReview || !hasReason(decision, "auto_spam_phishing_review") {
		t.Fatalf("decision = %+v", decision)
	}
}

func TestAutoSpamRequiresEveryGate(t *testing.T) {
	input := baseInput()
	input.Analysis.Risk = RiskSpam
	input.Analysis.RiskConfidence = 0.90
	input.Config.AutoSpam = true
	input.Project.AllowAutoSpam = true
	decision := Evaluate(input)
	if !hasAction(decision, ActionSpam) || decision.RequiresReview {
		t.Fatalf("decision = %+v", decision)
	}

	protected := []func(*Input){
		func(in *Input) { in.Legitimacy.VIP = true },
		func(in *Input) { in.Legitimacy.Allowlisted = true },
		func(in *Input) { in.Legitimacy.RepliedBefore = true },
		func(in *Input) { in.Analysis.RiskConfidence = 0.8999 },
		func(in *Input) { in.Project.AllowAutoSpam = false },
		func(in *Input) { in.Config.AutoSpam = false },
		func(in *Input) { in.Analysis.Signals.FinancialRequest = 0.9 },
		func(in *Input) { in.Analysis.ProjectSlug = "legal" },
	}
	for i, protect := range protected {
		candidate := input
		protect(&candidate)
		got := Evaluate(candidate)
		if hasAction(got, ActionSpam) {
			t.Errorf("case %d auto-spammed: %+v", i, got)
		}
	}
}

func TestAutoUnsubscribeCanRunWithoutMovingSpam(t *testing.T) {
	input := baseInput()
	input.Analysis.Risk = RiskSpam
	input.Analysis.RiskConfidence = 0.95
	input.Config.AutoUnsubscribe = true
	input.Project.AllowAutoSpam = true
	decision := Evaluate(input)
	if hasAction(decision, ActionSpam) || !hasAction(decision, ActionUnsubscribe) || decision.RequiresReview {
		t.Fatalf("decision = %+v", decision)
	}
	input.Legitimacy.VIP = true
	decision = Evaluate(input)
	if hasAction(decision, ActionUnsubscribe) || !decision.RequiresReview {
		t.Fatalf("protected sender decision = %+v", decision)
	}
}

func TestLocalMaliciousCannotSpamAJevLegitimateMail(t *testing.T) {
	input := baseInput()
	input.Legitimacy.State = LegitimacyMalicious
	input.Config.AutoSpam = true
	input.Project.AllowAutoSpam = true
	decision := Evaluate(input)
	if hasAction(decision, ActionSpam) || !hasReason(decision, "local_malicious") {
		t.Fatalf("decision = %+v", decision)
	}
}

func TestUnsureNeverRoutesOrDrafts(t *testing.T) {
	input := baseInput()
	input.Analysis.Risk = RiskUnsure
	decision := Evaluate(input)
	if hasAction(decision, ActionDraft) || hasAction(decision, ActionWebhook) || hasAction(decision, ActionSpam) || hasAction(decision, ActionUnsubscribe) {
		t.Fatalf("decision = %+v", decision)
	}
	if !decision.RequiresReview || !hasReason(decision, "unsure") {
		t.Fatalf("decision = %+v", decision)
	}
}

func TestAnalysisErrorProducesOnlyReview(t *testing.T) {
	input := baseInput()
	input.Analysis.Risk = RiskError
	decision := Evaluate(input)
	if !decision.RequiresReview || len(decision.Actions) != 2 || decision.Actions[0].Label != "Needs review" {
		t.Fatalf("decision = %+v", decision)
	}
	if hasAction(decision, ActionDraft) || hasAction(decision, ActionWebhook) {
		t.Fatalf("destructive action = %+v", decision.Actions)
	}
}

func TestFinancialAndLegalMailsDoNotDraft(t *testing.T) {
	financial := baseInput()
	financial.Analysis.Signals.FinancialRequest = 0.9
	decision := Evaluate(financial)
	if hasAction(decision, ActionDraft) || hasAction(decision, ActionWebhook) || !hasReason(decision, "financial_request") {
		t.Fatalf("financial = %+v", decision)
	}

	legal := baseInput()
	legal.Analysis.ProjectSlug = "legal"
	legal.Analysis.Intent = "legal"
	decision = Evaluate(legal)
	if hasAction(decision, ActionDraft) {
		t.Fatalf("legal = %+v", decision)
	}
}

func TestConfidenceThresholdsAreDeterministic(t *testing.T) {
	input := baseInput()
	input.Analysis.RiskConfidence = 0.6999
	decision := Evaluate(input)
	if hasAction(decision, ActionDraft) || !hasReason(decision, "draft_review") {
		t.Fatalf("below draft threshold = %+v", decision)
	}
	input.Analysis.RiskConfidence = 0.70
	decision = Evaluate(input)
	if !hasAction(decision, ActionDraft) {
		t.Fatalf("at draft threshold = %+v", decision)
	}
}

func TestModerateRiskConfidenceForcesReview(t *testing.T) {
	input := baseInput()
	input.Analysis.RiskConfidence = 0.60
	decision := Evaluate(input)
	if !decision.RequiresReview || !hasReason(decision, "moderate_confidence") {
		t.Fatalf("decision = %+v", decision)
	}
}

func TestDeadlineElevatesPriorityOnlyForLegitMail(t *testing.T) {
	input := baseInput()
	input.Analysis.Signals.Deadline = 0.9
	decision := Evaluate(input)
	if decision.Priority != PriorityHigh {
		t.Fatalf("priority = %q", decision.Priority)
	}
}

func TestConfigBounds(t *testing.T) {
	cases := []Config{
		{AnalysisMinConfidence: 0.2, ProjectMinConfidence: 0.7, AutoSpamMinConfidence: 0.9, DraftMinConfidence: 0.7, RoutingMinConfidence: 0.75},
		{AnalysisMinConfidence: 0.55, ProjectMinConfidence: 0.7, AutoSpamMinConfidence: 0.80, DraftMinConfidence: 0.7, RoutingMinConfidence: 0.75},
		{AnalysisMinConfidence: 0.55, ProjectMinConfidence: 0.7, AutoSpamMinConfidence: 0.9, DraftMinConfidence: 0.7, RoutingMinConfidence: 0.99},
	}
	for _, config := range cases {
		if err := config.Validate(); err == nil {
			t.Errorf("config %+v accepted", config)
		}
	}
	if err := DefaultConfig().Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestInvalidPolicyConfigNeverProducesActions(t *testing.T) {
	input := baseInput()
	input.Config.AutoSpamMinConfidence = 0.5
	decision := Evaluate(input)
	if !decision.RequiresReview || hasAction(decision, ActionDraft) || hasAction(decision, ActionWebhook) {
		t.Fatalf("decision = %+v", decision)
	}
}
