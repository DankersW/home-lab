package receipt

import "time"

// DefaultLimit caps an otherwise-unbounded list query.
const DefaultLimit = 500

// ReceiptQuery is the filter set for listing receipts. The zero value lists
// everything (newest first). Pointer and slice fields are optional filters;
// nil/empty means "no constraint on this dimension".
type ReceiptQuery struct {
	Text             string     // substring match over title, merchant, note
	TagIDs           []string   // receipts carrying ALL of these tags
	PurchaseFrom     *time.Time // inclusive lower bound on purchase date
	PurchaseTo       *time.Time // inclusive upper bound
	AmountMinMinor   *int64     // inclusive lower bound on amount
	AmountMaxMinor   *int64     // inclusive upper bound
	Currency         string     // exact currency code
	WarrantyActiveAt *time.Time // warranty present and not expired at this instant
	UploaderEmail    string     // exact uploader match

	Limit int // max rows; <= 0 applies DefaultLimit
}
