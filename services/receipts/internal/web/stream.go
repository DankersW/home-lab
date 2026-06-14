package web

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"

	"github.com/dankers/home-lab/services/receipts/internal/db/bucket"
	"github.com/dankers/home-lab/services/receipts/internal/receipt"
)

// StreamAttachment streams an attachment's bytes from the object store. It sets
// the trusted stored content type and a disposition of inline for known-safe
// image/pdf kinds, attachment otherwise. nosniff prevents content-type override.
func StreamAttachment(w http.ResponseWriter, r *http.Request, deps Deps, att *receipt.Attachment, forceDownload bool) {
	rc, info, err := deps.Objects.Get(r.Context(), att.ObjectKey)
	if err != nil {
		if errors.Is(err, bucket.ErrNotFound) {
			http.NotFound(w, r)
			return
		}
		deps.Logger.Error("get object", "key", att.ObjectKey, "err", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	defer func() {
		if cerr := rc.Close(); cerr != nil {
			deps.Logger.Warn("close object reader", "key", att.ObjectKey, "err", cerr)
		}
	}()

	disposition := "inline"
	if forceDownload || !att.Kind.Valid() {
		disposition = "attachment"
	}
	w.Header().Set("Content-Type", att.ContentType)
	w.Header().Set("Content-Disposition", fmt.Sprintf("%s; filename=%q", disposition, att.Filename))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "private, max-age=300")
	if info.Size > 0 {
		w.Header().Set("Content-Length", strconv.FormatInt(info.Size, 10))
	}
	if _, err := io.Copy(w, rc); err != nil {
		// The response has already started; we can only log.
		deps.Logger.Warn("stream copy interrupted", "key", att.ObjectKey, "err", err)
	}
}
