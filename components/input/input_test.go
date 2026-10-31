package input

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestPasswordToggleHasAccessibleNameAndState(t *testing.T) {
	var out bytes.Buffer
	if err := Input(Props{ID: "password", Name: "password", Type: TypePassword}).Render(context.Background(), &out); err != nil {
		t.Fatalf("Input.Render() error = %v", err)
	}
	html := out.String()
	for _, want := range []string{
		`data-tui-input-toggle-password="password"`,
		`aria-label="Show password"`,
		`aria-pressed="false"`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("password toggle missing %q", want)
		}
	}
}
