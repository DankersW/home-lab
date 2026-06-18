package web

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"mime/multipart"
	"path/filepath"
	"time"

	"github.com/DankersW/home-lab/containers/receipts/internal/db/bucket"
	"github.com/DankersW/home-lab/containers/receipts/internal/id"
	"github.com/DankersW/home-lab/containers/receipts/internal/receipt"
)

// SaveUpload validates one uploaded file, stores its bytes in the object store
// under a server-generated key, and records its metadata. The DB row is the
// source of truth: on a metadata failure it best-effort removes the just-written
// object so bytes do not orphan. The file's real type is sniffed from its bytes,
// never trusted from the client-declared type or filename.
func SaveUpload(ctx context.Context, deps Deps, receiptID string, fh *multipart.FileHeader) (*receipt.Attachment, error) {
	if fh.Size <= 0 {
		return nil, fmt.Errorf("%w: %q is empty", ErrValidation, fh.Filename)
	}
	if fh.Size > deps.MaxUploadBytes {
		return nil, fmt.Errorf("%w: %q exceeds %d MiB", ErrValidation, fh.Filename, deps.MaxUploadBytes>>20)
	}

	src, err := fh.Open()
	if err != nil {
		return nil, fmt.Errorf("open upload %q: %w", fh.Filename, err)
	}
	defer func() { _ = src.Close() }()

	head := make([]byte, 512)
	n, err := io.ReadFull(src, head)
	if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
		return nil, fmt.Errorf("read upload %q: %w", fh.Filename, err)
	}
	head = head[:n]

	contentType, kind, ok := receipt.ClassifyUpload(head)
	if !ok {
		return nil, fmt.Errorf("%w: %q has an unsupported file type", ErrValidation, fh.Filename)
	}

	// Re-join the sniffed head with the remaining stream so we never rely on the
	// multipart part being seekable (it may be memory- or disk-backed).
	body := io.MultiReader(bytes.NewReader(head), src)

	attID := id.New()
	key := bucket.Key(receiptID, attID)
	if err := deps.Objects.Put(ctx, key, body, fh.Size, contentType); err != nil {
		return nil, fmt.Errorf("store upload %q: %w", fh.Filename, err)
	}

	att := &receipt.Attachment{
		ID:          attID,
		ReceiptID:   receiptID,
		ObjectKey:   key,
		Filename:    filepath.Base(fh.Filename),
		ContentType: contentType,
		Kind:        kind,
		SizeBytes:   fh.Size,
		CreatedAt:   time.Now().UTC(),
	}
	if err := deps.Store.Attach(ctx, att); err != nil {
		if rmErr := deps.Objects.Remove(ctx, key); rmErr != nil {
			deps.Logger.Warn("orphaned object after attach failure", "key", key, "err", rmErr)
		}
		return nil, fmt.Errorf("record upload %q: %w", fh.Filename, err)
	}
	return att, nil
}
