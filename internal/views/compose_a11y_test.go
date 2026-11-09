package views

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestComposeFieldLabelsRecipientInput(t *testing.T) {
	var out bytes.Buffer
	if err := ComposeField("To", "to", "recipient@example.com").Render(context.Background(), &out); err != nil {
		t.Fatalf("ComposeField.Render() error = %v", err)
	}
	html := out.String()
	for _, want := range []string{
		`role="textbox"`,
		`aria-label="To"`,
		`data-placeholder="recipient@example.com"`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("compose recipient field missing %q", want)
		}
	}
}
