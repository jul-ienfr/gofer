package projects

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

const Version = "routes-v1.0.0"

type Match struct {
	Domains        []string `json:"domains,omitempty"`
	Senders        []string `json:"senders,omitempty"`
	SubjectRegex   string   `json:"subject_regex,omitempty"`
	Intents        []string `json:"intents,omitempty"`
	Risk           []string `json:"risk,omitempty"`
	MinConfidence  float64  `json:"min_confidence,omitempty"`
	ListIDs        []string `json:"list_ids,omitempty"`
	HasAttachments *bool    `json:"has_attachments,omitempty"`
}

type Action struct {
	Kind  string `json:"kind"`
	Value string `json:"value,omitempty"`
}

type Route struct {
	ID         string
	ProjectID  string
	Name       string
	Priority   int
	Enabled    bool
	Match      Match
	Actions    []Action
	MatchRaw   string
	ActionsRaw string
}

type Candidate struct {
	Sender         string
	SenderDomain   string
	Subject        string
	Intent         string
	Risk           string
	Confidence     float64
	ListID         string
	HasAttachments bool
}

type Decision struct {
	Route          Route
	Matched        bool
	RequiresReview bool
	ReasonCodes    []string
	Version        string
}

func ParseMatch(raw string) (Match, error) {
	var match Match
	if strings.TrimSpace(raw) == "" {
		return match, nil
	}
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&match); err != nil {
		return Match{}, fmt.Errorf("invalid route match: %w", err)
	}
	if match.MinConfidence < 0 || match.MinConfidence > 1 {
		return Match{}, fmt.Errorf("route min_confidence must be between 0 and 1")
	}
	if match.SubjectRegex != "" {
		if len(match.SubjectRegex) > 512 {
			return Match{}, fmt.Errorf("subject_regex is too long")
		}
		if _, err := regexp.Compile(match.SubjectRegex); err != nil {
			return Match{}, fmt.Errorf("invalid subject_regex: %w", err)
		}
	}
	return match, nil
}

func ParseActions(raw string) ([]Action, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	var actions []Action
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&actions); err != nil {
		return nil, fmt.Errorf("invalid route actions: %w", err)
	}
	for _, action := range actions {
		switch action.Kind {
		case "label", "move", "notify", "webhook", "draft":
		default:
			return nil, fmt.Errorf("unsupported route action %q", action.Kind)
		}
	}
	return actions, nil
}

func ParseRoute(id, projectID, name string, priority int, enabled bool, matchRaw, actionsRaw string) (Route, error) {
	match, err := ParseMatch(matchRaw)
	if err != nil {
		return Route{}, err
	}
	actions, err := ParseActions(actionsRaw)
	if err != nil {
		return Route{}, err
	}
	return Route{ID: id, ProjectID: projectID, Name: name, Priority: priority, Enabled: enabled, Match: match, Actions: actions, MatchRaw: matchRaw, ActionsRaw: actionsRaw}, nil
}

func Evaluate(candidate Candidate, routes []Route) Decision {
	decision := Decision{Version: Version}
	risk := strings.ToLower(strings.TrimSpace(candidate.Risk))
	if risk == "" || risk == "phishing" || risk == "spam" || risk == "error" {
		decision.ReasonCodes = append(decision.ReasonCodes, "security_guard")
		return decision
	}
	if risk == "unsure" {
		decision.RequiresReview = true
		decision.ReasonCodes = append(decision.ReasonCodes, "unsure_requires_review")
	}
	best := -1
	for i, route := range routes {
		if !route.Enabled {
			continue
		}
		if best >= 0 && route.Priority >= routes[best].Priority {
			continue
		}
		if matches(route.Match, candidate, risk) {
			best = i
		}
	}
	if best < 0 {
		decision.ReasonCodes = append(decision.ReasonCodes, "no_route")
		return decision
	}
	if decision.RequiresReview {
		decision.ReasonCodes = append(decision.ReasonCodes, "route_requires_review")
		return decision
	}
	decision.Route = routes[best]
	decision.Matched = true
	return decision
}

func matches(match Match, candidate Candidate, risk string) bool {
	if len(match.Risk) > 0 && !containsFold(match.Risk, risk) {
		return false
	}
	if candidate.Confidence < match.MinConfidence {
		return false
	}
	if match.HasAttachments != nil && *match.HasAttachments != candidate.HasAttachments {
		return false
	}
	if len(match.Domains) > 0 && !matchDomain(match.Domains, candidate.SenderDomain) {
		return false
	}
	if len(match.Senders) > 0 && !containsFold(match.Senders, candidate.Sender) {
		return false
	}
	if len(match.Intents) > 0 && !containsFold(match.Intents, candidate.Intent) {
		return false
	}
	if len(match.ListIDs) > 0 && !containsFold(match.ListIDs, candidate.ListID) {
		return false
	}
	if match.SubjectRegex != "" {
		compiled, err := regexp.Compile(match.SubjectRegex)
		if err != nil || !compiled.MatchString(candidate.Subject) {
			return false
		}
	}
	return true
}

func matchDomain(patterns []string, domain string) bool {
	domain = strings.ToLower(strings.TrimSpace(domain))
	for _, pattern := range patterns {
		pattern = strings.ToLower(strings.TrimSpace(pattern))
		if pattern == domain {
			return true
		}
		if strings.HasPrefix(pattern, "*.") {
			suffix := strings.TrimPrefix(pattern, "*")
			if strings.HasSuffix(domain, suffix) && len(domain) > len(suffix) {
				return true
			}
		}
	}
	return false
}

func containsFold(values []string, value string) bool {
	value = strings.ToLower(strings.TrimSpace(value))
	for _, candidate := range values {
		if strings.EqualFold(strings.TrimSpace(candidate), value) {
			return true
		}
	}
	return false
}
