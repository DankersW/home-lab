// Package receipt defines the core domain types for the receipt tracker plus
// the validation helpers shared by the HTTP surfaces. It contains no
// persistence or transport logic.
package receipt

import (
	"errors"
	"fmt"
	"time"
)

// ErrNotFound is returned by stores when a requested entity does not exist.
var ErrNotFound = errors.New("receipt: not found")

// DefaultCurrency is the household currency. The UI is single-currency; the API
// may still override it per receipt.
const DefaultCurrency = "SEK"

// Money is an exact monetary amount: integer minor units plus an ISO-4217 code.
// Integers avoid floating-point rounding error.
type Money struct {
	AmountMinor int64  // e.g. 1299 == 12.99
	Currency    string // ISO-4217, e.g. "EUR"
}

// String renders the amount as "12.99 EUR".
func (m Money) String() string {
	sign, v := "", m.AmountMinor
	if v < 0 {
		sign, v = "-", -v
	}
	return fmt.Sprintf("%s%d.%02d %s", sign, v/100, v%100, m.Currency)
}

// AttachmentKind classifies a stored file so the UI can render it correctly.
type AttachmentKind string

const (
	KindImage AttachmentKind = "image" // photo of a paper receipt (JPEG/PNG/HEIC)
	KindPDF   AttachmentKind = "pdf"   // online invoice
)

// Valid reports whether k is a recognized attachment kind.
func (k AttachmentKind) Valid() bool { return k == KindImage || k == KindPDF }

// Receipt is the aggregate root. All time values are stored and compared in UTC.
type Receipt struct {
	ID            string
	Title         string
	Description   string
	Merchant      string
	PurchaseDate  time.Time
	Amount        Money
	Note          string
	UploaderEmail string
	CreatedAt     time.Time
	UpdatedAt     time.Time

	Tags        []Tag        // hydrated by Get; empty on lean list rows
	Attachments []Attachment // hydrated by Get; empty on lean list rows
}

// Tag is a household-shared label. Name is stored normalized (lower, trimmed).
type Tag struct {
	ID   string
	Name string
}

// TagCount is a tag with how many receipts carry it. Used by the tag-filter
// sidebar to show per-tag totals.
type TagCount struct {
	Tag
	Count int
}

// AttachmentSummary is a per-receipt attachment rollup for rendering list cards
// without hydrating every attachment: the total count plus the first image
// (if any) to use as the card thumbnail.
type AttachmentSummary struct {
	Count        int
	FirstImageID string // empty when the receipt has no image attachment
}

// Attachment is file metadata; the bytes live in the object store under ObjectKey.
type Attachment struct {
	ID          string
	ReceiptID   string
	ObjectKey   string
	Filename    string
	ContentType string
	Kind        AttachmentKind
	SizeBytes   int64
	CreatedAt   time.Time
}

// IsImage reports whether the attachment is an image (renderable inline as <img>).
func (a Attachment) IsImage() bool { return a.Kind == KindImage }
