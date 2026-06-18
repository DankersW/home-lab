package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/DankersW/home-lab/containers/receipts/internal/auth"
	"github.com/DankersW/home-lab/containers/receipts/internal/id"
	"github.com/DankersW/home-lab/containers/receipts/internal/receipt"
	"github.com/DankersW/home-lab/containers/receipts/internal/web"
)

const maxFormMemory = 8 << 20

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	q, err := h.parseQuery(ctx, r)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	receipts, err := h.deps.Store.ListReceipts(ctx, q)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	out := make([]receiptDTO, 0, len(receipts))
	for i := range receipts {
		out = append(out, toReceiptDTO(&receipts[i]))
	}
	writeJSON(w, http.StatusOK, map[string]any{"receipts": out})
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	req, err := decode(r)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	rec, err := req.toReceipt()
	if err != nil {
		h.fail(w, r, err)
		return
	}
	now := time.Now().UTC()
	rec.ID = id.New()
	rec.UploaderEmail = auth.CurrentUser(ctx).Email
	rec.CreatedAt = now
	rec.UpdatedAt = now

	tags, err := h.deps.Store.EnsureTags(ctx, req.Tags)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	rec.Tags = tags

	if err := h.deps.Store.CreateReceipt(ctx, rec); err != nil {
		h.fail(w, r, err)
		return
	}
	full, err := h.deps.Store.GetReceipt(ctx, rec.ID)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, toReceiptDTO(full))
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	rec, err := h.deps.Store.GetReceipt(r.Context(), r.PathValue("id"))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, toReceiptDTO(rec))
}

func (h *Handler) update(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	recID := r.PathValue("id")
	req, err := decode(r)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	existing, err := h.deps.Store.GetReceipt(ctx, recID)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	rec, err := req.toReceipt()
	if err != nil {
		h.fail(w, r, err)
		return
	}
	rec.ID = recID
	rec.UpdatedAt = time.Now().UTC()

	// Tags are replaced only when the request includes a tags field; a nil tags
	// field preserves the existing set.
	if req.Tags != nil {
		tags, err := h.deps.Store.EnsureTags(ctx, req.Tags)
		if err != nil {
			h.fail(w, r, err)
			return
		}
		rec.Tags = tags
	} else {
		rec.Tags = existing.Tags
	}

	if err := h.deps.Store.UpdateReceipt(ctx, rec); err != nil {
		h.fail(w, r, err)
		return
	}
	full, err := h.deps.Store.GetReceipt(ctx, recID)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, toReceiptDTO(full))
}

func (h *Handler) delete(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	keys, err := h.deps.Store.DeleteReceipt(ctx, r.PathValue("id"))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	for _, k := range keys {
		if err := h.deps.Objects.Remove(ctx, k); err != nil {
			h.deps.Logger.Warn("remove object", "key", k, "err", err)
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) addAttachments(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	recID := r.PathValue("id")
	if _, err := h.deps.Store.GetReceipt(ctx, recID); err != nil {
		h.fail(w, r, err)
		return
	}
	if err := r.ParseMultipartForm(maxFormMemory); err != nil {
		h.fail(w, r, fmt.Errorf("%w: could not read the form", web.ErrValidation))
		return
	}
	files := r.MultipartForm.File["files"]
	if len(files) > h.deps.MaxFiles {
		h.fail(w, r, fmt.Errorf("%w: too many files (max %d)", web.ErrValidation, h.deps.MaxFiles))
		return
	}
	for _, fh := range files {
		if _, err := web.SaveUpload(ctx, h.deps, recID, fh); err != nil {
			h.fail(w, r, err)
			return
		}
	}
	full, err := h.deps.Store.GetReceipt(ctx, recID)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, toReceiptDTO(full))
}

func (h *Handler) streamAttachment(w http.ResponseWriter, r *http.Request) {
	att, err := h.deps.Store.GetAttachment(r.Context(), r.PathValue("id"), r.PathValue("attID"))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	web.StreamAttachment(w, r, h.deps, att, strings.HasSuffix(r.URL.Path, "/download"))
}

func (h *Handler) deleteAttachment(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	att, err := h.deps.Store.GetAttachment(ctx, r.PathValue("id"), r.PathValue("attID"))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	if _, err := h.deps.Store.Detach(ctx, att.ID); err != nil {
		h.fail(w, r, err)
		return
	}
	if err := h.deps.Objects.Remove(ctx, att.ObjectKey); err != nil {
		h.deps.Logger.Warn("remove object", "key", att.ObjectKey, "err", err)
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) listTags(w http.ResponseWriter, r *http.Request) {
	tags, err := h.deps.Store.ListTags(r.Context())
	if err != nil {
		h.fail(w, r, err)
		return
	}
	names := make([]string, 0, len(tags))
	for _, t := range tags {
		names = append(names, t.Name)
	}
	writeJSON(w, http.StatusOK, map[string]any{"tags": names})
}

func (h *Handler) createTag(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		h.fail(w, r, fmt.Errorf("%w: invalid JSON body", web.ErrValidation))
		return
	}
	if strings.TrimSpace(body.Name) == "" {
		h.fail(w, r, fmt.Errorf("%w: tag name is required", web.ErrValidation))
		return
	}
	tags, err := h.deps.Store.EnsureTags(r.Context(), []string{body.Name})
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"tag": tags[0].Name})
}

func (h *Handler) setTags(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	recID := r.PathValue("id")
	var body struct {
		Tags []string `json:"tags"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		h.fail(w, r, fmt.Errorf("%w: invalid JSON body", web.ErrValidation))
		return
	}
	if _, err := h.deps.Store.GetReceipt(ctx, recID); err != nil {
		h.fail(w, r, err)
		return
	}
	tags, err := h.deps.Store.EnsureTags(ctx, body.Tags)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	ids := make([]string, 0, len(tags))
	for _, t := range tags {
		ids = append(ids, t.ID)
	}
	if _, err := h.deps.Store.SetReceiptTags(ctx, recID, ids); err != nil {
		h.fail(w, r, err)
		return
	}
	full, err := h.deps.Store.GetReceipt(ctx, recID)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, toReceiptDTO(full))
}

func (h *Handler) export(w http.ResponseWriter, r *http.Request) {
	exp, err := h.deps.Store.ExportAll(r.Context())
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, exp)
}

// ---- helpers ----

func decode(r *http.Request) (receiptRequest, error) {
	var req receiptRequest
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		return receiptRequest{}, fmt.Errorf("%w: invalid JSON body: %v", web.ErrValidation, err)
	}
	return req, nil
}

func (h *Handler) parseQuery(ctx context.Context, r *http.Request) (receipt.ReceiptQuery, error) {
	v := r.URL.Query()
	q := receipt.ReceiptQuery{
		Text:          strings.TrimSpace(v.Get("q")),
		UploaderEmail: strings.TrimSpace(v.Get("uploader")),
		Currency:      strings.TrimSpace(v.Get("currency")),
	}
	if name := strings.ToLower(strings.TrimSpace(v.Get("tag"))); name != "" {
		tags, err := h.deps.Store.ListTags(ctx)
		if err != nil {
			return receipt.ReceiptQuery{}, err
		}
		id := "\x00unknown"
		for _, t := range tags {
			if t.Name == name {
				id = t.ID
				break
			}
		}
		q.TagIDs = []string{id}
	}
	if from, ok := parseDate(v.Get("from")); ok {
		q.PurchaseFrom = &from
	}
	if to, ok := parseDate(v.Get("to")); ok {
		q.PurchaseTo = &to
	}
	return q, nil
}

func parseDate(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, false
	}
	t, err := time.Parse(dateLayout, s)
	if err != nil {
		return time.Time{}, false
	}
	return t.UTC(), true
}
