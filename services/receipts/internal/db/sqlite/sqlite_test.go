package sqlite_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/dankers/home-lab/services/receipts/internal/db/sqlite"
	"github.com/dankers/home-lab/services/receipts/internal/id"
	"github.com/dankers/home-lab/services/receipts/internal/receipt"
	"github.com/stretchr/testify/require"
)

func openTestDB(t *testing.T) *sqlite.DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")
	db, err := sqlite.Open(context.Background(), path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func newReceipt(uploader, merchant string, amountMinor int64, purchase time.Time) *receipt.Receipt {
	now := time.Now().UTC().Truncate(time.Second)
	return &receipt.Receipt{
		ID:            id.New(),
		Title:         merchant + " purchase",
		Merchant:      merchant,
		PurchaseDate:  purchase.UTC().Truncate(time.Second),
		Amount:        receipt.Money{AmountMinor: amountMinor, Currency: "EUR"},
		Note:          "kept in garage",
		UploaderEmail: uploader,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
}

func TestMigrateIsIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "idem.db")
	db1, err := sqlite.Open(context.Background(), path)
	require.NoError(t, err)
	require.NoError(t, db1.Close())

	// Re-opening re-runs the migrator, which must be a no-op.
	db2, err := sqlite.Open(context.Background(), path)
	require.NoError(t, err)
	require.NoError(t, db2.Ping(context.Background()))
	require.NoError(t, db2.Close())
}

func TestEnsureTagsDeduplicatesAndNormalizes(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	tags, err := db.EnsureTags(ctx, []string{" Food ", "food", "DRINKS", ""})
	require.NoError(t, err)
	require.Len(t, tags, 2)

	// Re-ensuring returns the same IDs (upsert, not duplicate).
	again, err := db.EnsureTags(ctx, []string{"food", "drinks"})
	require.NoError(t, err)
	require.Len(t, again, 2)

	all, err := db.ListTags(ctx)
	require.NoError(t, err)
	require.Len(t, all, 2)
	require.Equal(t, "drinks", all[0].Name)
	require.Equal(t, "food", all[1].Name)
}

func TestCreateGetReceiptWithTagsAndAttachment(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	tags, err := db.EnsureTags(ctx, []string{"appliance", "kitchen"})
	require.NoError(t, err)

	r := newReceipt("wouter@example.com", "MediaMarkt", 49999, time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC))
	r.Tags = tags
	require.NoError(t, db.CreateReceipt(ctx, r))

	att := &receipt.Attachment{
		ID:          id.New(),
		ReceiptID:   r.ID,
		ObjectKey:   r.ID + "/" + id.New(),
		Filename:    "receipt.pdf",
		ContentType: "application/pdf",
		Kind:        receipt.KindPDF,
		SizeBytes:   1234,
		CreatedAt:   time.Now().UTC().Truncate(time.Second),
	}
	require.NoError(t, db.Attach(ctx, att))

	got, err := db.GetReceipt(ctx, r.ID)
	require.NoError(t, err)
	require.Equal(t, "MediaMarkt", got.Merchant)
	require.Equal(t, int64(49999), got.Amount.AmountMinor)
	require.Equal(t, "EUR", got.Amount.Currency)
	require.Len(t, got.Tags, 2)
	require.Len(t, got.Attachments, 1)
	require.Equal(t, "receipt.pdf", got.Attachments[0].Filename)

	_, err = db.GetReceipt(ctx, "missing")
	require.ErrorIs(t, err, receipt.ErrNotFound)
}

func TestListReceiptsFilters(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	food, err := db.EnsureTags(ctx, []string{"food"})
	require.NoError(t, err)
	electronics, err := db.EnsureTags(ctx, []string{"electronics"})
	require.NoError(t, err)

	groceries := newReceipt("wouter@example.com", "Albert Heijn", 2350, time.Date(2026, 1, 10, 0, 0, 0, 0, time.UTC))
	groceries.Tags = food
	require.NoError(t, db.CreateReceipt(ctx, groceries))

	tv := newReceipt("partner@example.com", "Coolblue", 89900, time.Date(2026, 2, 20, 0, 0, 0, 0, time.UTC))
	tv.Title = "OLED television"
	tv.Tags = electronics
	require.NoError(t, db.CreateReceipt(ctx, tv))

	all, err := db.ListReceipts(ctx, receipt.ReceiptQuery{})
	require.NoError(t, err)
	require.Len(t, all, 2)

	byText, err := db.ListReceipts(ctx, receipt.ReceiptQuery{Text: "television"})
	require.NoError(t, err)
	require.Len(t, byText, 1)
	require.Equal(t, "Coolblue", byText[0].Merchant)

	byTag, err := db.ListReceipts(ctx, receipt.ReceiptQuery{TagIDs: []string{food[0].ID}})
	require.NoError(t, err)
	require.Len(t, byTag, 1)
	require.Equal(t, "Albert Heijn", byTag[0].Merchant)

	byUploader, err := db.ListReceipts(ctx, receipt.ReceiptQuery{UploaderEmail: "partner@example.com"})
	require.NoError(t, err)
	require.Len(t, byUploader, 1)

	min := int64(50000)
	byAmount, err := db.ListReceipts(ctx, receipt.ReceiptQuery{AmountMinMinor: &min})
	require.NoError(t, err)
	require.Len(t, byAmount, 1)
	require.Equal(t, "Coolblue", byAmount[0].Merchant)

	from := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	byDate, err := db.ListReceipts(ctx, receipt.ReceiptQuery{PurchaseFrom: &from})
	require.NoError(t, err)
	require.Len(t, byDate, 1)
	require.Equal(t, "Coolblue", byDate[0].Merchant)
}

func TestUpdateAndDeleteReceipt(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	tags, err := db.EnsureTags(ctx, []string{"warranty"})
	require.NoError(t, err)
	r := newReceipt("wouter@example.com", "IKEA", 12999, time.Date(2026, 4, 4, 0, 0, 0, 0, time.UTC))
	r.Tags = tags
	require.NoError(t, db.CreateReceipt(ctx, r))

	att := &receipt.Attachment{
		ID: id.New(), ReceiptID: r.ID, ObjectKey: r.ID + "/" + id.New(),
		Filename: "img.jpg", ContentType: "image/jpeg", Kind: receipt.KindImage,
		SizeBytes: 10, CreatedAt: time.Now().UTC().Truncate(time.Second),
	}
	require.NoError(t, db.Attach(ctx, att))

	r.Merchant = "IKEA Stockholm"
	r.UpdatedAt = time.Now().UTC().Truncate(time.Second)
	r.Tags = nil // clear tags on update
	require.NoError(t, db.UpdateReceipt(ctx, r))

	got, err := db.GetReceipt(ctx, r.ID)
	require.NoError(t, err)
	require.Equal(t, "IKEA Stockholm", got.Merchant)
	require.Empty(t, got.Tags)

	missing := newReceipt("x@example.com", "Nope", 1, time.Now())
	require.ErrorIs(t, db.UpdateReceipt(ctx, missing), receipt.ErrNotFound)

	keys, err := db.DeleteReceipt(ctx, r.ID)
	require.NoError(t, err)
	require.Equal(t, []string{att.ObjectKey}, keys)

	_, err = db.GetReceipt(ctx, r.ID)
	require.ErrorIs(t, err, receipt.ErrNotFound)

	// The tag itself survives a receipt delete (only the link cascades).
	tagsLeft, err := db.ListTags(ctx)
	require.NoError(t, err)
	require.Len(t, tagsLeft, 1)
}

func TestExportAll(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	tags, err := db.EnsureTags(ctx, []string{"electronics"})
	require.NoError(t, err)
	r := newReceipt("wouter@example.com", "Coolblue", 89900, time.Date(2026, 2, 20, 0, 0, 0, 0, time.UTC))
	r.Tags = tags
	require.NoError(t, db.CreateReceipt(ctx, r))

	exp, err := db.ExportAll(ctx)
	require.NoError(t, err)
	require.Equal(t, receipt.ExportVersion, exp.Version)
	require.Len(t, exp.Receipts, 1)
	require.Equal(t, []string{"electronics"}, exp.Receipts[0].Tags)
}
