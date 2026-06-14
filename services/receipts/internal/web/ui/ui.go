// Package ui serves the htmx-driven HTML surface at /. It renders server-side
// templates and HTML fragments; the only client-side JavaScript is htmx itself
// plus a tiny inline image-preview helper.
package ui

import (
	"bytes"
	"embed"
	"errors"
	"fmt"
	"html/template"
	"net/http"
	"strings"
	"time"

	"github.com/dankers/home-lab/services/receipts/internal/receipt"
	"github.com/dankers/home-lab/services/receipts/internal/web"
)

//go:embed templates/*.gohtml
var templateFS embed.FS

//go:embed static/*
var staticFS embed.FS

// Handler renders the HTML surface.
type Handler struct {
	deps web.Deps
	tmpl *template.Template
}

// New builds the UI handler, parsing the embedded templates once.
func New(deps web.Deps) *Handler {
	funcs := template.FuncMap{
		"date":        formatDate,
		"amountValue": amountValue,
		"today":       func() string { return time.Now().Format("2006-01-02") },
	}
	tmpl := template.Must(template.New("").Funcs(funcs).ParseFS(templateFS, "templates/*.gohtml"))
	return &Handler{deps: deps, tmpl: tmpl}
}

// Routes returns the HTML surface mounted at the site root.
func (h *Handler) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", h.index)
	mux.HandleFunc("GET /receipts", h.search)
	mux.HandleFunc("POST /receipts", h.create)
	mux.HandleFunc("GET /receipts/{id}", h.detail)
	mux.HandleFunc("PUT /receipts/{id}", h.update)
	mux.HandleFunc("DELETE /receipts/{id}", h.delete)
	mux.HandleFunc("POST /receipts/{id}/attachments", h.addAttachments)
	mux.HandleFunc("GET /receipts/{id}/attachments/{attID}", h.streamAttachment)
	mux.HandleFunc("GET /receipts/{id}/attachments/{attID}/download", h.streamAttachment)
	mux.HandleFunc("DELETE /receipts/{id}/attachments/{attID}", h.deleteAttachment)
	mux.HandleFunc("GET /tags", h.listTags)
	mux.HandleFunc("POST /tags", h.createTag)
	mux.HandleFunc("POST /receipts/{id}/tags", h.setTags)
	return mux
}

// StaticHandler serves the embedded static assets. Mount it under /static/.
func StaticHandler() http.Handler {
	return http.FileServer(http.FS(staticFS))
}

// render executes a named template into a buffer first, so a template error
// becomes a clean 500 rather than a half-written response.
func (h *Handler) render(w http.ResponseWriter, status int, name string, data any) {
	var buf bytes.Buffer
	if err := h.tmpl.ExecuteTemplate(&buf, name, data); err != nil {
		h.deps.Logger.Error("render template", "template", name, "err", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_, _ = buf.WriteTo(w)
}

// fail maps an error to a user-facing response: validation errors flash an OOB
// message, missing entities 404, everything else logs and flashes a generic
// message.
func (h *Handler) fail(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, receipt.ErrNotFound):
		http.NotFound(w, r)
	case errors.Is(err, web.ErrValidation):
		msg := strings.TrimPrefix(err.Error(), "validation: ")
		h.render(w, http.StatusOK, "flash_error", msg)
	default:
		h.deps.Logger.Error("ui handler", "path", r.URL.Path, "err", err)
		h.render(w, http.StatusOK, "flash_error", "Something went wrong.")
	}
}

func formatDate(v any) string {
	switch t := v.(type) {
	case time.Time:
		if t.IsZero() {
			return ""
		}
		return t.Format("2006-01-02")
	case *time.Time:
		if t == nil || t.IsZero() {
			return ""
		}
		return t.Format("2006-01-02")
	default:
		return ""
	}
}

func amountValue(m receipt.Money) string {
	sign, v := "", m.AmountMinor
	if v < 0 {
		sign, v = "-", -v
	}
	return fmt.Sprintf("%s%d.%02d", sign, v/100, v%100)
}
