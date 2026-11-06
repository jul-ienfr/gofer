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

func TestMailImportRendersFrench(t *testing.T) {
	var upload bytes.Buffer
	if err := MailImportUploadForm(i18n.French, "").Render(context.Background(), &upload); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Fichiers", "Importer et vérifier", "plusieurs fichiers"} {
		if !strings.Contains(upload.String(), want) {
			t.Fatalf("French import upload form is missing %q", want)
		}
	}

	review := MailImportReviewData{
		ID:        "import-1",
		Files:     []string{"mailbox.mbox"},
		AccountID: "account-1",
		Accounts:  []MailImportAccount{{MailExportAccount: MailExportAccount{ID: "account-1", Email: "robin@example.test", Provider: "imap"}}},
		Folders:   []MailImportFolder{{ID: "folder-1", Path: "Inbox"}},
		Sources:   []MailImportSource{{Index: 0, Name: "Inbox", Messages: 2, Target: "folder-1"}},
	}
	var reviewHTML bytes.Buffer
	if err := MailImportReview(i18n.French, review).Render(context.Background(), &reviewHTML); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Dans le compte", "Destination des messages", "2 messages dans 1 dossier", "Lancer l"} {
		if !strings.Contains(reviewHTML.String(), want) {
			t.Fatalf("French import review is missing %q", want)
		}
	}

	var jobs bytes.Buffer
	job := MailImportJob{ID: "import-1", Title: "mailbox.mbox", State: "done", Imported: 2, Started: "6 nov. 15:04"}
	if err := MailImportJobs(i18n.French, []MailImportJob{job}).Render(context.Background(), &jobs); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Imports", "Import terminé : 2 messages", "Retirer"} {
		if !strings.Contains(jobs.String(), want) {
			t.Fatalf("French import jobs are missing %q", want)
		}
	}
}

func TestMailTransferRendersFrench(t *testing.T) {
	form := MailTransferPanelData{Form: MailTransferFormData{
		Sources:     []MailImportAccount{{MailExportAccount: MailExportAccount{ID: "source-1", Email: "robin@example.test"}}, {MailExportAccount: MailExportAccount{ID: "target-1", Email: "juno@example.test"}}},
		Targets:     []MailImportAccount{{MailExportAccount: MailExportAccount{ID: "target-1", Email: "juno@example.test"}}},
		SourceID:    "source-1",
		TargetID:    "target-1",
		SourceEmail: "robin@example.test",
		Mode:        "copy",
		Folders:     []MailTransferFolder{{ID: "folder-1", Path: "Inbox", Messages: 2, Target: "folder-2"}},
		Destination: []MailImportFolder{{ID: "folder-2", Path: "Archives"}},
	}}
	var panel bytes.Buffer
	if err := MailTransferPanel(i18n.French, form).Render(context.Background(), &panel); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Depuis", "Vers", "Copier", "Déplacer", "Dossiers et destination", "Lancer"} {
		if !strings.Contains(panel.String(), want) {
			t.Fatalf("French transfer panel is missing %q", want)
		}
	}

	var jobs bytes.Buffer
	job := MailTransferJob{ID: "transfer-1", Source: "robin@example.test", Target: "juno@example.test", State: "done", Copied: 2, Move: true, Deleted: 1, Started: "6 nov. 15:04"}
	if err := MailTransferJobs(i18n.French, []MailTransferJob{job}).Render(context.Background(), &jobs); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Transferts", "Copie terminée : 2 messages", "Suppression de robin@example.test : 1 message", "Retirer"} {
		if !strings.Contains(jobs.String(), want) {
			t.Fatalf("French transfer jobs are missing %q", want)
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
