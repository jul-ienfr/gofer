package handler

import (
	"context"
	"strings"
	"testing"

	"github.com/cristianadrielbraun/gofer/internal/i18n"
)

// Every AI API error that goes through serverText must have a real value in
// each fully translated catalog, not just an English fallback or the raw key.
func TestServerTextTranslatesAIErrors(t *testing.T) {
	for _, code := range []string{"fr", "es", "pt-br", "zh-cn"} {
		t.Run(code, func(t *testing.T) {
			locale, ok := i18n.Parse(code)
			if !ok {
				t.Fatalf("%s locale is unavailable", code)
			}
			ctx := i18n.WithLocale(context.Background(), locale)
			count := 0
			for text, key := range serverTextKeys {
				if !strings.HasPrefix(key, "server.error.ai_") {
					continue
				}
				count++
				if got := serverText(ctx, text); got == text || got == key {
					t.Errorf("serverText(%q) = %q, want a %s message", text, got, code)
				}
			}
			if count < 70 {
				t.Fatalf("only %d AI errors are mapped, want at least 70", count)
			}
		})
	}
}

// Every mapped server error must have a real value in each fully translated
// catalog, not just an English fallback or the raw key.
func TestServerTextTranslatesEveryMappedError(t *testing.T) {
	for _, code := range []string{"fr", "es", "pt-br", "zh-cn"} {
		t.Run(code, func(t *testing.T) {
			locale, ok := i18n.Parse(code)
			if !ok {
				t.Fatalf("%s locale is unavailable", code)
			}
			ctx := i18n.WithLocale(context.Background(), locale)
			for text, key := range serverTextKeys {
				// "Invitation" is the same word in French.
				if code == "fr" && key == "server.msg.invitation" {
					continue
				}
				if got := serverText(ctx, text); got == text || got == key {
					t.Errorf("serverText(%q) = %q, want a %s message", text, got, code)
				}
			}
		})
	}
}
