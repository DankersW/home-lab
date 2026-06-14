package ui

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/dankers/home-lab/services/receipts/internal/auth"
	"github.com/dankers/home-lab/services/receipts/internal/id"
	"github.com/dankers/home-lab/services/receipts/internal/receipt"
	"github.com/dankers/home-lab/services/receipts/internal/web"
)

// View types passed to templates.
type pageEnvelope struct {
	Title  string
	User   auth.User
	Index  *indexView
	Detail *detailView
}

type indexView struct {
	Receipts []receipt.Receipt
	Tags     []receipt.Tag
}

type detailView struct {
	Receipt *receipt.Receipt
}

type listView struct {
	Receipts []receipt.Receipt
}

type tagsView struct {
	Tags []receipt.Tag
}

const maxFormMemory = 8 << 20 // multipart in-memory threshold; overflow spills to TMPDIR

func (h *Handler) index(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	receipts, err := h.deps.Store.ListReceipts(ctx, receipt.ReceiptQuery{})
	if err != nil {
		h.fail(w, r, err)
		return
	}
	tags, err := h.deps.Store.ListTags(ctx)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	h.render(w, http.StatusOK, "index", pageEnvelope{
		Title: "All receipts",
		User:  auth.CurrentUser(ctx),
		Index: &indexView{Receipts: receipts, Tags: tags},
	})
}

func (h *Handler) search(w http.ResponseWriter, r *http.Request) {
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
	h.render(w, http.StatusOK, "receipt_list", listView{Receipts: receipts})
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	user := auth.CurrentUser(ctx)
	if err := r.ParseMultipartForm(maxFormMemory); err != nil {
		h.fail(w, r, fmt.Errorf("%w: could not read the form", web.ErrValidation))
		return
	}
	rec, err := parseReceiptFields(r)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	now := time.Now().UTC()
	rec.ID = id.New()
	rec.UploaderEmail = user.Email
	rec.CreatedAt = now
	rec.UpdatedAt = now

	tags, err := h.deps.Store.EnsureTags(ctx, splitTags(r.FormValue("tags")))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	rec.Tags = tags

	if err := h.deps.Store.CreateReceipt(ctx, rec); err != nil {
		h.fail(w, r, err)
		return
	}
	if err := h.saveFiles(ctx, rec.ID, r); err != nil {
		h.fail(w, r, err)
		return
	}

	full, err := h.deps.Store.GetReceipt(ctx, rec.ID)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	w.Header().Set("HX-Trigger", "receiptCreated")
	h.render(w, http.StatusOK, "receipt_row", full)
}

func (h *Handler) detail(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rec, err := h.deps.Store.GetReceipt(ctx, r.PathValue("id"))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	title := rec.Title
	if title == "" {
		title = rec.Merchant
	}
	h.render(w, http.StatusOK, "detail", pageEnvelope{
		Title:  title,
		User:   auth.CurrentUser(ctx),
		Detail: &detailView{Receipt: rec},
	})
}

func (h *Handler) update(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	recID := r.PathValue("id")
	if err := r.ParseForm(); err != nil {
		h.fail(w, r, fmt.Errorf("%w: could not read the form", web.ErrValidation))
		return
	}
	// Load the existing receipt to preserve tags (edited via the tag form, not here).
	existing, err := h.deps.Store.GetReceipt(ctx, recID)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	rec, err := parseReceiptFields(r)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	rec.ID = recID
	rec.Tags = existing.Tags
	rec.UpdatedAt = time.Now().UTC()
	if err := h.deps.Store.UpdateReceipt(ctx, rec); err != nil {
		h.fail(w, r, err)
		return
	}
	w.Header().Set("HX-Redirect", "/receipts/"+recID)
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) delete(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	keys, err := h.deps.Store.DeleteReceipt(ctx, r.PathValue("id"))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	h.purgeObjects(ctx, keys)
	w.Header().Set("HX-Redirect", "/")
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
	if err := h.saveFiles(ctx, recID, r); err != nil {
		h.fail(w, r, err)
		return
	}
	full, err := h.deps.Store.GetReceipt(ctx, recID)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	h.render(w, http.StatusOK, "attachments", full)
}

func (h *Handler) streamAttachment(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	att, err := h.deps.Store.GetAttachment(ctx, r.PathValue("id"), r.PathValue("attID"))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	web.StreamAttachment(w, r, h.deps, att, strings.HasSuffix(r.URL.Path, "/download"))
}

func (h *Handler) deleteAttachment(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	recID := r.PathValue("id")
	att, err := h.deps.Store.GetAttachment(ctx, recID, r.PathValue("attID"))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	if _, err := h.deps.Store.Detach(ctx, att.ID); err != nil {
		h.fail(w, r, err)
		return
	}
	h.purgeObjects(ctx, []string{att.ObjectKey})
	full, err := h.deps.Store.GetReceipt(ctx, recID)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	h.render(w, http.StatusOK, "attachments", full)
}

func (h *Handler) listTags(w http.ResponseWriter, r *http.Request) {
	tags, err := h.deps.Store.ListTags(r.Context())
	if err != nil {
		h.fail(w, r, err)
		return
	}
	h.render(w, http.StatusOK, "tag_datalist", tagsView{Tags: tags})
}

func (h *Handler) createTag(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if err := r.ParseForm(); err != nil {
		h.fail(w, r, fmt.Errorf("%w: could not read the form", web.ErrValidation))
		return
	}
	if strings.TrimSpace(r.FormValue("name")) == "" {
		h.fail(w, r, fmt.Errorf("%w: tag name is required", web.ErrValidation))
		return
	}
	if _, err := h.deps.Store.EnsureTags(ctx, []string{r.FormValue("name")}); err != nil {
		h.fail(w, r, err)
		return
	}
	tags, err := h.deps.Store.ListTags(ctx)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	h.render(w, http.StatusOK, "tag_datalist", tagsView{Tags: tags})
}

func (h *Handler) setTags(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	recID := r.PathValue("id")
	if err := r.ParseForm(); err != nil {
		h.fail(w, r, fmt.Errorf("%w: could not read the form", web.ErrValidation))
		return
	}
	tags, err := h.deps.Store.EnsureTags(ctx, splitTags(r.FormValue("tags")))
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
	h.render(w, http.StatusOK, "tag_chips", full)
}

// ---- helpers ----

// saveFiles stores all uploaded files in the "files" field for the given receipt.
func (h *Handler) saveFiles(ctx context.Context, receiptID string, r *http.Request) error {
	if r.MultipartForm == nil {
		return nil
	}
	files := r.MultipartForm.File["files"]
	if len(files) > h.deps.MaxFiles {
		return fmt.Errorf("%w: too many files (max %d)", web.ErrValidation, h.deps.MaxFiles)
	}
	for _, fh := range files {
		if _, err := web.SaveUpload(ctx, h.deps, receiptID, fh); err != nil {
			return err
		}
	}
	return nil
}

func (h *Handler) purgeObjects(ctx context.Context, keys []string) {
	for _, k := range keys {
		if err := h.deps.Objects.Remove(ctx, k); err != nil {
			h.deps.Logger.Warn("remove object", "key", k, "err", err)
		}
	}
}

func (h *Handler) parseQuery(ctx context.Context, r *http.Request) (receipt.ReceiptQuery, error) {
	v := r.URL.Query()
	q := receipt.ReceiptQuery{
		Text:          strings.TrimSpace(v.Get("q")),
		UploaderEmail: strings.TrimSpace(v.Get("uploader")),
	}
	if name := strings.ToLower(strings.TrimSpace(v.Get("tag"))); name != "" {
		id, err := h.tagIDByName(ctx, name)
		if err != nil {
			return receipt.ReceiptQuery{}, err
		}
		// An unknown tag yields no matches rather than ignoring the filter.
		if id == "" {
			id = "\x00unknown"
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

func (h *Handler) tagIDByName(ctx context.Context, name string) (string, error) {
	tags, err := h.deps.Store.ListTags(ctx)
	if err != nil {
		return "", err
	}
	for _, t := range tags {
		if t.Name == name {
			return t.ID, nil
		}
	}
	return "", nil
}

// parseReceiptFields extracts the editable receipt fields from a form. It does
// not set ID, timestamps, uploader, or tags.
func parseReceiptFields(r *http.Request) (*receipt.Receipt, error) {
	merchant := strings.TrimSpace(r.FormValue("merchant"))
	if merchant == "" {
		return nil, fmt.Errorf("%w: merchant is required", web.ErrValidation)
	}
	purchase, err := time.Parse("2006-01-02", r.FormValue("purchase_date"))
	if err != nil {
		return nil, fmt.Errorf("%w: purchase date must be YYYY-MM-DD", web.ErrValidation)
	}
	amountMinor, err := receipt.ParseMoneyMinor(r.FormValue("amount"))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", web.ErrValidation, err)
	}

	return &receipt.Receipt{
		Title:        strings.TrimSpace(r.FormValue("title")),
		Description:  strings.TrimSpace(r.FormValue("description")),
		Merchant:     merchant,
		PurchaseDate: purchase.UTC(),
		Amount:       receipt.Money{AmountMinor: amountMinor, Currency: receipt.DefaultCurrency},
		Note:         strings.TrimSpace(r.FormValue("note")),
	}, nil
}

func splitTags(s string) []string {
	var out []string
	seen := make(map[string]bool)
	for _, p := range strings.Split(s, ",") {
		t := strings.ToLower(strings.TrimSpace(p))
		if t == "" || seen[t] {
			continue
		}
		seen[t] = true
		out = append(out, t)
	}
	return out
}

func parseDate(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, false
	}
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return time.Time{}, false
	}
	return t.UTC(), true
}
