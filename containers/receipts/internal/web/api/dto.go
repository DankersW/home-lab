package api

import (
	"fmt"
	"time"

	"github.com/DankersW/home-lab/containers/receipts/internal/receipt"
	"github.com/DankersW/home-lab/containers/receipts/internal/web"
)

const dateLayout = "2006-01-02"

// receiptRequest is the JSON body for creating or updating a receipt. Amount may
// be given as integer minor units (amount_minor) or a human string (amount).
type receiptRequest struct {
	Title        string   `json:"title"`
	Description  string   `json:"description"`
	Merchant     string   `json:"merchant"`
	PurchaseDate string   `json:"purchase_date"` // YYYY-MM-DD
	AmountMinor  *int64   `json:"amount_minor"`
	Amount       string   `json:"amount"` // alternative to amount_minor, e.g. "12.99"
	Currency     string   `json:"currency"`
	Note         string   `json:"note"`
	Tags         []string `json:"tags"`
}

// toReceipt validates the request and maps it to a domain receipt (without ID,
// timestamps, uploader, or tags — those are set by the handler).
func (req receiptRequest) toReceipt() (*receipt.Receipt, error) {
	if req.Merchant == "" {
		return nil, fmt.Errorf("%w: merchant is required", web.ErrValidation)
	}
	purchase, err := time.Parse(dateLayout, req.PurchaseDate)
	if err != nil {
		return nil, fmt.Errorf("%w: purchase_date must be YYYY-MM-DD", web.ErrValidation)
	}

	var amountMinor int64
	switch {
	case req.AmountMinor != nil:
		amountMinor = *req.AmountMinor
	case req.Amount != "":
		if amountMinor, err = receipt.ParseMoneyMinor(req.Amount); err != nil {
			return nil, fmt.Errorf("%w: %v", web.ErrValidation, err)
		}
	}

	currency := req.Currency
	if currency == "" {
		currency = receipt.DefaultCurrency
	}
	if len(currency) != 3 {
		return nil, fmt.Errorf("%w: currency must be a 3-letter code", web.ErrValidation)
	}

	return &receipt.Receipt{
		Title:        req.Title,
		Description:  req.Description,
		Merchant:     req.Merchant,
		PurchaseDate: purchase.UTC(),
		Amount:       receipt.Money{AmountMinor: amountMinor, Currency: currency},
		Note:         req.Note,
	}, nil
}

// receiptDTO is the JSON representation of a receipt.
type receiptDTO struct {
	ID            string          `json:"id"`
	Title         string          `json:"title"`
	Description   string          `json:"description"`
	Merchant      string          `json:"merchant"`
	PurchaseDate  string          `json:"purchase_date"`
	AmountMinor   int64           `json:"amount_minor"`
	Currency      string          `json:"currency"`
	Note          string          `json:"note"`
	UploaderEmail string          `json:"uploader_email"`
	CreatedAt     time.Time       `json:"created_at"`
	UpdatedAt     time.Time       `json:"updated_at"`
	Tags          []string        `json:"tags"`
	Attachments   []attachmentDTO `json:"attachments"`
}

type attachmentDTO struct {
	ID          string `json:"id"`
	Filename    string `json:"filename"`
	ContentType string `json:"content_type"`
	Kind        string `json:"kind"`
	SizeBytes   int64  `json:"size_bytes"`
	URL         string `json:"url"`
}

func toReceiptDTO(r *receipt.Receipt) receiptDTO {
	dto := receiptDTO{
		ID:            r.ID,
		Title:         r.Title,
		Description:   r.Description,
		Merchant:      r.Merchant,
		PurchaseDate:  r.PurchaseDate.UTC().Format(dateLayout),
		AmountMinor:   r.Amount.AmountMinor,
		Currency:      r.Amount.Currency,
		Note:          r.Note,
		UploaderEmail: r.UploaderEmail,
		CreatedAt:     r.CreatedAt,
		UpdatedAt:     r.UpdatedAt,
		Tags:          make([]string, 0, len(r.Tags)),
		Attachments:   make([]attachmentDTO, 0, len(r.Attachments)),
	}
	for _, t := range r.Tags {
		dto.Tags = append(dto.Tags, t.Name)
	}
	for _, a := range r.Attachments {
		dto.Attachments = append(dto.Attachments, attachmentDTO{
			ID:          a.ID,
			Filename:    a.Filename,
			ContentType: a.ContentType,
			Kind:        string(a.Kind),
			SizeBytes:   a.SizeBytes,
			URL:         fmt.Sprintf("/api/receipts/%s/attachments/%s", a.ReceiptID, a.ID),
		})
	}
	return dto
}
