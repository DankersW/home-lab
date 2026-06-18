package sqlite_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/DankersW/home-lab/containers/receipts/internal/db/sqlite"
	"github.com/DankersW/home-lab/containers/receipts/internal/id"
	"github.com/DankersW/home-lab/containers/receipts/internal/receipt"
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

func TestListReceiptsTokenSearchIncludesTags(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	garden, err := db.EnsureTags(ctx, []string{"garden"})
	require.NoError(t, err)
	mower := newReceipt("wouter@example.com", "Bauhaus", 12490, time.Date(2025, 4, 22, 0, 0, 0, 0, time.UTC))
	mower.Title = "Husqvarna Automower"
	mower.Tags = garden
	require.NoError(t, db.CreateReceipt(ctx, mower))

	tv := newReceipt("wouter@example.com", "Elgiganten", 18990, time.Date(2024, 11, 2, 0, 0, 0, 0, time.UTC))
	tv.Title = "LG OLED"
	require.NoError(t, db.CreateReceipt(ctx, tv))

	// A token that only appears as a tag name still matches.
	byTag, err := db.ListReceipts(ctx, receipt.ReceiptQuery{Text: "garden"})
	require.NoError(t, err)
	require.Len(t, byTag, 1)
	require.Equal(t, "Bauhaus", byTag[0].Merchant)

	// Every whitespace-separated token must match (AND): merchant + tag.
	both, err := db.ListReceipts(ctx, receipt.ReceiptQuery{Text: "bauhaus garden"})
	require.NoError(t, err)
	require.Len(t, both, 1)

	none, err := db.ListReceipts(ctx, receipt.ReceiptQuery{Text: "bauhaus oled"})
	require.NoError(t, err)
	require.Empty(t, none)
}

func TestSearchEscapesLikeWildcards(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	a := newReceipt("wouter@example.com", "Store", 100, time.Now())
	a.Title = "Coffee 100 percent arabica"
	a.Note = ""
	require.NoError(t, db.CreateReceipt(ctx, a))
	b := newReceipt("wouter@example.com", "Other", 100, time.Now())
	b.Title = "Plain item"
	b.Note = ""
	require.NoError(t, db.CreateReceipt(ctx, b))

	// "%" is treated literally, not as a wildcard, so it matches nothing here.
	pct, err := db.ListReceipts(ctx, receipt.ReceiptQuery{Text: "%"})
	require.NoError(t, err)
	require.Empty(t, pct)

	// "_" likewise literal.
	und, err := db.ListReceipts(ctx, receipt.ReceiptQuery{Text: "_"})
	require.NoError(t, err)
	require.Empty(t, und)

	// Ordinary token still matches as a substring.
	hit, err := db.ListReceipts(ctx, receipt.ReceiptQuery{Text: "percent"})
	require.NoError(t, err)
	require.Len(t, hit, 1)
	require.Equal(t, "Store", hit[0].Merchant)
}

func TestTagCounts(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	living, err := db.EnsureTags(ctx, []string{"living room"})
	require.NoError(t, err)
	_, err = db.EnsureTags(ctx, []string{"garden"})
	require.NoError(t, err)

	for i := 0; i < 2; i++ {
		r := newReceipt("wouter@example.com", "Store", 100, time.Now())
		r.Tags = living
		require.NoError(t, db.CreateReceipt(ctx, r))
	}

	counts, err := db.TagCounts(ctx)
	require.NoError(t, err)
	require.Len(t, counts, 2)
	// Most-used first.
	require.Equal(t, "living room", counts[0].Name)
	require.Equal(t, 2, counts[0].Count)
	require.Equal(t, "garden", counts[1].Name)
	require.Equal(t, 0, counts[1].Count)
}

func TestAttachmentSummaries(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	r := newReceipt("wouter@example.com", "Store", 100, time.Now())
	require.NoError(t, db.CreateReceipt(ctx, r))

	pdf := &receipt.Attachment{
		ID: id.New(), ReceiptID: r.ID, ObjectKey: r.ID + "/" + id.New(),
		Filename: "invoice.pdf", ContentType: "application/pdf", Kind: receipt.KindPDF,
		SizeBytes: 1, CreatedAt: time.Now().UTC().Truncate(time.Second),
	}
	require.NoError(t, db.Attach(ctx, pdf))
	img := &receipt.Attachment{
		ID: id.New(), ReceiptID: r.ID, ObjectKey: r.ID + "/" + id.New(),
		Filename: "photo.jpg", ContentType: "image/jpeg", Kind: receipt.KindImage,
		SizeBytes: 1, CreatedAt: time.Now().UTC().Truncate(time.Second).Add(time.Second),
	}
	require.NoError(t, db.Attach(ctx, img))

	sums, err := db.AttachmentSummaries(ctx, []string{r.ID, "missing"})
	require.NoError(t, err)
	require.Len(t, sums, 1)
	require.Equal(t, 2, sums[r.ID].Count)
	require.Equal(t, img.ID, sums[r.ID].FirstImageID)

	empty, err := db.AttachmentSummaries(ctx, nil)
	require.NoError(t, err)
	require.Empty(t, empty)
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
