// Package web wires the HTTP server. It defines the storage interfaces the
// handlers consume, the shared middleware, and reusable upload/stream helpers.
// The ui and api sub-packages provide the two handler surfaces and depend on
// this package; composition happens in cmd/receipts so there is no import cycle.
package web

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/dankers/home-lab/services/receipts/internal/db/bucket"
	"github.com/dankers/home-lab/services/receipts/internal/receipt"
)

// Store is the metadata persistence contract the HTTP layer needs. The concrete
// implementation lives in internal/db/sqlite.
type Store interface {
	CreateReceipt(ctx context.Context, r *receipt.Receipt) error
	GetReceipt(ctx context.Context, id string) (*receipt.Receipt, error)
	UpdateReceipt(ctx context.Context, r *receipt.Receipt) error
	DeleteReceipt(ctx context.Context, id string) (objectKeys []string, err error)
	ListReceipts(ctx context.Context, q receipt.ReceiptQuery) ([]receipt.Receipt, error)
	CountReceipts(ctx context.Context) (int, error)
	ExportAll(ctx context.Context) (receipt.Export, error)

	Attach(ctx context.Context, a *receipt.Attachment) error
	Detach(ctx context.Context, attachmentID string) (objectKey string, err error)
	GetAttachment(ctx context.Context, receiptID, attID string) (*receipt.Attachment, error)
	ListAttachments(ctx context.Context, receiptID string) ([]receipt.Attachment, error)
	AttachmentSummaries(ctx context.Context, receiptIDs []string) (map[string]receipt.AttachmentSummary, error)

	EnsureTags(ctx context.Context, names []string) ([]receipt.Tag, error)
	ListTags(ctx context.Context) ([]receipt.Tag, error)
	TagCounts(ctx context.Context) ([]receipt.TagCount, error)
	DeleteTag(ctx context.Context, id string) error
	SetReceiptTags(ctx context.Context, receiptID string, tagIDs []string) ([]receipt.Tag, error)

	Ping(ctx context.Context) error
}

// ObjectStore is the blob contract. The concrete implementation lives in
// internal/db/bucket.
type ObjectStore interface {
	Put(ctx context.Context, key string, r io.Reader, size int64, contentType string) error
	Get(ctx context.Context, key string) (io.ReadCloser, bucket.ObjectInfo, error)
	Remove(ctx context.Context, key string) error
	HealthCheck(ctx context.Context) error
}

// Deps are the shared dependencies for both HTTP surfaces.
type Deps struct {
	Store          Store
	Objects        ObjectStore
	Logger         *slog.Logger
	MaxUploadBytes int64
	MaxFiles       int
}

// NewServer builds an *http.Server with sane timeouts. There is deliberately no
// WriteTimeout: attachment downloads stream bodies of arbitrary size.
func NewServer(addr string, h http.Handler) *http.Server {
	return &http.Server{
		Addr:              addr,
		Handler:           h,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
}
