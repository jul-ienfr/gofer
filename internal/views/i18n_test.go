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

func TestMailExportRendersFrench(t *testing.T) {
	var panel bytes.Buffer
	if err := MailExportPanel(i18n.French, MailExportPanelData{}).Render(context.Background(), &panel); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(panel.String(), "Ajoutez un compte de messagerie") {
		t.Fatalf("French export panel is missing its empty message: %s", panel.String())
	}

	form := MailExportFormData{
		Account: MailExportAccount{ID: "account-1", Email: "robin@example.test", Provider: "imap"},
		Folders: []MailExportFolder{{ID: "folder-1", Path: "Inbox", Role: "inbox", Messages: 3}},
	}
	var export bytes.Buffer
	if err := MailExportForm(i18n.French, form).Render(context.Background(), &export); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Dossiers", "3 messages", "Uniquement les messages enregistrés dans Gofer", "Lancer l"} {
		if !strings.Contains(export.String(), want) {
			t.Fatalf("French export form is missing %q", want)
		}
	}

	var jobs bytes.Buffer
	job := MailExportJob{ID: "job-1", Account: "robin@example.test", State: "done", Exported: 1, Started: "6 nov. 15:04"}
	if err := MailExportJobs(i18n.French, []MailExportJob{job}).Render(context.Background(), &jobs); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Exports", "Export terminé : 1 message", "Télécharger", "Supprimer"} {
		if !strings.Contains(jobs.String(), want) {
			t.Fatalf("French export jobs are missing %q", want)
		}
	}
}

func TestProfileTransferRendersFrench(t *testing.T) {
	var tab bytes.Buffer
	if err := SettingsExportImportTab(map[string]string{"language": "fr"}).Render(context.Background(), &tab); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Profil", "Exporter le profil", "Importer un profil", "Messages", "Exporter les messages"} {
		if !strings.Contains(tab.String(), want) {
			t.Fatalf("French profile card is missing %q", want)
		}
	}

	var export bytes.Buffer
	if err := ProfileExportForm(map[string]string{"language": "fr"}).Render(context.Background(), &export); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Inclure les contacts", "Exporter le profil"} {
		if !strings.Contains(export.String(), want) {
			t.Fatalf("French export form is missing %q", want)
		}
	}

	var upload bytes.Buffer
	if err := ProfileImportUploadForm(i18n.French, "").Render(context.Background(), &upload); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Fichier de profil", "Vérifier l"} {
		if !strings.Contains(upload.String(), want) {
			t.Fatalf("French upload form is missing %q", want)
		}
	}

	var result bytes.Buffer
	data := ProfileImportResultData{
		Preferences: true,
		Signatures:  1,
		Contacts:    2,
		Accounts:    []string{"robin@example.test"},
	}
	if err := ProfileImportResult(i18n.French, data).Render(context.Background(), &result); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Profil importé.", "1 signature ajoutée.", "2 contacts ajoutés.", "1 compte ajouté"} {
		if !strings.Contains(result.String(), want) {
			t.Fatalf("French result is missing %q", want)
		}
	}
}
