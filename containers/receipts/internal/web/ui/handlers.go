package ui

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/DankersW/home-lab/containers/receipts/internal/auth"
	"github.com/DankersW/home-lab/containers/receipts/internal/id"
	"github.com/DankersW/home-lab/containers/receipts/internal/receipt"
	"github.com/DankersW/home-lab/containers/receipts/internal/web"
)

const maxFormMemory = 8 << 20 // multipart in-memory threshold; overflow spills to TMPDIR

// pageView is the model for a full page. Both shells render from it: the mobile
// shell shows the screen named by Screen; the desktop shell always shows the
// three-pane and uses Selected for the detail pane and active card.
type pageView struct {
	Title     string
	User      auth.User
	Flash     string
	Screen    string // mobile screen: "add" | "list" | "detail" | "edit"
	Query     string
	ActiveTag string
	Items     []listItem
	Total     int // total receipts, ignoring the active filter
	Tags      []receipt.TagCount
	Selected  *receipt.Receipt // detail/edit target and desktop selection
}

// listItem is one card's data: a lean receipt plus its attachment rollup.
type listItem struct {
	R       receipt.Receipt
	Count   int
	ImageID string // first image attachment id, "" if none
}

// resultsView is the model for the swappable list-results fragment.
type resultsView struct {
	Items     []listItem
	Total     int
	ActiveTag string
	Desktop   bool
	SelID     string // selected receipt id, for the active highlight
}

type modalView struct {
	Mode    string // "add" | "edit"
	Receipt *receipt.Receipt
}

type confirmView struct {
	Receipt *receipt.Receipt
	Desktop bool
}

// ---- pages ----

func (h *Handler) home(w http.ResponseWriter, r *http.Request) {
	h.renderPage(w, r, "add", "Receipts", nil)
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	h.renderPage(w, r, "list", "Your receipts", nil)
}

func (h *Handler) detail(w http.ResponseWriter, r *http.Request) {
	sel, err := h.deps.Store.GetReceipt(r.Context(), r.PathValue("id"))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	h.renderPage(w, r, "detail", receiptTitle(sel), sel)
}

func (h *Handler) editPage(w http.ResponseWriter, r *http.Request) {
	sel, err := h.deps.Store.GetReceipt(r.Context(), r.PathValue("id"))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	h.renderPage(w, r, "edit", "Edit receipt", sel)
}

// renderPage loads the shell data (filtered list + tag counts + total) and
// renders the full page for the given mobile screen and optional selection.
func (h *Handler) renderPage(w http.ResponseWriter, r *http.Request, screen, title string, sel *receipt.Receipt) {
	ctx := r.Context()
	q, tag := searchParams(r)
	items, tags, total, err := h.loadShell(ctx, q, tag)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	h.render(w, http.StatusOK, "page", pageView{
		Title:     title,
		User:      auth.CurrentUser(ctx),
		Flash:     flash(r),
		Screen:    screen,
		Query:     q,
		ActiveTag: tag,
		Items:     items,
		Total:     total,
		Tags:      tags,
		Selected:  sel,
	})
}

// ---- fragments ----

func (h *Handler) fragmentList(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	q, tag := searchParams(r)
	items, _, total, err := h.loadShell(ctx, q, tag)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	h.render(w, http.StatusOK, "list_results", resultsView{
		Items:     items,
		Total:     total,
		ActiveTag: tag,
		Desktop:   r.URL.Query().Get("shell") == "desktop",
	})
}

func (h *Handler) fragmentModal(w http.ResponseWriter, r *http.Request) {
	mode := r.URL.Query().Get("mode")
	view := modalView{Mode: mode}
	if mode == "edit" {
		sel, err := h.deps.Store.GetReceipt(r.Context(), r.URL.Query().Get("id"))
		if err != nil {
			h.fail(w, r, err)
			return
		}
		view.Receipt = sel
	}
	h.render(w, http.StatusOK, "form_modal", view)
}

func (h *Handler) fragmentConfirm(w http.ResponseWriter, r *http.Request) {
	sel, err := h.deps.Store.GetReceipt(r.Context(), r.URL.Query().Get("id"))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	h.render(w, http.StatusOK, "confirm", confirmView{
		Receipt: sel,
		Desktop: r.URL.Query().Get("shell") == "desktop",
	})
}

func (h *Handler) fragmentDetail(w http.ResponseWriter, r *http.Request) {
	sel, err := h.deps.Store.GetReceipt(r.Context(), r.URL.Query().Get("id"))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	h.render(w, http.StatusOK, "detail_pane_inner", sel)
}

// ---- mutations ----

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

	dest := "/receipts/" + rec.ID
	if r.FormValue("origin") == "mobile" {
		dest = "/list"
	}
	h.redirect(w, dest, "Receipt saved")
}

func (h *Handler) update(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	recID := r.PathValue("id")
	if err := r.ParseForm(); err != nil {
		h.fail(w, r, fmt.Errorf("%w: could not read the form", web.ErrValidation))
		return
	}
	if _, err := h.deps.Store.GetReceipt(ctx, recID); err != nil {
		h.fail(w, r, err)
		return
	}
	rec, err := parseReceiptFields(r)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	rec.ID = recID
	rec.UpdatedAt = time.Now().UTC()

	tags, err := h.deps.Store.EnsureTags(ctx, splitTags(r.FormValue("tags")))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	rec.Tags = tags

	if err := h.deps.Store.UpdateReceipt(ctx, rec); err != nil {
		h.fail(w, r, err)
		return
	}
	h.redirect(w, "/receipts/"+recID, "Changes saved")
}

func (h *Handler) delete(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	keys, err := h.deps.Store.DeleteReceipt(ctx, r.PathValue("id"))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	h.purgeObjects(ctx, keys)
	h.redirect(w, "/list", "Receipt deleted")
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
	h.render(w, http.StatusOK, "edit_thumbs", full)
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
	h.render(w, http.StatusOK, "edit_thumbs", full)
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

// ---- helpers ----

// loadShell loads the data both shells need: the filtered list (as cards), the
// tag counts for the filter UI, and the unfiltered total for the counters.
func (h *Handler) loadShell(ctx context.Context, q, tag string) (items []listItem, tags []receipt.TagCount, total int, err error) {
	query := receipt.ReceiptQuery{Text: q}
	if tag != "" {
		tagID, terr := h.tagIDByName(ctx, tag)
		if terr != nil {
			return nil, nil, 0, terr
		}
		// An unknown tag yields no matches rather than ignoring the filter.
		if tagID == "" {
			tagID = "\x00unknown"
		}
		query.TagIDs = []string{tagID}
	}
	receipts, err := h.deps.Store.ListReceipts(ctx, query)
	if err != nil {
		return nil, nil, 0, err
	}
	items, err = h.buildItems(ctx, receipts)
	if err != nil {
		return nil, nil, 0, err
	}
	tags, err = h.deps.Store.TagCounts(ctx)
	if err != nil {
		return nil, nil, 0, err
	}
	total, err = h.deps.Store.CountReceipts(ctx)
	if err != nil {
		return nil, nil, 0, err
	}
	return items, tags, total, nil
}

func (h *Handler) buildItems(ctx context.Context, receipts []receipt.Receipt) ([]listItem, error) {
	ids := make([]string, len(receipts))
	for i := range receipts {
		ids[i] = receipts[i].ID
	}
	sums, err := h.deps.Store.AttachmentSummaries(ctx, ids)
	if err != nil {
		return nil, err
	}
	items := make([]listItem, 0, len(receipts))
	for i := range receipts {
		s := sums[receipts[i].ID]
		items = append(items, listItem{R: receipts[i], Count: s.Count, ImageID: s.FirstImageID})
	}
	return items, nil
}

// redirect tells htmx to navigate to dest, carrying a toast message as ?flash.
func (h *Handler) redirect(w http.ResponseWriter, dest, msg string) {
	if msg != "" {
		dest += "?flash=" + url.QueryEscape(msg)
	}
	w.Header().Set("HX-Redirect", dest)
	w.WriteHeader(http.StatusNoContent)
}

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

func searchParams(r *http.Request) (q, tag string) {
	v := r.URL.Query()
	return strings.TrimSpace(v.Get("q")), strings.ToLower(strings.TrimSpace(v.Get("tag")))
}

func flash(r *http.Request) string {
	return strings.TrimSpace(r.URL.Query().Get("flash"))
}

func receiptTitle(rec *receipt.Receipt) string {
	if rec.Title != "" {
		return rec.Title
	}
	if rec.Merchant != "" {
		return rec.Merchant
	}
	return "Receipt"
}

// parseReceiptFields extracts the editable fields from a form. Only Title is
// required; merchant and amount may be blank and the date defaults to today.
// It does not set ID, timestamps, uploader, or tags.
func parseReceiptFields(r *http.Request) (*receipt.Receipt, error) {
	title := strings.TrimSpace(r.FormValue("title"))
	if title == "" {
		return nil, fmt.Errorf("%w: title is required", web.ErrValidation)
	}
	purchase := time.Now().UTC()
	if v := strings.TrimSpace(r.FormValue("purchase_date")); v != "" {
		t, err := time.Parse("2006-01-02", v)
		if err != nil {
			return nil, fmt.Errorf("%w: purchase date must be YYYY-MM-DD", web.ErrValidation)
		}
		purchase = t.UTC()
	}
	var amountMinor int64
	if v := strings.TrimSpace(r.FormValue("amount")); v != "" {
		minor, err := receipt.ParseMoneyMinor(v)
		if err != nil {
			return nil, fmt.Errorf("%w: %v", web.ErrValidation, err)
		}
		amountMinor = minor
	}
	return &receipt.Receipt{
		Title:        title,
		Merchant:     strings.TrimSpace(r.FormValue("merchant")),
		PurchaseDate: purchase,
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
