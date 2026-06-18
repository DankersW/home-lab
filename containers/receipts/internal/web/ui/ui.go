// Package ui serves the htmx-driven HTML surface at /. It renders server-side
// templates and HTML fragments for two layouts that share the same leaf
// partials and data: a phone-column stack (< 960px) and a desktop three-pane
// master–detail (>= 960px), switched purely in CSS. The only client-side
// JavaScript is htmx plus a small helper file (static/app.js).
package ui

import (
	"bytes"
	"embed"
	"errors"
	"fmt"
	"html/template"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/DankersW/home-lab/containers/receipts/internal/receipt"
	"github.com/DankersW/home-lab/containers/receipts/internal/web"
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
		"dateShort":   formatDateShort,
		"amount":      formatAmount,
		"amountValue": amountValue,
		"today":       func() string { return time.Now().Format("2006-01-02") },
		"initial":     initial,
		"tagsCSV":     tagsCSV,
		"isSel":       func(sel *receipt.Receipt, id string) bool { return sel != nil && sel.ID == id },
		"dict":        dict,
	}
	tmpl := template.Must(template.New("").Funcs(funcs).ParseFS(templateFS, "templates/*.gohtml"))
	return &Handler{deps: deps, tmpl: tmpl}
}

// Routes returns the HTML surface mounted at the site root.
func (h *Handler) Routes() http.Handler {
	mux := http.NewServeMux()
	// Full pages (each renders both shells; CSS shows one).
	mux.HandleFunc("GET /{$}", h.home)
	mux.HandleFunc("GET /list", h.list)
	mux.HandleFunc("GET /receipts/{id}", h.detail)
	mux.HandleFunc("GET /receipts/{id}/edit", h.editPage)

	// HTMX fragments.
	mux.HandleFunc("GET /fragments/list", h.fragmentList)
	mux.HandleFunc("GET /fragments/modal", h.fragmentModal)
	mux.HandleFunc("GET /fragments/confirm", h.fragmentConfirm)
	mux.HandleFunc("GET /fragments/detail", h.fragmentDetail)

	// Mutations.
	mux.HandleFunc("POST /receipts", h.create)
	mux.HandleFunc("PUT /receipts/{id}", h.update)
	mux.HandleFunc("DELETE /receipts/{id}", h.delete)
	mux.HandleFunc("POST /receipts/{id}/attachments", h.addAttachments)
	mux.HandleFunc("DELETE /receipts/{id}/attachments/{attID}", h.deleteAttachment)
	mux.HandleFunc("GET /receipts/{id}/attachments/{attID}", h.streamAttachment)
	mux.HandleFunc("GET /receipts/{id}/attachments/{attID}/download", h.streamAttachment)
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

// ---- template helpers ----

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

// formatDateShort renders a date as "2 Nov 2024" (the design's en-GB style).
func formatDateShort(t time.Time) string {
	if t.IsZero() {
		return "—"
	}
	return t.Format("2 Jan 2006")
}

// amountValue renders the amount for a form input as plain "12.34".
func amountValue(m receipt.Money) string {
	if m.AmountMinor == 0 {
		return ""
	}
	sign, v := "", m.AmountMinor
	if v < 0 {
		sign, v = "-", -v
	}
	return fmt.Sprintf("%s%d.%02d", sign, v/100, v%100)
}

// formatAmount renders the amount grouped sv-SE style ("18 990", "18 990,50").
// A zero amount renders as "—" (treated as "no amount recorded").
func formatAmount(m receipt.Money) string {
	if m.AmountMinor == 0 {
		return "—"
	}
	minor := m.AmountMinor
	neg := minor < 0
	if neg {
		minor = -minor
	}
	out := groupDigits(strconv.FormatInt(minor/100, 10))
	if frac := minor % 100; frac != 0 {
		out = fmt.Sprintf("%s,%02d", out, frac)
	}
	if neg {
		out = "-" + out
	}
	return out
}

// groupDigits inserts a non-breaking space every three digits from the right.
func groupDigits(s string) string {
	n := len(s)
	if n <= 3 {
		return s
	}
	var b strings.Builder
	if pre := n % 3; pre > 0 {
		b.WriteString(s[:pre])
	}
	for i := n % 3; i < n; i += 3 {
		if b.Len() > 0 {
			b.WriteString(" ")
		}
		b.WriteString(s[i : i+3])
	}
	return b.String()
}

// initial returns the uppercased first character of s, or "?" if empty.
func initial(s string) string {
	for _, r := range strings.TrimSpace(s) {
		return strings.ToUpper(string(r))
	}
	return "?"
}

// tagsCSV joins tag names as "a, b, c" for the edit form's hidden field.
func tagsCSV(tags []receipt.Tag) string {
	names := make([]string, 0, len(tags))
	for _, t := range tags {
		names = append(names, t.Name)
	}
	return strings.Join(names, ", ")
}

// dict builds a map from alternating key/value args, for passing several values
// to a partial template: {{template "card" dict "It" . "Desktop" true}}.
func dict(values ...any) (map[string]any, error) {
	if len(values)%2 != 0 {
		return nil, errors.New("dict: odd number of arguments")
	}
	m := make(map[string]any, len(values)/2)
	for i := 0; i < len(values); i += 2 {
		key, ok := values[i].(string)
		if !ok {
			return nil, errors.New("dict: keys must be strings")
		}
		m[key] = values[i+1]
	}
	return m, nil
}
