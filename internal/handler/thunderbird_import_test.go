package handler

import (
	"archive/zip"
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/cristianadrielbraun/gofer/internal/auth"
	"github.com/cristianadrielbraun/gofer/internal/i18n"
	"github.com/cristianadrielbraun/gofer/internal/providers"
	"github.com/cristianadrielbraun/gofer/internal/thunderbird"
)

const thunderbirdTestPrefs = `
user_pref("mail.accountmanager.accounts", "account1,account2,account3");
user_pref("mail.account.account1.server", "server1");
user_pref("mail.account.account1.identities", "id1,id2");
user_pref("mail.account.account2.server", "server2");
user_pref("mail.account.account2.identities", "id3");
user_pref("mail.account.account3.server", "server3");
user_pref("mail.account.account3.identities", "id4");
user_pref("mail.identity.id1.useremail", "robin@example.test");
user_pref("mail.identity.id1.fullName", "Robin Example");
user_pref("mail.identity.id1.smtpServer", "smtp1");
user_pref("mail.identity.id1.htmlSigText", "<p>Robin</p><script>alert(1)</script>");
user_pref("mail.identity.id1.htmlSigFormat", true);
user_pref("mail.identity.id2.useremail", "sales@example.test");
user_pref("mail.identity.id2.fullName", "Robin Sales");
user_pref("mail.identity.id2.smtpServer", "smtp1");
user_pref("mail.identity.id2.htmlSigText", "Sales signature");
user_pref("mail.identity.id3.useremail", "old@example.test");
user_pref("mail.server.server1.type", "imap");
user_pref("mail.server.server1.hostname", "imap.example.test");
user_pref("mail.server.server1.port", 993);
user_pref("mail.server.server1.socketType", 2);
user_pref("mail.server.server1.userName", "robin");
user_pref("mail.server.server2.type", "pop3");
user_pref("mail.server.server2.hostname", "pop.example.test");
user_pref("mail.server.server3.type", "none");
user_pref("mail.smtpservers", "smtp1");
user_pref("mail.smtpserver.smtp1.hostname", "smtp.example.test");
user_pref("mail.smtpserver.smtp1.port", 587);
user_pref("mail.smtpserver.smtp1.socketType", 3);
user_pref("mail.smtpserver.smtp1.username", "robin-smtp");
`

func parseThunderbirdTestProfile(t *testing.T, prefs string) *thunderbird.Profile {
	t.Helper()
	profile, err := thunderbird.Parse([]byte(prefs))
	if err != nil {
		t.Fatalf("thunderbird.Parse() error = %v", err)
	}
	return profile
}

func thunderbirdRequest(req *http.Request) *http.Request {
	ctx := auth.ContextWithUser(req.Context(), &auth.User{ID: "default", Username: "default"})
	return req.WithContext(ctx)
}

func TestThunderbirdImportAddsAccountWaitingForReconnect(t *testing.T) {
	h := newProfileTestHandler(t)
	items, warnings := h.thunderbirdAccounts(i18n.English, parseThunderbirdTestProfile(t, thunderbirdTestPrefs))
	if len(items) != 1 || len(warnings) != 1 || !strings.Contains(warnings[0], "POP3") {
		t.Fatalf("items = %+v, warnings = %v", items, warnings)
	}
	item := items[0]
	if item.account.Provider != providers.ProviderIMAP || item.account.EmailAddress != "robin@example.test" || item.account.DisplayName != "Robin Example" {
		t.Fatalf("converted account = %+v", item.account)
	}
	if item.account.IMAPHost != "imap.example.test" || item.account.IMAPPort != 993 || item.account.IMAPTLSMode != "tls" || item.account.Username != "robin" {
		t.Fatalf("IMAP settings = %+v", item.account)
	}
	if item.account.SMTPHost != "smtp.example.test" || item.account.SMTPPort != 587 || item.account.SMTPTLSMode != "starttls" || item.account.SMTPUsername != "robin-smtp" {
		t.Fatalf("SMTP settings = %+v", item.account)
	}
	if len(item.account.Identities) != 1 || item.account.Identities[0].Email != "sales@example.test" {
		t.Fatalf("aliases = %+v", item.account.Identities)
	}
	if len(item.signatures) != 2 || item.account.Signatures == nil || !item.account.Signatures.New.Enabled || !item.account.Signatures.Reply.Enabled || item.account.Signatures.Forward.Enabled || item.account.Signatures.ReplyPlacement != "before" {
		t.Fatalf("signatures = %+v, settings = %+v", item.signatures, item.account.Signatures)
	}
	if item.account.Identities[0].Signature != item.signatures[1].Ref {
		t.Fatalf("alias signature = %q, signatures = %+v", item.account.Identities[0].Signature, item.signatures)
	}

	result, err := h.importThunderbirdAccounts(t.Context(), "default", items, map[string]bool{item.account.Ref: true}, true)
	if err != nil {
		t.Fatalf("importThunderbirdAccounts() error = %v", err)
	}
	if len(result.Added) != 1 || result.Added[0] != "robin@example.test" || len(result.Failed) != 0 || result.Skipped != 0 || result.Signatures != 2 {
		t.Fatalf("result = %+v", result)
	}

	accounts, err := h.db.GetAccounts(t.Context(), "default")
	if err != nil || len(accounts) != 1 {
		t.Fatalf("GetAccounts() = %v, %v", accounts, err)
	}
	account := accounts[0]
	if account.Provider != providers.ProviderIMAP || !account.ReconnectPending || account.EmailSyncEnabled {
		t.Fatalf("imported account = %+v", account)
	}
	cfg, err := h.accountStore.GetConfig(t.Context(), account.ID)
	if err != nil {
		t.Fatalf("GetConfig() error = %v", err)
	}
	if cfg.IMAPHost != "imap.example.test" || cfg.IMAPPort != 993 || cfg.IMAPTLSMode != "tls" || cfg.Username != "robin" {
		t.Fatalf("stored IMAP settings = %+v", cfg)
	}
	if cfg.SMTPHost != "smtp.example.test" || cfg.SMTPPort != 587 || cfg.SMTPTLSMode != "starttls" || cfg.SmtpUsername != "robin-smtp" {
		t.Fatalf("stored SMTP settings = %+v", cfg)
	}
	if password, err := h.accountStore.DecryptPassword(t.Context(), account.ID); err != nil || password != "" {
		t.Fatalf("imported password = %q, %v", password, err)
	}
	identities, err := h.db.ListAccountIdentities(t.Context(), "default", account.ID)
	if err != nil || len(identities) != 1 || identities[0].Email != "sales@example.test" || identities[0].Name != "Robin Sales" {
		t.Fatalf("identities = %+v, %v", identities, err)
	}
	edit, err := h.accountStore.GetEditData(t.Context(), account.ID)
	if err != nil {
		t.Fatalf("GetEditData() error = %v", err)
	}
	if len(edit.Signatures) != 2 || edit.SignatureSettings.NewSignatureID == "" || !edit.SignatureSettings.NewEnabled || !edit.SignatureSettings.ReplyEnabled || edit.SignatureSettings.ForwardEnabled {
		t.Fatalf("signatures = %+v, settings = %+v", edit.Signatures, edit.SignatureSettings)
	}
	for _, signature := range edit.Signatures {
		if strings.Contains(strings.ToLower(signature.HTMLBody), "<script") || signature.TextBody == "" {
			t.Fatalf("signature = %+v", signature)
		}
	}
	if identities[0].SignatureID == "" || identities[0].SignatureID == edit.SignatureSettings.NewSignatureID {
		t.Fatalf("alias signature = %q, settings = %+v", identities[0].SignatureID, edit.SignatureSettings)
	}

	again, err := h.importThunderbirdAccounts(t.Context(), "default", items, map[string]bool{item.account.Ref: true}, true)
	if err != nil {
		t.Fatalf("second import error = %v", err)
	}
	if len(again.Added) != 0 || again.Skipped != 1 || again.Signatures != 0 {
		t.Fatalf("second import result = %+v", again)
	}
}

func TestThunderbirdPreviewMarksPlaintextAccountUnsupported(t *testing.T) {
	h := newProfileTestHandler(t)
	prefs := strings.NewReplacer(
		`user_pref("mail.server.server1.socketType", 2);`, `user_pref("mail.server.server1.socketType", 0);`,
		`user_pref("mail.smtpserver.smtp1.socketType", 3);`, `user_pref("mail.smtpserver.smtp1.socketType", 0);`,
	).Replace(thunderbirdTestPrefs)
	items, _ := h.thunderbirdAccounts(i18n.English, parseThunderbirdTestProfile(t, prefs))
	views := h.thunderbirdAccountViews(i18n.English, items, nil)
	if len(views) != 1 || views[0].Status != "unsupported" || !strings.Contains(views[0].Warning, "unencrypted") {
		t.Fatalf("views = %+v", views)
	}
	result, err := h.importThunderbirdAccounts(t.Context(), "default", items, map[string]bool{items[0].account.Ref: true}, true)
	if err != nil {
		t.Fatalf("importThunderbirdAccounts() error = %v", err)
	}
	if len(result.Added) != 0 || result.Skipped != 1 || result.Signatures != 0 {
		t.Fatalf("result = %+v", result)
	}
	accounts, err := h.db.GetAccounts(t.Context(), "default")
	if err != nil || len(accounts) != 0 {
		t.Fatalf("accounts = %v, %v", accounts, err)
	}
}

func TestThunderbirdImportFindsSMTPOnAnIdentity(t *testing.T) {
	h := newProfileTestHandler(t)
	prefs := strings.Replace(thunderbirdTestPrefs, `user_pref("mail.identity.id1.smtpServer", "smtp1");`, "", 1)
	items, _ := h.thunderbirdAccounts(i18n.English, parseThunderbirdTestProfile(t, prefs))
	if len(items) != 1 || items[0].account.SMTPHost != "smtp.example.test" || items[0].account.SMTPPort != 587 {
		t.Fatalf("items = %+v", items)
	}
}

func TestThunderbirdImportCanSkipSignatures(t *testing.T) {
	h := newProfileTestHandler(t)
	items, _ := h.thunderbirdAccounts(i18n.English, parseThunderbirdTestProfile(t, thunderbirdTestPrefs))
	result, err := h.importThunderbirdAccounts(t.Context(), "default", items, map[string]bool{items[0].account.Ref: true}, false)
	if err != nil {
		t.Fatalf("importThunderbirdAccounts() error = %v", err)
	}
	if len(result.Added) != 1 || result.Signatures != 0 {
		t.Fatalf("result = %+v", result)
	}
	signatures, err := h.db.ListSignatures(t.Context(), "default")
	if err != nil || len(signatures) != 0 {
		t.Fatalf("signatures = %+v, %v", signatures, err)
	}
	accounts, err := h.db.GetAccounts(t.Context(), "default")
	if err != nil || len(accounts) != 1 {
		t.Fatalf("accounts = %v, %v", accounts, err)
	}
	identities, err := h.db.ListAccountIdentities(t.Context(), "default", accounts[0].ID)
	if err != nil || len(identities) != 1 || identities[0].SignatureID != "" {
		t.Fatalf("identities = %+v, %v", identities, err)
	}
	edit, err := h.accountStore.GetEditData(t.Context(), accounts[0].ID)
	if err != nil || edit.SignatureSettings.NewSignatureID != "" || edit.SignatureSettings.NewEnabled {
		t.Fatalf("signature settings = %+v, %v", edit.SignatureSettings, err)
	}
}

func TestThunderbirdGmailFallsBackToIMAPWithoutSignIn(t *testing.T) {
	h := newProfileTestHandler(t)
	prefs := strings.Replace(thunderbirdTestPrefs, "imap.example.test", "imap.gmail.com", 1)
	prefs = strings.Replace(prefs, "smtp.example.test", "smtp.gmail.com", 1)
	items, _ := h.thunderbirdAccounts(i18n.English, parseThunderbirdTestProfile(t, prefs))
	if len(items) != 1 || items[0].account.Provider != providers.ProviderIMAP {
		t.Fatalf("items = %+v", items)
	}
	if !strings.Contains(items[0].warning, "Google sign-in") {
		t.Fatalf("warning = %q", items[0].warning)
	}
}

func TestThunderbirdPrefsFromUploadReadsZip(t *testing.T) {
	var archive bytes.Buffer
	writer := zip.NewWriter(&archive)
	entry, err := writer.Create("profile/prefs.js")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := entry.Write([]byte(`user_pref("mail.accountmanager.accounts", "account1");`)); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	prefs, err := thunderbirdPrefsFromUpload("Thunderbird profile.zip", archive.Bytes())
	if err != nil {
		t.Fatalf("thunderbirdPrefsFromUpload() error = %v", err)
	}
	if !strings.Contains(string(prefs), "mail.accountmanager.accounts") {
		t.Fatalf("prefs = %q", prefs)
	}
	if _, err := thunderbirdPrefsFromUpload("empty.zip", []byte("PK\x03\x04broken")); err == nil {
		t.Fatal("broken ZIP was accepted")
	}
}

func TestThunderbirdPrefsFromUploadReadsZipWithWindowsPaths(t *testing.T) {
	var archive bytes.Buffer
	writer := zip.NewWriter(&archive)
	entry, err := writer.Create(`Thunderbird Profile\prefs.js`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := entry.Write([]byte(`user_pref("mail.accountmanager.accounts", "account1");`)); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	prefs, err := thunderbirdPrefsFromUpload("profile.zip", archive.Bytes())
	if err != nil {
		t.Fatalf("thunderbirdPrefsFromUpload() error = %v", err)
	}
	if !strings.Contains(string(prefs), "mail.accountmanager.accounts") {
		t.Fatalf("prefs = %q", prefs)
	}
}

func TestThunderbirdPreviewRouteListsAccounts(t *testing.T) {
	h := newProfileTestHandler(t)
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("thunderbird", "prefs.js")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write([]byte(thunderbirdTestPrefs)); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/thunderbird/import/preview", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	recorder := httptest.NewRecorder()
	h.handleThunderbirdPreviewImport(recorder, thunderbirdRequest(req))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	if html := recorder.Body.String(); !strings.Contains(html, "robin@example.test") || !strings.Contains(html, "Ready to import") || !strings.Contains(html, "2 signatures") {
		t.Fatalf("preview = %s", html)
	}
}

func TestThunderbirdPreviewRouteUsesFrench(t *testing.T) {
	h := newProfileTestHandler(t)
	if err := h.db.SetUISettings(t.Context(), "default", map[string]string{"language": "fr"}); err != nil {
		t.Fatal(err)
	}
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("thunderbird", "prefs.js")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write([]byte(thunderbirdTestPrefs)); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/thunderbird/import/preview", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	recorder := httptest.NewRecorder()
	h.handleThunderbirdPreviewImport(recorder, thunderbirdRequest(req))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	html := recorder.Body.String()
	for _, want := range []string{"Comptes", "Prêt à importer", "2 signatures", "Annuler", "Importer"} {
		if !strings.Contains(html, want) {
			t.Fatalf("preview missing %q: %s", want, html)
		}
	}
	for _, unwanted := range []string{"Ready to import", "Cancel", "Review import"} {
		if strings.Contains(html, unwanted) {
			t.Fatalf("preview leaked English %q: %s", unwanted, html)
		}
	}
}

func TestThunderbirdImportRouteCreatesAccount(t *testing.T) {
	h := newProfileTestHandler(t)
	items, _ := h.thunderbirdAccounts(i18n.English, parseThunderbirdTestProfile(t, thunderbirdTestPrefs))
	form := url.Values{"prefs": {thunderbirdTestPrefs}, "account": {items[0].account.Ref}, "signatures": {"1"}}
	req := httptest.NewRequest(http.MethodPost, "/api/thunderbird/import", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	h.handleThunderbirdImport(recorder, thunderbirdRequest(req))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	if html := recorder.Body.String(); !strings.Contains(html, "robin@example.test") || !strings.Contains(html, "No passwords were imported") || !strings.Contains(html, "2 signatures added") {
		t.Fatalf("result = %s", html)
	}
	accounts, err := h.db.GetAccounts(t.Context(), "default")
	if err != nil || len(accounts) != 1 {
		t.Fatalf("accounts = %v, %v", accounts, err)
	}
}
