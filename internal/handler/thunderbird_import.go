package handler

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"path"
	"strings"

	"github.com/cristianadrielbraun/gofer/internal/config"
	"github.com/cristianadrielbraun/gofer/internal/i18n"
	"github.com/cristianadrielbraun/gofer/internal/mail/message"
	mailtransport "github.com/cristianadrielbraun/gofer/internal/mail/transport"
	"github.com/cristianadrielbraun/gofer/internal/providers"
	"github.com/cristianadrielbraun/gofer/internal/storage"
	"github.com/cristianadrielbraun/gofer/internal/thunderbird"
	"github.com/cristianadrielbraun/gofer/internal/views"
)

const (
	thunderbirdUploadMaxBytes = 32 << 20
	thunderbirdFormMaxBytes   = thunderbird.MaxPrefsBytes + (512 << 10)
)

var (
	errThunderbirdZipRead    = errors.New("thunderbird: zip could not be read")
	errThunderbirdZipNoPrefs = errors.New("thunderbird: zip has no prefs.js")
	errThunderbirdZipEntry   = errors.New("thunderbird: zip prefs.js could not be read")
)

func (h *Handler) registerThunderbirdImportRoutes(register func(string, http.HandlerFunc)) {
	register("GET /api/thunderbird/import/upload", h.handleThunderbirdImportUpload)
	register("POST /api/thunderbird/import/preview", h.handleThunderbirdPreviewImport)
	register("POST /api/thunderbird/import", h.handleThunderbirdImport)
}

func (h *Handler) handleThunderbirdImportUpload(w http.ResponseWriter, r *http.Request) {
	locale := h.uiLocale(r.Context(), h.userID(r.Context()))
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = views.ThunderbirdImportUploadForm(locale, "").Render(r.Context(), w)
}

func renderThunderbirdImportError(w http.ResponseWriter, r *http.Request, locale i18n.Locale, message string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = views.ThunderbirdImportUploadForm(locale, message).Render(r.Context(), w)
}

func (h *Handler) handleThunderbirdPreviewImport(w http.ResponseWriter, r *http.Request) {
	ctx, owner := r.Context(), h.userID(r.Context())
	locale := h.uiLocale(ctx, owner)
	r.Body = http.MaxBytesReader(w, r.Body, thunderbirdUploadMaxBytes+(256<<10))
	if err := r.ParseMultipartForm(4 << 20); err != nil {
		renderThunderbirdImportError(w, r, locale, i18n.L(locale, "thunderbird.error.choose_file"))
		return
	}
	if r.MultipartForm != nil {
		defer r.MultipartForm.RemoveAll()
	}
	upload, header, err := r.FormFile("thunderbird")
	if err != nil {
		renderThunderbirdImportError(w, r, locale, i18n.L(locale, "thunderbird.error.choose_prefs"))
		return
	}
	defer upload.Close()
	data, err := io.ReadAll(io.LimitReader(upload, thunderbirdUploadMaxBytes+1))
	if err != nil || len(data) > thunderbirdUploadMaxBytes {
		renderThunderbirdImportError(w, r, locale, i18n.L(locale, "thunderbird.error.choose_file"))
		return
	}
	prefs, err := thunderbirdPrefsFromUpload(header.Filename, data)
	if err != nil {
		renderThunderbirdImportError(w, r, locale, thunderbirdErrorMessage(locale, err))
		return
	}
	profile, err := thunderbird.Parse(prefs)
	if err != nil {
		renderThunderbirdImportError(w, r, locale, thunderbirdErrorMessage(locale, err))
		return
	}
	items, warnings := h.thunderbirdAccounts(locale, profile)
	existing, err := h.profileExistingAccounts(ctx, owner)
	if err != nil {
		log.Printf("thunderbird preview: %v", err)
		renderThunderbirdImportError(w, r, locale, i18n.L(locale, "thunderbird.error.import_failed"))
		return
	}
	preview := views.ThunderbirdImportPreviewData{Prefs: string(prefs), Warnings: warnings}
	preview.Accounts = h.thunderbirdAccountViews(locale, items, existing)
	for i := range items {
		if preview.Accounts[i].Status == string(profileImportNew) {
			preview.Signatures += len(items[i].signatures)
		}
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_ = views.ThunderbirdImportPreview(locale, preview).Render(ctx, w)
}

func (h *Handler) handleThunderbirdImport(w http.ResponseWriter, r *http.Request) {
	ctx, owner := r.Context(), h.userID(r.Context())
	locale := h.uiLocale(ctx, owner)
	r.Body = http.MaxBytesReader(w, r.Body, thunderbirdFormMaxBytes)
	body, err := io.ReadAll(io.LimitReader(r.Body, thunderbirdFormMaxBytes+1))
	if err != nil || len(body) > thunderbirdFormMaxBytes {
		renderThunderbirdImportError(w, r, locale, i18n.L(locale, "thunderbird.error.read"))
		return
	}
	values, err := url.ParseQuery(string(body))
	if err != nil {
		renderThunderbirdImportError(w, r, locale, i18n.L(locale, "thunderbird.error.read"))
		return
	}
	profile, err := thunderbird.Parse([]byte(values.Get("prefs")))
	if err != nil {
		renderThunderbirdImportError(w, r, locale, thunderbirdErrorMessage(locale, err))
		return
	}
	items, _ := h.thunderbirdAccounts(locale, profile)
	selected := map[string]bool{}
	for _, ref := range values["account"] {
		selected[ref] = true
	}
	withSignatures := len(values["signatures"]) > 0
	result, err := h.importThunderbirdAccounts(ctx, owner, items, selected, withSignatures)
	if err != nil {
		log.Printf("thunderbird import: %v", err)
		renderThunderbirdImportError(w, r, locale, i18n.L(locale, "thunderbird.error.import_failed"))
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_ = views.ThunderbirdImportResult(locale, views.ThunderbirdImportResultData{
		Added: result.Added, Failed: result.Failed, Skipped: result.Skipped, Signatures: result.Signatures,
	}).Render(ctx, w)
}

func thunderbirdErrorMessage(locale i18n.Locale, err error) string {
	switch {
	case errors.Is(err, thunderbird.ErrTooLarge):
		return i18n.L(locale, "thunderbird.error.prefs_too_large")
	case errors.Is(err, thunderbird.ErrNotPrefs):
		return i18n.L(locale, "thunderbird.error.not_prefs")
	case errors.Is(err, thunderbird.ErrNoAccounts):
		return i18n.L(locale, "thunderbird.error.no_accounts")
	case errors.Is(err, errThunderbirdZipRead):
		return i18n.L(locale, "thunderbird.error.zip_read")
	case errors.Is(err, errThunderbirdZipNoPrefs):
		return i18n.L(locale, "thunderbird.error.zip_no_prefs")
	case errors.Is(err, errThunderbirdZipEntry):
		return i18n.L(locale, "thunderbird.error.zip_entry")
	}
	return i18n.L(locale, "thunderbird.error.import_failed")
}

func thunderbirdPrefsFromUpload(name string, data []byte) ([]byte, error) {
	if isZipArchive(data) || strings.EqualFold(path.Ext(name), ".zip") {
		return thunderbirdPrefsFromZip(data)
	}
	if len(data) > thunderbird.MaxPrefsBytes {
		return nil, thunderbird.ErrTooLarge
	}
	return data, nil
}

func isZipArchive(data []byte) bool {
	return len(data) >= 4 && bytes.Equal(data[:4], []byte{'P', 'K', 3, 4})
}

func thunderbirdPrefsFromZip(data []byte) ([]byte, error) {
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, errThunderbirdZipRead
	}
	var found *zip.File
	var foundPath string
	for _, file := range reader.File {
		if file.FileInfo().IsDir() {
			continue
		}
		name := strings.ReplaceAll(file.Name, `\`, "/")
		if !strings.EqualFold(path.Base(name), "prefs.js") {
			continue
		}
		if found == nil || strings.Count(name, "/") < strings.Count(foundPath, "/") {
			found, foundPath = file, name
		}
	}
	if found == nil {
		return nil, errThunderbirdZipNoPrefs
	}
	if found.UncompressedSize64 > thunderbird.MaxPrefsBytes {
		return nil, thunderbird.ErrTooLarge
	}
	entry, err := found.Open()
	if err != nil {
		return nil, errThunderbirdZipEntry
	}
	defer entry.Close()
	prefs, err := io.ReadAll(io.LimitReader(entry, thunderbird.MaxPrefsBytes+1))
	if err != nil || len(prefs) > thunderbird.MaxPrefsBytes {
		return nil, thunderbird.ErrTooLarge
	}
	return prefs, nil
}

type thunderbirdAccount struct {
	account    profileAccount
	signatures []profileSignature
	warning    string
}

func (h *Handler) thunderbirdAccounts(locale i18n.Locale, profile *thunderbird.Profile) ([]thunderbirdAccount, []string) {
	var items []thunderbirdAccount
	var warnings []string
	seen := map[string]bool{}
	for _, warning := range profile.Warnings {
		warnings = append(warnings, thunderbirdProfileWarning(locale, warning))
	}
	for _, source := range profile.Accounts {
		email := profile.AccountEmail(source)
		label := email
		if label == "" {
			label = i18n.L(locale, "thunderbird.account_fallback")
		}
		serverType := strings.ToLower(strings.TrimSpace(source.Server.Type))
		switch serverType {
		case "imap":
		case "":
			if strings.TrimSpace(source.Server.Host) == "" {
				warnings = append(warnings, i18n.Lf(locale, "thunderbird.warning.no_server", label))
				continue
			}
		case "none":
			continue
		default:
			warnings = append(warnings, i18n.Lf(locale, "thunderbird.warning.unsupported_type", label, strings.ToUpper(serverType)))
			continue
		}
		if email == "" {
			warnings = append(warnings, i18n.L(locale, "thunderbird.warning.no_email"))
			continue
		}
		if len(source.Identities) == 0 {
			continue
		}
		key := strings.ToLower(email)
		if seen[key] {
			warnings = append(warnings, i18n.Lf(locale, "thunderbird.warning.duplicate", label))
			continue
		}
		seen[key] = true

		provider := providers.ProviderIMAP
		warning := ""
		host := thunderbirdHost(source.Server.Host)
		switch {
		case thunderbirdGmailHost(host):
			if h.profileProviderAvailable(providers.ProviderGmail) {
				provider = providers.ProviderGmail
			} else {
				warning = i18n.Lf(locale, "thunderbird.warning.gmail_no_oauth", label)
			}
		case thunderbirdOutlookHost(host):
			if h.profileProviderAvailable(providers.ProviderOutlook) {
				provider = providers.ProviderOutlook
			} else {
				warning = i18n.Lf(locale, "thunderbird.warning.outlook_no_oauth", label)
			}
		}

		imapTLS := thunderbirdTLSMode(source.Server.SocketType, source.Server.Port)
		imapPort := source.Server.Port
		if imapPort == 0 {
			imapPort = thunderbirdDefaultPort(true, imapTLS)
		}
		username := strings.TrimSpace(source.Server.Username)
		if username == "" {
			username = email
		}
		converted := profileAccount{
			Ref:          fmt.Sprintf("@account:%d@", len(items)+1),
			Provider:     provider,
			EmailAddress: email,
			DisplayName:  strings.TrimSpace(source.Identities[0].Name),
			IMAPHost:     strings.TrimSpace(source.Server.Host),
			IMAPPort:     imapPort,
			IMAPTLSMode:  imapTLS,
			Username:     username,
			AuthMethod:   "plain",
			EmailSync:    true,
		}
		if smtp := thunderbirdPrimarySMTP(source); smtp != nil {
			smtpTLS := thunderbirdTLSMode(smtp.SocketType, smtp.Port)
			smtpPort := smtp.Port
			if smtpPort == 0 {
				smtpPort = thunderbirdDefaultPort(false, smtpTLS)
			}
			converted.SMTPHost = strings.TrimSpace(smtp.Host)
			converted.SMTPPort = smtpPort
			converted.SMTPTLSMode = smtpTLS
			converted.SMTPUsername = strings.TrimSpace(smtp.Username)
			if converted.SMTPUsername == "" {
				converted.SMTPUsername = email
			}
		} else {
			warning = joinThunderbirdWarning(warning, i18n.Lf(locale, "thunderbird.warning.no_smtp", label))
		}

		signatureRefs := make([]string, len(source.Identities))
		var signatures []profileSignature
		for i, identity := range source.Identities {
			if !identity.SignatureKnown {
				continue
			}
			signature := profileSignature{}
			if identity.SignatureIsHTML {
				signature.HTMLBody = strings.TrimSpace(string(message.SanitizeHTML([]byte(identity.Signature))))
				if signature.HTMLBody != "" {
					signature.TextBody = signatureHTMLToText(signature.HTMLBody)
				}
			} else {
				signature.TextBody = strings.TrimSpace(identity.Signature)
				if signature.TextBody != "" {
					signature.HTMLBody = plainTextToSignatureHTML(signature.TextBody)
				}
			}
			if signature.HTMLBody == "" && signature.TextBody == "" {
				continue
			}
			ref := fmt.Sprintf("@signature:%d:%d@", len(items)+1, i+1)
			signatureRefs[i] = ref
			signature.Ref = ref
			name := strings.TrimSpace(identity.Name)
			if name == "" {
				name = strings.TrimSpace(identity.Email)
			}
			signature.Name = name
			signatures = append(signatures, signature)
		}
		if primarySignature := signatureRefs[0]; primarySignature != "" {
			identity := source.Identities[0]
			placement := "before"
			if identity.SignatureBelow {
				placement = "after"
			}
			converted.Signatures = &profileAccountSignatures{
				New:              profileSignatureUse{Signature: primarySignature, Enabled: identity.SignatureOnNew},
				Reply:            profileSignatureUse{Signature: primarySignature, Enabled: identity.SignatureOnReply},
				Forward:          profileSignatureUse{Signature: primarySignature, Enabled: identity.SignatureOnForward},
				ReplyPlacement:   placement,
				ForwardPlacement: placement,
			}
		}

		for i, identity := range source.Identities[1:] {
			identityEmail := strings.TrimSpace(identity.Email)
			if identityEmail == "" || strings.EqualFold(identityEmail, email) {
				continue
			}
			alias := profileIdentity{Name: strings.TrimSpace(identity.Name), Email: identityEmail, ReplyTo: strings.TrimSpace(identity.ReplyTo), Signature: signatureRefs[i+1]}
			differentHost := identity.SMTP != nil && strings.TrimSpace(identity.SMTP.Host) != "" && !strings.EqualFold(strings.TrimSpace(identity.SMTP.Host), converted.SMTPHost)
			differentUsername := identity.SMTP != nil && strings.TrimSpace(identity.SMTP.Username) != "" && !strings.EqualFold(strings.TrimSpace(identity.SMTP.Username), converted.SMTPUsername)
			if differentHost || differentUsername {
				aliasTLS := thunderbirdTLSMode(identity.SMTP.SocketType, identity.SMTP.Port)
				if aliasTLS != mailtransport.TLSModePlaintext {
					aliasPort := identity.SMTP.Port
					if aliasPort == 0 {
						aliasPort = thunderbirdDefaultPort(false, aliasTLS)
					}
					aliasUser := strings.TrimSpace(identity.SMTP.Username)
					if aliasUser == "" {
						aliasUser = identityEmail
					}
					alias.SMTP = &profileIdentitySMTP{
						Host: strings.TrimSpace(identity.SMTP.Host), Port: aliasPort, TLSMode: aliasTLS, Username: aliasUser,
					}
				}
			}
			converted.Identities = append(converted.Identities, alias)
		}

		items = append(items, thunderbirdAccount{account: converted, signatures: signatures, warning: warning})
	}
	return items, warnings
}

func thunderbirdProfileWarning(locale i18n.Locale, warning thunderbird.Warning) string {
	switch warning.Code {
	case "no_server":
		return i18n.L(locale, "thunderbird.warning.account_no_server")
	case "no_email":
		return i18n.L(locale, "thunderbird.warning.account_no_email")
	}
	return warning.Message
}

func (h *Handler) thunderbirdAccountViews(locale i18n.Locale, items []thunderbirdAccount, existing map[string]profileExistingAccount) []views.ThunderbirdImportAccount {
	accounts := make([]profileAccount, len(items))
	for i := range items {
		accounts[i] = items[i].account
	}
	plans := h.profileAccountPlansFor(accounts, existing)
	out := make([]views.ThunderbirdImportAccount, 0, len(items))
	for _, item := range items {
		status := string(plans[item.account.Ref].Status)
		warning := item.warning
		if thunderbirdPlaintext(item.account) {
			status = string(profileImportUnsupported)
			warning = joinThunderbirdWarning(warning, i18n.L(locale, "thunderbird.warning.plaintext"))
		}
		var needs []string
		if status == string(profileImportNew) {
			needs = profileImportPending(item.account).Missing()
		}
		out = append(out, views.ThunderbirdImportAccount{
			Ref:      item.account.Ref,
			Email:    item.account.EmailAddress,
			Name:     item.account.DisplayName,
			Provider: item.account.Provider,
			Host:     item.account.IMAPHost,
			Status:   status,
			Needs:    needs,
			Warning:  warning,
		})
	}
	return out
}

type thunderbirdImportResult struct {
	Added      []string
	Failed     []string
	Skipped    int
	Signatures int
}

func (h *Handler) importThunderbirdAccounts(ctx context.Context, owner string, items []thunderbirdAccount, selected map[string]bool, withSignatures bool) (thunderbirdImportResult, error) {
	var result thunderbirdImportResult
	accounts := make([]profileAccount, len(items))
	for i := range items {
		accounts[i] = items[i].account
	}
	existing, err := h.profileExistingAccounts(ctx, owner)
	if err != nil {
		return result, err
	}
	plans := h.profileAccountPlansFor(accounts, existing)
	eligible := make(map[string]bool, len(items))
	for _, item := range items {
		if selected[item.account.Ref] && plans[item.account.Ref].Status == profileImportNew && !thunderbirdPlaintext(item.account) {
			eligible[item.account.Ref] = true
		}
	}
	signatureIDs := map[string]string{}
	if withSignatures {
		var signatures []profileSignature
		for _, item := range items {
			if eligible[item.account.Ref] {
				signatures = append(signatures, item.signatures...)
			}
		}
		if len(signatures) > 0 {
			if err := h.withProfileStore(ctx, owner, func(_ *config.AccountStore, db *storage.DB) error {
				added, err := importProfileSignatures(ctx, db, owner, signatures, signatureIDs)
				result.Signatures = added
				return err
			}); err != nil {
				return result, err
			}
		}
	}
	for _, item := range items {
		if !eligible[item.account.Ref] {
			if selected[item.account.Ref] {
				result.Skipped++
			}
			continue
		}
		if _, err := h.importProfileAccount(ctx, owner, item.account, signatureIDs); err != nil {
			log.Printf("thunderbird import: account %s: %v", profileAccountName(item.account), err)
			result.Failed = append(result.Failed, profileAccountName(item.account))
			continue
		}
		result.Added = append(result.Added, profileAccountName(item.account))
	}
	return result, nil
}

func thunderbirdPrimarySMTP(account thunderbird.Account) *thunderbird.SMTPServer {
	for _, identity := range account.Identities {
		if identity.SMTP != nil && strings.TrimSpace(identity.SMTP.Host) != "" {
			return identity.SMTP
		}
	}
	return nil
}

func thunderbirdPlaintext(account profileAccount) bool {
	if account.Provider != providers.ProviderIMAP {
		return false
	}
	return account.IMAPTLSMode == mailtransport.TLSModePlaintext || account.SMTPTLSMode == mailtransport.TLSModePlaintext
}

func thunderbirdTLSMode(socketType, port int) string {
	switch socketType {
	case 0:
		return mailtransport.TLSModePlaintext
	case 1, 3:
		return mailtransport.TLSModeStartTLS
	case 2:
		return mailtransport.TLSModeImplicit
	}
	if port == 993 || port == 465 {
		return mailtransport.TLSModeImplicit
	}
	return mailtransport.TLSModeStartTLS
}

func thunderbirdDefaultPort(imap bool, mode string) int {
	if imap {
		if mode == mailtransport.TLSModeImplicit {
			return 993
		}
		return 143
	}
	switch mode {
	case mailtransport.TLSModeImplicit:
		return 465
	case mailtransport.TLSModePlaintext:
		return 25
	default:
		return 587
	}
}

func thunderbirdHost(host string) string {
	return strings.TrimSuffix(strings.ToLower(strings.TrimSpace(host)), ".")
}

func thunderbirdGmailHost(host string) bool {
	return host == "imap.gmail.com"
}

func thunderbirdOutlookHost(host string) bool {
	return host == "outlook.office365.com" || host == "imap-mail.outlook.com" || strings.HasSuffix(host, ".outlook.office365.com")
}

func joinThunderbirdWarning(current, next string) string {
	if current == "" {
		return next
	}
	return current + " " + next
}
