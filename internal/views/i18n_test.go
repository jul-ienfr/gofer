package views

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/cristianadrielbraun/gofer/internal/models"
)

func TestSettingsLayoutRendersSelectedLanguage(t *testing.T) {
	var output bytes.Buffer
	settings := map[string]string{"language": "fr"}
	if err := SettingsLayout(nil, models.SyncSettings{}, "regional", settings, nil, AISettingsData{}).Render(context.Background(), &output); err != nil {
		t.Fatal(err)
	}
	html := output.String()
	for _, want := range []string{`<html lang="fr"`, "Paramètres régionaux", "Langue de l", "Fuseau horaire", `value="fr"`} {
		if !strings.Contains(html, want) {
			t.Fatalf("French settings page is missing %q", want)
		}
	}
}

func TestSettingsLayoutFallsBackToEnglish(t *testing.T) {
	var output bytes.Buffer
	settings := map[string]string{"language": "de"}
	if err := SettingsLayout(nil, models.SyncSettings{}, "regional", settings, nil, AISettingsData{}).Render(context.Background(), &output); err != nil {
		t.Fatal(err)
	}
	html := output.String()
	for _, want := range []string{`<html lang="en"`, "Regional", "Interface language", `value="en"`} {
		if !strings.Contains(html, want) {
			t.Fatalf("English settings page is missing %q", want)
		}
	}
}
