package legitimacy

import (
	"net/mail"
	"strings"
	"testing"
	"time"
)

func headerMap(values map[string]string) mail.Header {
	header := mail.Header{}
	for key, value := range values {
		header[key] = []string{value}
	}
	return header
}

func TestEstablishedAuthenticatedSenderBecomesTrusted(t *testing.T) {
	now := time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC)
	result := Evaluate(Observation{
		MessageCount: 8, ReplyCount: 3, SentCount: 2,
		FirstSeen: now.AddDate(0, -6, 0), LastSeen: now.Add(-time.Hour),
		AuthPassCount: 8,
	}, Headers{Auth: AuthPass, FromDomain: "example.com", ReplyToDomain: "example.com", ReturnPathDomain: "example.com"}, "legit", now)
	if result.State != StateTrusted {
		t.Fatalf("state = %q, result %+v", result.State, result)
	}
	if result.Score < 0.70 || result.FirstContact || !result.RepliedBefore || !result.AuthPass {
		t.Fatalf("result = %+v", result)
	}
}

func TestFailedAuthenticationIsSuspiciousEvenForKnownSender(t *testing.T) {
	result := Evaluate(Observation{MessageCount: 20, ReplyCount: 5, KnownContact: true}, Headers{Auth: AuthFail, FromDomain: "example.com", ReplyToDomain: "example.com", ReturnPathDomain: "example.com"}, "legit", time.Now())
	if result.State != StateSuspicious {
		t.Fatalf("state = %q, result %+v", result.State, result)
	}
}

func TestTrustedSenderCannotWhitewashJevPhishing(t *testing.T) {
	result := Evaluate(Observation{MessageCount: 30, ReplyCount: 10, Allowlisted: true}, Headers{Auth: AuthPass, FromDomain: "example.com", ReplyToDomain: "example.com", ReturnPathDomain: "example.com"}, "phishing", time.Now())
	if result.State == StateTrusted {
		t.Fatalf("state = %q", result.State)
	}
	if result.State != StateSuspicious && result.State != StateMalicious {
		t.Fatalf("state = %q", result.State)
	}
}

func TestBlockedAndLookalikeDomainsAreMalicious(t *testing.T) {
	blocked := Evaluate(Observation{Blocked: true}, Headers{Auth: AuthPass, FromDomain: "example.com"}, "legit", time.Now())
	if blocked.State != StateMalicious {
		t.Fatalf("blocked state = %q", blocked.State)
	}
	lookalike := Evaluate(Observation{MessageCount: 1}, Headers{Auth: AuthPass, FromDomain: "examp1e.com", KnownDomains: []string{"example.com"}}, "legit", time.Now())
	if lookalike.State != StateSuspicious {
		t.Fatalf("lookalike state = %q", lookalike.State)
	}
}

func TestFirstContactWithoutAuthenticationIsUnknown(t *testing.T) {
	result := Evaluate(Observation{}, Headers{Auth: AuthAbsent, FromDomain: "new.example"}, "legit", time.Now())
	if result.State != StateUnknown || !result.FirstContact {
		t.Fatalf("result = %+v", result)
	}
}

func TestParseAuthenticationResults(t *testing.T) {
	cases := []struct {
		header string
		want   AuthResult
	}{
		{"mx.example; spf=pass smtp.mailfrom=a@example.com; dkim=pass header.d=example.com; dmarc=pass", AuthPass},
		{"mx.example; spf=pass; dkim=pass", AuthPartial},
		{"mx.example; spf=fail; dkim=pass; dmarc=fail", AuthFail},
		{"mx.example; spf=softfail", AuthFail},
		{"", AuthAbsent},
	}
	for _, test := range cases {
		got := ParseAuthentication(headerMap(map[string]string{"Authentication-Results": test.header}))
		if got != test.want {
			t.Errorf("header %q = %q, want %q", test.header, got, test.want)
		}
	}
}

func TestHeaderMismatchAndBootstrapping(t *testing.T) {
	headers := ParseHeaders(headerMap(map[string]string{
		"From":        "Alice <alice@example.com>",
		"Reply-To":    "billing@evil.example.net",
		"Return-Path": "<bounce@evil.example.net>",
	}), []string{"example.com"})
	if !strings.EqualFold(headers.FromDomain, "example.com") || !hasHeaderMismatch(headers) {
		t.Fatalf("headers = %+v", headers)
	}
}
