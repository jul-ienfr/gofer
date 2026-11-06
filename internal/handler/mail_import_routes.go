package handler

import (
	"errors"
	"log"
	"net/http"
	"strings"

	"github.com/cristianadrielbraun/gofer/internal/i18n"
	"github.com/cristianadrielbraun/gofer/internal/models"
	"github.com/cristianadrielbraun/gofer/internal/views"
)

func (h *Handler) registerMailImportRoutes(register func(string, http.HandlerFunc)) {
	register("GET /api/mail/imports", h.handleMailImports)
	register("GET /api/mail/imports/jobs", h.handleMailImportJobs)
	register("POST /api/mail/imports", h.handleUploadMailImport)
	register("GET /api/mail/imports/{id}/review", h.handleReviewMailImport)
	register("POST /api/mail/imports/{id}/start", h.handleStartMailImport)
	register("POST /api/mail/imports/{id}/cancel", h.handleCancelMailImport)
	register("POST /api/mail/imports/{id}/resume", h.handleResumeMailImport)
	register("POST /api/mail/imports/{id}/delete", h.handleDeleteMailImport)
}

func (h *Handler) mailImportsAvailable(w http.ResponseWriter) bool {
	if h.mailImports == nil {
		http.Error(w, "mail import is not available", http.StatusServiceUnavailable)
		return false
	}
	return true
}

func (h *Handler) mailImportJobViews(owner string) ([]views.MailImportJob, error) {
	jobs, err := h.mailImports.jobs(owner)
	if err != nil {
		return nil, err
	}
	result := make([]views.MailImportJob, 0, len(jobs))
	for _, job := range jobs {
		processed, total, imported, duplicates, failed := job.totals()
		view := views.MailImportJob{
			ID: job.ID, Title: mailImportTitle(job), Account: job.AccountEmail, State: job.State, Error: job.Error,
			Processed: processed, Total: total, Imported: imported, Duplicates: duplicates, Failed: failed,
			Started: job.CreatedAt.Local().Format("Jan 2, 15:04"),
		}
		if job.State == mailJobRunning && h.mailImports.isWaiting(job.ID) {
			view.Waiting = true
		} else if source := job.current(); source != nil {
			view.Source = source.Name
		}
		for _, skipped := range job.Skipped {
			subject := skipped.Subject
			if subject == "" {
				subject = "(no subject)"
			}
			view.Skipped = append(view.Skipped, skipped.Source+": "+subject+" ("+skipped.Reason+")")
		}
		result = append(result, view)
	}
	return result, nil
}

func mailImportTitle(job *mailImportJob) string {
	names := make([]string, 0, len(job.Files))
	for _, file := range job.Files {
		names = append(names, file.Name)
	}
	return strings.Join(names, ", ")
}

// renderMailImportPanel shows the upload form in the Import mail dialog.
func (h *Handler) renderMailImportPanel(w http.ResponseWriter, r *http.Request, message string) {
	h.renderMailImportUploadError(w, r, message)
}

func (h *Handler) renderMailImportJobs(w http.ResponseWriter, r *http.Request) {
	ctx, owner := r.Context(), h.userID(r.Context())
	locale := h.uiLocale(ctx, owner)
	jobs, err := h.mailImportJobViews(owner)
	if err != nil {
		log.Printf("mail import: %v", err)
		http.Error(w, "could not list imports", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = views.MailImportJobs(locale, jobs).Render(ctx, w)
}

func (h *Handler) handleMailImports(w http.ResponseWriter, r *http.Request) {
	if !h.mailImportsAvailable(w) {
		return
	}
	h.renderMailImportPanel(w, r, "")
}

func (h *Handler) handleMailImportJobs(w http.ResponseWriter, r *http.Request) {
	if !h.mailImportsAvailable(w) {
		return
	}
	h.renderMailImportJobs(w, r)
}

func (h *Handler) renderMailImportUploadError(w http.ResponseWriter, r *http.Request, message string) {
	ctx, owner := r.Context(), h.userID(r.Context())
	locale := h.uiLocale(ctx, owner)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = views.MailImportUploadForm(locale, message).Render(ctx, w)
}

func (h *Handler) handleUploadMailImport(w http.ResponseWriter, r *http.Request) {
	if !h.mailImportsAvailable(w) {
		return
	}
	ctx, owner := r.Context(), h.userID(r.Context())
	locale := h.uiLocale(ctx, owner)
	reader, err := r.MultipartReader()
	if err != nil {
		h.renderMailImportUploadError(w, r, i18n.T(locale, "mail.import.choose_files"))
		return
	}
	job, err := h.saveMailImportUpload(owner, reader)
	if err != nil {
		message := i18n.T(locale, "mail.import.upload_failed")
		switch {
		case errors.Is(err, errMailImportNoFiles):
			message = i18n.T(locale, "mail.import.choose_files")
		case errors.Is(err, errMailImportNoMessages):
			message = i18n.T(locale, "mail.import.no_messages")
		case errors.Is(err, errMailImportBadZip):
			message = i18n.T(locale, "mail.import.zip_unreadable")
		default:
			log.Printf("mail import: upload: %v", err)
		}
		h.renderMailImportUploadError(w, r, message)
		return
	}
	h.renderMailImportReview(w, r, job, "", "")
}

// renderMailImportReview shows where an upload's messages will go in the
// account chosen, or the first account they can go into.
func (h *Handler) renderMailImportReview(w http.ResponseWriter, r *http.Request, job *mailImportJob, accountID, message string) {
	ctx := r.Context()
	locale := h.uiLocale(ctx, job.Owner)
	accounts, err := h.mailExportAccounts(ctx, job.Owner)
	if err != nil {
		log.Printf("mail import: list accounts: %v", err)
		http.Error(w, "could not list accounts", http.StatusInternalServerError)
		return
	}
	data := views.MailImportReviewData{ID: job.ID, ExportedFrom: job.ExportedFrom, Message: message}
	for _, file := range job.Files {
		data.Files = append(data.Files, file.Name)
	}
	var chosen *models.Account
	for i, account := range accounts {
		if account.IsDeleting {
			continue
		}
		problem := mailImportAccountProblem(locale, account)
		data.Accounts = append(data.Accounts, views.MailImportAccount{MailExportAccount: mailExportAccountView(account), Problem: problem})
		if problem == "" && (account.ID == accountID || (accountID == "" && chosen == nil)) {
			chosen = &accounts[i]
		}
	}
	for i, source := range job.Sources {
		data.Sources = append(data.Sources, views.MailImportSource{Index: i, Name: source.Name, Path: source.Path, Messages: source.Messages})
	}
	if chosen != nil {
		data.AccountID = chosen.ID
		folders, err := h.mailExportFolders(ctx, job.Owner, chosen.ID)
		if err != nil {
			log.Printf("mail import: list folders: %v", err)
			data.Message = i18n.T(locale, "mail.import.folders_error")
		}
		targets := mailImportTargets(chosen.Provider, folders)
		for _, folder := range targets {
			data.Folders = append(data.Folders, views.MailImportFolder{ID: folder.ID, Path: folder.Path})
		}
		for i, source := range job.Sources {
			data.Sources[i].Target, data.Sources[i].NewName = suggestMailImportTarget(source, targets)
			if data.Sources[i].NewName == "" {
				data.Sources[i].NewName = source.Path
			}
		}
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = views.MailImportReview(locale, data).Render(ctx, w)
}

func (h *Handler) handleReviewMailImport(w http.ResponseWriter, r *http.Request) {
	if !h.mailImportsAvailable(w) {
		return
	}
	ctx, owner := r.Context(), h.userID(r.Context())
	locale := h.uiLocale(ctx, owner)
	job, err := h.mailImports.loadOwned(owner, r.PathValue("id"))
	if err != nil || job.State != mailJobReview {
		h.renderMailImportUploadError(w, r, i18n.T(locale, "mail.import.upload_gone"))
		return
	}
	h.renderMailImportReview(w, r, job, r.URL.Query().Get("account"), "")
}

func (h *Handler) handleStartMailImport(w http.ResponseWriter, r *http.Request) {
	if !h.mailImportsAvailable(w) {
		return
	}
	ctx, owner := r.Context(), h.userID(r.Context())
	locale := h.uiLocale(ctx, owner)
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	job, err := h.mailImports.loadOwned(owner, r.PathValue("id"))
	if err != nil || job.State != mailJobReview {
		h.renderMailImportUploadError(w, r, i18n.T(locale, "mail.import.upload_gone"))
		return
	}
	accountID := r.PostForm.Get("account")
	account, found, err := h.mailExportAccount(ctx, owner, accountID)
	if err != nil || !found || mailImportAccountProblem(locale, account) != "" {
		h.renderMailImportReviewInPanel(w, r, job, "", i18n.T(locale, "mail.import.choose_account"))
		return
	}
	if err := h.startMailImport(ctx, job, account, r.PostForm); err != nil {
		message := i18n.T(locale, "mail.import.start_error")
		switch {
		case errors.Is(err, errMailImportNoFolderName):
			message = i18n.T(locale, "mail.import.name_folders")
		case errors.Is(err, errMailImportNoTarget):
			message = i18n.T(locale, "mail.import.folder_gone")
		case errors.Is(err, errMailImportNothingChosen):
			message = i18n.T(locale, "mail.import.choose_folder")
		default:
			log.Printf("mail import: start: %v", err)
		}
		h.renderMailImportReviewInPanel(w, r, job, account.ID, message)
		return
	}
	mailJobStarted(w, "mail-import-dialog")
	h.renderMailImportPanel(w, r, "")
}

func (h *Handler) renderMailImportReviewInPanel(w http.ResponseWriter, r *http.Request, job *mailImportJob, accountID, message string) {
	h.renderMailImportReview(w, r, job, accountID, message)
}

func (h *Handler) handleCancelMailImport(w http.ResponseWriter, r *http.Request) {
	h.changeMailImport(w, r, h.mailImports.cancelJob)
}

func (h *Handler) handleResumeMailImport(w http.ResponseWriter, r *http.Request) {
	h.changeMailImport(w, r, h.mailImports.resume)
}

func (h *Handler) handleDeleteMailImport(w http.ResponseWriter, r *http.Request) {
	h.changeMailImport(w, r, h.mailImports.remove)
}

func (h *Handler) changeMailImport(w http.ResponseWriter, r *http.Request, change func(*mailImportJob)) {
	if !h.mailImportsAvailable(w) {
		return
	}
	job, err := h.mailImports.loadOwned(h.userID(r.Context()), r.PathValue("id"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	change(job)
	if r.FormValue("panel") == "1" {
		h.renderMailImportPanel(w, r, "")
		return
	}
	h.renderMailImportJobs(w, r)
}
