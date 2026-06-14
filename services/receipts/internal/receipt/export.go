package receipt

import "time"

// ExportVersion is the schema version of the JSON export document.
const ExportVersion = 1

// Export is a portable, self-describing dump of all receipt metadata. File
// bytes are referenced by object key, not inlined, so the object store is
// mirrored separately. Tags are exported by name so a future import can merge
// them across independently-created datasets.
type Export struct {
	Version    int             `json:"version"`
	ExportedAt time.Time       `json:"exported_at"`
	Receipts   []ExportReceipt `json:"receipts"`
}

// ExportReceipt is one receipt in an Export document.
type ExportReceipt struct {
	ID            string             `json:"id"`
	Title         string             `json:"title"`
	Description   string             `json:"description"`
	Merchant      string             `json:"merchant"`
	PurchaseDate  time.Time          `json:"purchase_date"`
	AmountMinor   int64              `json:"amount_minor"`
	Currency      string             `json:"currency"`
	Note          string             `json:"note"`
	WarrantyUntil *time.Time         `json:"warranty_until,omitempty"`
	UploaderEmail string             `json:"uploader_email"`
	CreatedAt     time.Time          `json:"created_at"`
	UpdatedAt     time.Time          `json:"updated_at"`
	Tags          []string           `json:"tags"`        // names, for merge-by-name on import
	Attachments   []ExportAttachment `json:"attachments"` // referenced by object key
}

// ExportAttachment references a stored file by its object key.
type ExportAttachment struct {
	ObjectKey   string `json:"object_key"`
	Filename    string `json:"filename"`
	ContentType string `json:"content_type"`
	Kind        string `json:"kind"`
	SizeBytes   int64  `json:"size_bytes"`
}
