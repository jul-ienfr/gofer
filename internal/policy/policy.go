package policy

import (
	"errors"
	"fmt"
	"math"
	"strings"
)

const Version = "policy-v1.0.0"

type Risk string

const (
	RiskLegit    Risk = "legit"
	RiskSpam     Risk = "spam"
	RiskPhishing Risk = "phishing"
	RiskUnsure   Risk = "unsure"
	RiskError    Risk = "error"
)

type Priority string

const (
	PriorityLow      Priority = "low"
	PriorityNormal   Priority = "normal"
	PriorityHigh     Priority = "high"
	PriorityCritical Priority = "critical"
)

type LegitimacyState string

const (
	LegitimacyUnknown    LegitimacyState = "unknown"
	LegitimacyTrusted    LegitimacyState = "trusted"
	LegitimacyNeutral    LegitimacyState = "neutral"
	LegitimacySuspicious LegitimacyState = "suspicious"
	LegitimacyMalicious  LegitimacyState = "malicious"
)

type ActionKind string

const (
	ActionLabel       ActionKind = "label"
	ActionMove        ActionKind = "move"
	ActionNotify      ActionKind = "notify"
	ActionWebhook     ActionKind = "webhook"
	ActionDraft       ActionKind = "draft"
	ActionSpam        ActionKind = "spam"
	ActionUnsubscribe ActionKind = "unsubscribe"
	ActionReview      ActionKind = "review"
)

type Action struct {
	Kind     ActionKind `json:"kind"`
	Target   string     `json:"target,omitempty"`
	Label    string     `json:"label,omitempty"`
	External bool       `json:"external,omitempty"`
}

type Analysis struct {
	Risk              Risk
	RiskConfidence    float64
	Priority          Priority
	Intent            string
	ProjectID         string
	ProjectSlug       string
	ProjectName       string
	ProjectConfidence float64
	NeedsReply        bool
	Signals           Signals
}

type Signals struct {
	Bulk              float64
	CredentialRequest float64
	FinancialRequest  float64
	Deadline          float64
	FirstContact      bool
}

type Legitimacy struct {
	State         LegitimacyState
	Score         float64
	VIP           bool
	Allowlisted   bool
	Blocked       bool
	RepliedBefore bool
}

type Project struct {
	AllowExternalRouting bool
	AllowLLMDraft        bool
	AllowAutoSpam        bool
}

type Config struct {
	AnalysisMinConfidence float64
	ProjectMinConfidence  float64
	AutoSpamMinConfidence float64
	DraftMinConfidence    float64
	RoutingMinConfidence  float64

	AnalyzeEnabled  bool
	AutoSpam        bool
	AutoUnsubscribe bool
	Drafting        bool
	Routing         bool
}

type Input struct {
	Analysis   Analysis
	Legitimacy Legitimacy
	Project    Project
	Config     Config
}

type Decision struct {
	Priority       Priority `json:"priority"`
	Risk           Risk     `json:"risk"`
	ProjectID      string   `json:"project_id,omitempty"`
	Actions        []Action `json:"actions"`
	RequiresReview bool     `json:"requires_review"`
	ReasonCodes    []string `json:"reason_codes"`
	Version        string   `json:"version"`
}

func DefaultConfig() Config {
	return Config{
		AnalysisMinConfidence: 0.55,
		ProjectMinConfidence:  0.70,
		AutoSpamMinConfidence: 0.90,
		DraftMinConfidence:    0.70,
		RoutingMinConfidence:  0.75,
	}
}

func (c Config) Validate() error {
	checks := []struct {
		name     string
		value    float64
		min, max float64
	}{
		{"analysis_min_confidence", c.AnalysisMinConfidence, 0.40, 0.80},
		{"project_min_confidence", c.ProjectMinConfidence, 0.50, 0.90},
		{"auto_spam_min_confidence", c.AutoSpamMinConfidence, 0.85, 0.98},
		{"draft_min_confidence", c.DraftMinConfidence, 0.50, 0.95},
		{"routing_min_confidence", c.RoutingMinConfidence, 0.60, 0.95},
	}
	for _, check := range checks {
		if math.IsNaN(check.value) || check.value < check.min || check.value > check.max {
			return fmt.Errorf("%s must be between %.2f and %.2f", check.name, check.min, check.max)
		}
	}
	return nil
}

func Evaluate(input Input) Decision {
	config := input.Config
	decision := Decision{
		Priority: normalizePriority(input.Analysis.Priority),
		Risk:     normalizeRisk(input.Analysis.Risk),
		Version:  Version,
		Actions:  []Action{},
	}
	if decision.Priority == "" {
		decision.Priority = PriorityNormal
	}
	if config.AnalysisMinConfidence == 0 {
		config.AnalysisMinConfidence = DefaultConfig().AnalysisMinConfidence
	}
	if config.ProjectMinConfidence == 0 {
		config.ProjectMinConfidence = DefaultConfig().ProjectMinConfidence
	}
	if config.AutoSpamMinConfidence == 0 {
		config.AutoSpamMinConfidence = DefaultConfig().AutoSpamMinConfidence
	}
	if config.DraftMinConfidence == 0 {
		config.DraftMinConfidence = DefaultConfig().DraftMinConfidence
	}
	if config.RoutingMinConfidence == 0 {
		config.RoutingMinConfidence = DefaultConfig().RoutingMinConfidence
	}

	if config.Validate() != nil {
		return review(decision, "policy_invalid")
	}
	if decision.Risk == RiskError {
		return review(decision, "analysis_error")
	}
	if invalidConfidence(input.Analysis.RiskConfidence) {
		return review(decision, "invalid_confidence")
	}
	if input.Legitimacy.State == LegitimacyMalicious {
		decision.ReasonCodes = append(decision.ReasonCodes, "local_malicious")
	}

	switch decision.Risk {
	case RiskPhishing:
		decision.Actions = append(decision.Actions, Action{Kind: ActionLabel, Label: "Phishing"})
		return handleHostile(decision, input, config, "phishing")
	case RiskSpam:
		decision.Actions = append(decision.Actions, Action{Kind: ActionLabel, Label: "Spam"})
		return handleHostile(decision, input, config, "spam")
	case RiskUnsure:
		decision.Actions = append(decision.Actions, Action{Kind: ActionLabel, Label: "Needs review"})
		decision.RequiresReview = true
		decision.ReasonCodes = append(decision.ReasonCodes, "unsure")
		return decision
	case RiskLegit:
	default:
		decision.Actions = append(decision.Actions, Action{Kind: ActionLabel, Label: "Needs review"})
		decision.RequiresReview = true
		decision.ReasonCodes = append(decision.ReasonCodes, "unknown_risk")
		return decision
	}

	confidence := input.Analysis.RiskConfidence
	if confidence < config.AnalysisMinConfidence {
		return review(decision, "low_confidence")
	}
	if confidence < config.ProjectMinConfidence {
		decision.RequiresReview = true
		decision.ReasonCodes = append(decision.ReasonCodes, "moderate_confidence")
	}

	projectReady := input.Analysis.ProjectID != "" &&
		input.Analysis.ProjectSlug != "" &&
		input.Analysis.ProjectSlug != "unknown" &&
		input.Analysis.ProjectConfidence >= config.ProjectMinConfidence
	if projectReady {
		decision.ProjectID = input.Analysis.ProjectID
		decision.Actions = append(decision.Actions, Action{
			Kind: ActionLabel, Label: "Project/" + input.Analysis.ProjectSlug,
		})
	} else if input.Analysis.ProjectID != "" && input.Analysis.ProjectConfidence < config.ProjectMinConfidence {
		decision.RequiresReview = true
		decision.ReasonCodes = append(decision.ReasonCodes, "project_low_confidence")
	}

	legalProject := strings.EqualFold(input.Analysis.ProjectSlug, "legal") || strings.EqualFold(input.Analysis.Intent, "legal")
	financialRisk := input.Analysis.Signals.FinancialRequest >= 0.8
	credentialRisk := input.Analysis.Signals.CredentialRequest >= 0.8
	if financialRisk || credentialRisk {
		decision.RequiresReview = true
		if financialRisk {
			decision.ReasonCodes = append(decision.ReasonCodes, "financial_request")
		}
		if credentialRisk {
			decision.ReasonCodes = append(decision.ReasonCodes, "credential_request")
		}
	}

	if input.Analysis.Signals.Deadline >= 0.8 && confidence >= config.AnalysisMinConfidence {
		decision.Priority = elevate(decision.Priority)
	}

	if projectReady && input.Config.Routing && input.Project.AllowExternalRouting &&
		input.Analysis.ProjectConfidence >= config.RoutingMinConfidence && !financialRisk && !credentialRisk {
		decision.Actions = append(decision.Actions, Action{Kind: ActionWebhook, Target: input.Analysis.ProjectID, External: true})
	} else if projectReady && !input.Project.AllowExternalRouting {
		decision.ReasonCodes = append(decision.ReasonCodes, "routing_not_authorized")
	}

	if projectReady && input.Config.Drafting && input.Project.AllowLLMDraft &&
		input.Analysis.NeedsReply && confidence >= config.DraftMinConfidence &&
		!legalProject && !financialRisk && !credentialRisk && input.Legitimacy.State != LegitimacySuspicious {
		decision.Actions = append(decision.Actions, Action{Kind: ActionDraft, Target: input.Analysis.ProjectID})
	} else if decision.RequiresReview && input.Analysis.NeedsReply {
		decision.ReasonCodes = append(decision.ReasonCodes, "draft_review")
	}

	if len(decision.Actions) > 0 {
		decision.Actions = append(decision.Actions, Action{Kind: ActionNotify, Target: decision.ProjectID})
	}
	return decision
}

func handleHostile(decision Decision, input Input, config Config, reason string) Decision {
	decision.RequiresReview = true
	decision.ReasonCodes = append(decision.ReasonCodes, reason)
	if reason != "spam" {
		decision.ReasonCodes = append(decision.ReasonCodes, "auto_spam_phishing_review")
		return decision
	}
	if !input.Project.AllowAutoSpam || (!config.AutoSpam && !config.AutoUnsubscribe) {
		decision.ReasonCodes = append(decision.ReasonCodes, "auto_spam_disabled")
		return decision
	}
	if input.Analysis.RiskConfidence < config.AutoSpamMinConfidence {
		decision.ReasonCodes = append(decision.ReasonCodes, "auto_spam_low_confidence")
		return decision
	}
	if input.Legitimacy.VIP || input.Legitimacy.Allowlisted || input.Legitimacy.Blocked || input.Legitimacy.RepliedBefore {
		decision.ReasonCodes = append(decision.ReasonCodes, "auto_spam_contact_protected")
		return decision
	}
	if input.Analysis.Signals.FinancialRequest >= 0.8 {
		decision.ReasonCodes = append(decision.ReasonCodes, "auto_spam_financial_review")
		return decision
	}
	if strings.EqualFold(input.Analysis.ProjectSlug, "legal") || strings.EqualFold(input.Analysis.Intent, "legal") {
		decision.ReasonCodes = append(decision.ReasonCodes, "auto_spam_legal_review")
		return decision
	}
	if config.AutoSpam {
		decision.Actions = append(decision.Actions, Action{Kind: ActionSpam, Target: "junk"})
	}
	if config.AutoUnsubscribe {
		decision.Actions = append(decision.Actions, Action{Kind: ActionUnsubscribe, Target: "one-click"})
	}
	if len(decision.Actions) == 0 {
		decision.ReasonCodes = append(decision.ReasonCodes, "auto_spam_disabled")
		return decision
	}
	decision.RequiresReview = false
	return decision
}

func review(decision Decision, reason string) Decision {
	decision.RequiresReview = true
	decision.ReasonCodes = append(decision.ReasonCodes, reason)
	decision.Actions = []Action{{Kind: ActionLabel, Label: "Needs review"}, {Kind: ActionReview}}
	return decision
}

func normalizeRisk(risk Risk) Risk {
	switch Risk(strings.ToLower(strings.TrimSpace(string(risk)))) {
	case RiskLegit, "legitimate", "normal":
		return RiskLegit
	case RiskSpam:
		return RiskSpam
	case RiskPhishing:
		return RiskPhishing
	case RiskUnsure:
		return RiskUnsure
	case RiskError, "":
		return RiskError
	default:
		return RiskError
	}
}

func normalizePriority(priority Priority) Priority {
	switch Priority(strings.ToLower(strings.TrimSpace(string(priority)))) {
	case PriorityLow, "p3":
		return PriorityLow
	case PriorityNormal, "p2":
		return PriorityNormal
	case PriorityHigh, "p1":
		return PriorityHigh
	case PriorityCritical, "p0":
		return PriorityCritical
	default:
		return PriorityNormal
	}
}

func elevate(priority Priority) Priority {
	switch priority {
	case PriorityLow:
		return PriorityNormal
	case PriorityNormal:
		return PriorityHigh
	default:
		return priority
	}
}

func invalidConfidence(value float64) bool {
	return math.IsNaN(value) || math.IsInf(value, 0) || value < 0 || value > 1
}

var ErrInvalidInput = errors.New("invalid policy input")
