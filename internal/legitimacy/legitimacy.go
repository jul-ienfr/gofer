package legitimacy

import (
	"math"
	"net/mail"
	"regexp"
	"strings"
	"time"
)

const Version = "legitimacy-v1.0.0"

type State string

const (
	StateUnknown    State = "unknown"
	StateTrusted    State = "trusted"
	StateNeutral    State = "neutral"
	StateSuspicious State = "suspicious"
	StateMalicious  State = "malicious"
)

type AuthResult string

const (
	AuthPass    AuthResult = "pass"
	AuthPartial AuthResult = "partial"
	AuthFail    AuthResult = "fail"
	AuthAbsent  AuthResult = "absent"
)

type Observation struct {
	MessageCount        int
	ReplyCount          int
	SentCount           int
	SpamCount           int
	HamCount            int
	UserCorrectionCount int
	AuthPassCount       int
	AuthFailCount       int
	FirstSeen           time.Time
	LastSeen            time.Time
	VIP                 bool
	Allowlisted         bool
	Blocked             bool
	KnownContact        bool
}

type Headers struct {
	Auth             AuthResult
	FromDomain       string
	ReplyToDomain    string
	ReturnPathDomain string
	ListUnsubscribe  bool
	ListID           string
	KnownDomains     []string
}

type Reason struct {
	Code   string  `json:"code"`
	Weight float64 `json:"weight,omitempty"`
}

type Result struct {
	State         State    `json:"state"`
	Score         float64  `json:"score"`
	FirstContact  bool     `json:"first_contact"`
	RepliedBefore bool     `json:"replied_before"`
	IsVIP         bool     `json:"vip"`
	Allowlisted   bool     `json:"allowlisted"`
	Blocked       bool     `json:"blocked"`
	AuthPass      bool     `json:"auth_pass"`
	Reasons       []Reason `json:"reasons"`
	Version       string   `json:"version"`
}

func Evaluate(observation Observation, headers Headers, jevRisk string, now time.Time) Result {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	if headers.Auth == "" {
		headers.Auth = AuthAbsent
	}
	authScore := authValue(headers.Auth)
	historyScore := historyValue(observation, now)
	replyScore := 0.0
	if observation.ReplyCount > 0 || observation.SentCount > 0 {
		replyScore = 1
	}
	relationshipScore := 0.0
	if observation.KnownContact {
		relationshipScore = 0.7
	}
	if observation.Allowlisted {
		relationshipScore = 1
	}
	if observation.VIP {
		relationshipScore = 1
	}
	headerScore, headerReasons := headerValue(headers)

	score := 0.30*authScore + 0.25*historyScore + 0.20*replyScore + 0.15*relationshipScore + 0.10*headerScore
	if score < 0 {
		score = 0
	}
	if score > 1 {
		score = 1
	}
	result := Result{
		Score:         math.Round(score*1000) / 1000,
		FirstContact:  observation.MessageCount == 0,
		RepliedBefore: observation.ReplyCount > 0 || observation.SentCount > 0,
		IsVIP:         observation.VIP,
		Allowlisted:   observation.Allowlisted,
		Blocked:       observation.Blocked,
		AuthPass:      headers.Auth == AuthPass,
		Version:       Version,
	}
	result.Reasons = append(result.Reasons, headerReasons...)
	result.Reasons = append(result.Reasons, Reason{Code: "auth_" + string(headers.Auth), Weight: authScore})
	if observation.Blocked {
		result.State = StateMalicious
		result.Reasons = append(result.Reasons, Reason{Code: "blocked"})
		return result
	}

	hostile := strings.EqualFold(jevRisk, "phishing") || strings.EqualFold(jevRisk, "spam")
	if hostile {
		if score < 0.45 || headers.Auth == AuthFail {
			result.State = StateMalicious
			result.Reasons = append(result.Reasons, Reason{Code: "hostile_with_weak_signals"})
		} else {
			result.State = StateSuspicious
			result.Reasons = append(result.Reasons, Reason{Code: "hostile_risk"})
		}
		return result
	}

	if hasHeaderMismatch(headers) || hasLookalike(headers.FromDomain, headers.KnownDomains) {
		result.State = StateSuspicious
		return result
	}
	if observation.VIP || observation.Allowlisted {
		result.State = StateTrusted
		result.Reasons = append(result.Reasons, Reason{Code: "explicit_trust"})
		return result
	}
	if headers.Auth == AuthFail {
		result.State = StateSuspicious
		return result
	}
	if headers.Auth == AuthPass && score >= 0.70 && (result.RepliedBefore || observation.MessageCount >= 3) {
		result.State = StateTrusted
		return result
	}
	if observation.MessageCount == 0 && headers.Auth == AuthAbsent {
		result.State = StateUnknown
		return result
	}
	result.State = StateNeutral
	return result
}

func ParseHeaders(header mail.Header, knownDomains []string) Headers {
	headers := Headers{Auth: ParseAuthentication(header), FromDomain: domainOf(header.Get("From")), ReplyToDomain: domainOf(header.Get("Reply-To")), ReturnPathDomain: domainOf(header.Get("Return-Path")), ListUnsubscribe: strings.TrimSpace(header.Get("List-Unsubscribe")) != "", ListID: strings.TrimSpace(header.Get("List-Id")), KnownDomains: knownDomains}
	if headers.ReplyToDomain == "" {
		headers.ReplyToDomain = headers.FromDomain
	}
	return headers
}

var authResultPattern = regexp.MustCompile(`(?i)\b(spf|dkim|dmarc)\s*=\s*(pass|fail|softfail|neutral|none|temperror|permerror)`)

func ParseAuthentication(header mail.Header) AuthResult {
	var values []string
	for key, entries := range header {
		if strings.EqualFold(key, "Authentication-Results") {
			values = append(values, entries...)
		}
	}
	raw := strings.Join(values, " ")
	matches := authResultPattern.FindAllStringSubmatch(raw, -1)
	if len(matches) == 0 {
		return AuthAbsent
	}
	passes, fails := 0, 0
	for _, match := range matches {
		switch strings.ToLower(match[2]) {
		case "pass":
			passes++
		case "fail", "softfail":
			fails++
		}
	}
	switch {
	case fails > 0:
		return AuthFail
	case passes == 3:
		return AuthPass
	case passes > 0:
		return AuthPartial
	default:
		return AuthAbsent
	}
}

func authValue(result AuthResult) float64 {
	switch result {
	case AuthPass:
		return 1
	case AuthPartial:
		return 0.5
	case AuthFail:
		return 0
	default:
		return 0.4
	}
}

func historyValue(observation Observation, now time.Time) float64 {
	count := float64(observation.MessageCount+observation.ReplyCount+observation.SentCount) / 10
	if count > 1 {
		count = 1
	}
	age := 0.0
	if !observation.FirstSeen.IsZero() && now.After(observation.FirstSeen) {
		age = now.Sub(observation.FirstSeen).Hours() / (24 * 90)
		if age > 1 {
			age = 1
		}
	}
	value := 0.6*count + 0.4*age
	if value > 1 {
		return 1
	}
	return value
}

func headerValue(headers Headers) (float64, []Reason) {
	value := 1.0
	var reasons []Reason
	if hasDomainMismatch(headers.FromDomain, headers.ReplyToDomain) {
		value -= 0.5
		reasons = append(reasons, Reason{Code: "reply_to_mismatch", Weight: -0.5})
	}
	if hasDomainMismatch(headers.FromDomain, headers.ReturnPathDomain) {
		value -= 0.3
		reasons = append(reasons, Reason{Code: "return_path_mismatch", Weight: -0.3})
	}
	if hasLookalike(headers.FromDomain, headers.KnownDomains) {
		value -= 0.5
		reasons = append(reasons, Reason{Code: "lookalike_domain", Weight: -0.5})
	}
	if value < 0 {
		value = 0
	}
	return value, reasons
}

func hasHeaderMismatch(headers Headers) bool {
	return hasDomainMismatch(headers.FromDomain, headers.ReplyToDomain) || hasDomainMismatch(headers.FromDomain, headers.ReturnPathDomain) || hasLookalike(headers.FromDomain, headers.KnownDomains)
}

func domainOf(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if address, err := mail.ParseAddress(value); err == nil {
		value = address.Address
	}
	if at := strings.LastIndex(value, "@"); at >= 0 {
		value = value[at+1:]
	}
	return strings.ToLower(strings.Trim(value, "<> "))
}

func hasDomainMismatch(a, b string) bool {
	if a == "" || b == "" || a == b {
		return false
	}
	return !(strings.HasSuffix(a, "."+b) || strings.HasSuffix(b, "."+a))
}

func hasLookalike(domain string, known []string) bool {
	if domain == "" {
		return false
	}
	for _, candidate := range known {
		candidate = strings.ToLower(strings.TrimSpace(candidate))
		if candidate == "" || candidate == domain {
			continue
		}
		if strings.HasSuffix(domain, "."+candidate) || strings.HasSuffix(candidate, "."+domain) {
			continue
		}
		if domainSuffix(domain) != domainSuffix(candidate) {
			continue
		}
		if levenshtein(domain, candidate) <= 1 && len([]rune(domain)) >= 5 {
			return true
		}
	}
	return false
}

func domainSuffix(domain string) string {
	if index := strings.LastIndex(domain, "."); index >= 0 {
		return domain[index:]
	}
	return ""
}

func levenshtein(a, b string) int {
	left, right := []rune(a), []rune(b)
	if len(left) == 0 {
		return len(right)
	}
	if len(right) == 0 {
		return len(left)
	}
	previous := make([]int, len(right)+1)
	current := make([]int, len(right)+1)
	for j := 0; j <= len(right); j++ {
		previous[j] = j
	}
	for i := 1; i <= len(left); i++ {
		current[0] = i
		for j := 1; j <= len(right); j++ {
			cost := 1
			if left[i-1] == right[j-1] {
				cost = 0
			}
			current[j] = minInt(current[j-1]+1, previous[j]+1, previous[j-1]+cost)
		}
		previous, current = current, previous
	}
	return previous[len(right)]
}

func minInt(values ...int) int {
	result := values[0]
	for _, value := range values[1:] {
		if value < result {
			result = value
		}
	}
	return result
}
