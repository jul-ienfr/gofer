package handler

import (
	"context"
	"strings"
	"testing"

	"github.com/cristianadrielbraun/gofer/internal/i18n"
)

// Every AI API error that goes through serverText must have a French value,
// not just an English fallback or the raw key.
func TestServerTextTranslatesAIErrors(t *testing.T) {
	fr, ok := i18n.Parse("fr")
	if !ok {
		t.Fatal("French locale is unavailable")
	}
	ctx := i18n.WithLocale(context.Background(), fr)
	count := 0
	for text, key := range serverTextKeys {
		if !strings.HasPrefix(key, "server.error.ai_") {
			continue
		}
		count++
		if got := serverText(ctx, text); got == text || got == key {
			t.Errorf("serverText(%q) = %q, want a French message", text, got)
		}
	}
	if count < 70 {
		t.Fatalf("only %d AI errors are mapped, want at least 70", count)
	}
}
