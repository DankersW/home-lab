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
	WarrantyUntil *time.Time // nil == no warranty tracked
	UploaderEmail string
	CreatedAt     time.Time
	UpdatedAt     time.Time

	Tags        []Tag        // hydrated by Get; empty on lean list rows
	Attachments []Attachment // hydrated by Get; empty on lean list rows
}

// WarrantyActive reports whether a warranty is present and still in force at t.
func (r Receipt) WarrantyActive(t time.Time) bool {
	return r.WarrantyUntil != nil && !r.WarrantyUntil.Before(t)
}

// Tag is a household-shared label. Name is stored normalized (lower, trimmed).
type Tag struct {
	ID   string
	Name string
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
