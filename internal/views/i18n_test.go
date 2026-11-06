package views

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/cristianadrielbraun/gofer/internal/i18n"
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

func TestThunderbirdImportRendersFrench(t *testing.T) {
	var upload bytes.Buffer
	if err := ThunderbirdImportUploadForm(i18n.French, "").Render(context.Background(), &upload); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"prefs.js de Thunderbird", "Vérifier l", "Aide"} {
		if !strings.Contains(upload.String(), want) {
			t.Fatalf("French upload form is missing %q", want)
		}
	}

	var result bytes.Buffer
	data := ThunderbirdImportResultData{Added: []string{"robin@example.test"}, Signatures: 2}
	if err := ThunderbirdImportResult(i18n.French, data).Render(context.Background(), &result); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"import Thunderbird est terminé.", "1 compte ajouté", "2 signatures ajoutées", "Aller aux comptes"} {
		if !strings.Contains(result.String(), want) {
			t.Fatalf("French result is missing %q", want)
		}
	}
}
