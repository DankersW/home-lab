// Package api serves the typed JSON REST surface at /api over the same stores
// as the UI. It is intended for future programmatic or mobile clients.
package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/DankersW/home-lab/containers/receipts/internal/receipt"
	"github.com/DankersW/home-lab/containers/receipts/internal/web"
)

// Handler serves the JSON API. It is mounted under /api by the composer.
type Handler struct {
	deps web.Deps
}

// New builds the API handler.
func New(deps web.Deps) *Handler { return &Handler{deps: deps} }

// Routes returns the API surface. Patterns are relative; the composer strips
// the /api prefix before dispatching here.
func (h *Handler) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /receipts", h.list)
	mux.HandleFunc("POST /receipts", h.create)
	mux.HandleFunc("GET /receipts/{id}", h.get)
	mux.HandleFunc("PUT /receipts/{id}", h.update)
	mux.HandleFunc("DELETE /receipts/{id}", h.delete)
	mux.HandleFunc("POST /receipts/{id}/attachments", h.addAttachments)
	mux.HandleFunc("GET /receipts/{id}/attachments/{attID}", h.streamAttachment)
	mux.HandleFunc("GET /receipts/{id}/attachments/{attID}/download", h.streamAttachment)
	mux.HandleFunc("DELETE /receipts/{id}/attachments/{attID}", h.deleteAttachment)
	mux.HandleFunc("GET /tags", h.listTags)
	mux.HandleFunc("POST /tags", h.createTag)
	mux.HandleFunc("PUT /receipts/{id}/tags", h.setTags)
	mux.HandleFunc("GET /export", h.export)
	return mux
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

type errorBody struct {
	Error string `json:"error"`
}

func (h *Handler) fail(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, receipt.ErrNotFound):
		writeJSON(w, http.StatusNotFound, errorBody{Error: "not found"})
	case errors.Is(err, web.ErrValidation):
		writeJSON(w, http.StatusUnprocessableEntity, errorBody{Error: strings.TrimPrefix(err.Error(), "validation: ")})
	default:
		h.deps.Logger.Error("api handler", "path", r.URL.Path, "err", err)
		writeJSON(w, http.StatusInternalServerError, errorBody{Error: "internal server error"})
	}
}
