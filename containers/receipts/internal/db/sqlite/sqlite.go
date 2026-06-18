// Package sqlite implements the receipt metadata store on a single SQLite file
// using the pure-Go modernc.org/sqlite driver (no cgo).
package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/DankersW/home-lab/containers/receipts/internal/id"
	"github.com/DankersW/home-lab/containers/receipts/internal/receipt"

	_ "modernc.org/sqlite"
)

// DB is the concrete metadata repository. It exposes no interface; consumers
// declare the narrow interface they need.
type DB struct {
	sql *sql.DB
}

// dsnFmt sets the connection pragmas: WAL for concurrent reads, FK enforcement
// (off by default in SQLite), a busy timeout, and NORMAL sync (safe with WAL).
const dsnFmt = "file:%s?" +
	"_pragma=journal_mode(WAL)&" +
	"_pragma=foreign_keys(ON)&" +
	"_pragma=busy_timeout(5000)&" +
	"_pragma=synchronous(NORMAL)"

const selectReceiptCols = `SELECT id, title, description, merchant, purchase_date,
	amount_minor, currency, note, uploader_email, created_at, updated_at`

// likeEscaper escapes the LIKE wildcards (and the escape char itself) in a search
// token so it matches literally. Used with an `ESCAPE '\'` clause.
var likeEscaper = strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)

// Open opens (creating if needed) the database at path, applies all embedded
// migrations, and returns a ready repository.
func Open(ctx context.Context, path string) (*DB, error) {
	sqldb, err := sql.Open("sqlite", fmt.Sprintf(dsnFmt, path))
	if err != nil {
		return nil, fmt.Errorf("sqlite: open %q: %w", path, err)
	}
	// WAL permits one writer; a single connection serializes writes and avoids
	// SQLITE_BUSY entirely at this scale.
	sqldb.SetMaxOpenConns(1)
	if err := sqldb.PingContext(ctx); err != nil {
		_ = sqldb.Close()
		return nil, fmt.Errorf("sqlite: ping: %w", err)
	}
	if err := migrate(ctx, sqldb); err != nil {
		_ = sqldb.Close()
		return nil, err
	}
	return &DB{sql: sqldb}, nil
}

// Close releases the database.
func (d *DB) Close() error { return d.sql.Close() }

// Ping verifies connectivity.
func (d *DB) Ping(ctx context.Context) error {
	if err := d.sql.PingContext(ctx); err != nil {
		return fmt.Errorf("sqlite: ping: %w", err)
	}
	return nil
}

// ---- Receipts ----

// CreateReceipt inserts r and links r.Tags (by ID), in one transaction. The
// caller is responsible for generating r.ID and the timestamps and for
// resolving r.Tags via EnsureTags.
func (d *DB) CreateReceipt(ctx context.Context, r *receipt.Receipt) error {
	tx, err := d.sql.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("sqlite: begin create receipt: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO receipts
			(id, title, description, merchant, purchase_date, amount_minor, currency,
			 note, uploader_email, created_at, updated_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?)`,
		r.ID, r.Title, r.Description, r.Merchant, fmtTime(r.PurchaseDate),
		r.Amount.AmountMinor, r.Amount.Currency, r.Note,
		r.UploaderEmail, fmtTime(r.CreatedAt), fmtTime(r.UpdatedAt),
	); err != nil {
		return fmt.Errorf("sqlite: insert receipt: %w", err)
	}
	if err := linkTags(ctx, tx, r.ID, r.Tags); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("sqlite: commit create receipt: %w", err)
	}
	return nil
}

// GetReceipt returns the receipt with its tags and attachments hydrated.
// It returns receipt.ErrNotFound when no row matches.
func (d *DB) GetReceipt(ctx context.Context, recID string) (*receipt.Receipt, error) {
	row := d.sql.QueryRowContext(ctx, selectReceiptCols+` FROM receipts WHERE id = ?`, recID)
	r, err := scanReceipt(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, receipt.ErrNotFound
		}
		return nil, fmt.Errorf("sqlite: get receipt: %w", err)
	}
	if r.Tags, err = d.tagsForReceipt(ctx, recID); err != nil {
		return nil, err
	}
	if r.Attachments, err = d.ListAttachments(ctx, recID); err != nil {
		return nil, err
	}
	return r, nil
}

// UpdateReceipt updates the mutable scalar fields and replaces the tag set,
// bumping updated_at. The uploader is never changed. Returns ErrNotFound if the
// receipt does not exist.
func (d *DB) UpdateReceipt(ctx context.Context, r *receipt.Receipt) error {
	tx, err := d.sql.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("sqlite: begin update receipt: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	res, err := tx.ExecContext(ctx, `
		UPDATE receipts SET title=?, description=?, merchant=?, purchase_date=?,
			amount_minor=?, currency=?, note=?, updated_at=?
		WHERE id=?`,
		r.Title, r.Description, r.Merchant, fmtTime(r.PurchaseDate),
		r.Amount.AmountMinor, r.Amount.Currency, r.Note,
		fmtTime(r.UpdatedAt), r.ID,
	)
	if err != nil {
		return fmt.Errorf("sqlite: update receipt: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return receipt.ErrNotFound
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM receipt_tags WHERE receipt_id=?`, r.ID); err != nil {
		return fmt.Errorf("sqlite: clear tags: %w", err)
	}
	if err := linkTags(ctx, tx, r.ID, r.Tags); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("sqlite: commit update receipt: %w", err)
	}
	return nil
}

// DeleteReceipt removes the receipt (cascading to receipt_tags and attachment
// rows) and returns the object keys of its attachments so the caller can purge
// the bytes from the object store. Returns ErrNotFound if absent.
func (d *DB) DeleteReceipt(ctx context.Context, recID string) (objectKeys []string, err error) {
	atts, err := d.ListAttachments(ctx, recID)
	if err != nil {
		return nil, err
	}
	res, err := d.sql.ExecContext(ctx, `DELETE FROM receipts WHERE id=?`, recID)
	if err != nil {
		return nil, fmt.Errorf("sqlite: delete receipt: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil, receipt.ErrNotFound
	}
	keys := make([]string, 0, len(atts))
	for _, a := range atts {
		keys = append(keys, a.ObjectKey)
	}
	return keys, nil
}

// ListReceipts runs the dynamic filter query, returning lean rows (tags and
// attachments are NOT hydrated) ordered newest first.
func (d *DB) ListReceipts(ctx context.Context, q receipt.ReceiptQuery) ([]receipt.Receipt, error) {
	var sb strings.Builder
	sb.WriteString(selectReceiptCols + ` FROM receipts r WHERE 1=1`)
	var args []any

	// Every whitespace-separated token must match somewhere in the title,
	// merchant, note, or a tag name (LIKE is case-insensitive for ASCII). Tokens
	// are treated as literal substrings: LIKE metacharacters are escaped so a
	// user typing "%" or "_" does not match as a wildcard.
	for _, tok := range strings.Fields(q.Text) {
		sb.WriteString(` AND (r.title LIKE ? ESCAPE '\' OR r.merchant LIKE ? ESCAPE '\' OR r.note LIKE ? ESCAPE '\'` +
			` OR EXISTS (SELECT 1 FROM receipt_tags rt JOIN tags t ON t.id = rt.tag_id` +
			` WHERE rt.receipt_id = r.id AND t.name LIKE ? ESCAPE '\'))`)
		like := "%" + likeEscaper.Replace(tok) + "%"
		args = append(args, like, like, like, like)
	}
	if len(q.TagIDs) > 0 {
		sb.WriteString(` AND r.id IN (SELECT receipt_id FROM receipt_tags WHERE tag_id IN (`)
		for i, tid := range q.TagIDs {
			if i > 0 {
				sb.WriteString(",")
			}
			sb.WriteString("?")
			args = append(args, tid)
		}
		sb.WriteString(`) GROUP BY receipt_id HAVING COUNT(DISTINCT tag_id) = ?)`)
		args = append(args, len(q.TagIDs))
	}
	if q.PurchaseFrom != nil {
		sb.WriteString(` AND r.purchase_date >= ?`)
		args = append(args, fmtTime(*q.PurchaseFrom))
	}
	if q.PurchaseTo != nil {
		sb.WriteString(` AND r.purchase_date <= ?`)
		args = append(args, fmtTime(*q.PurchaseTo))
	}
	if q.AmountMinMinor != nil {
		sb.WriteString(` AND r.amount_minor >= ?`)
		args = append(args, *q.AmountMinMinor)
	}
	if q.AmountMaxMinor != nil {
		sb.WriteString(` AND r.amount_minor <= ?`)
		args = append(args, *q.AmountMaxMinor)
	}
	if q.Currency != "" {
		sb.WriteString(` AND r.currency = ?`)
		args = append(args, q.Currency)
	}
	if q.UploaderEmail != "" {
		sb.WriteString(` AND r.uploader_email = ?`)
		args = append(args, q.UploaderEmail)
	}

	limit := q.Limit
	if limit <= 0 {
		limit = receipt.DefaultLimit
	}
	sb.WriteString(` ORDER BY r.created_at DESC LIMIT ?`)
	args = append(args, limit)

	rows, err := d.sql.QueryContext(ctx, sb.String(), args...)
	if err != nil {
		return nil, fmt.Errorf("sqlite: list receipts: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []receipt.Receipt
	for rows.Next() {
		r, err := scanReceipt(rows)
		if err != nil {
			return nil, fmt.Errorf("sqlite: scan receipt: %w", err)
		}
		out = append(out, *r)
	}
	return out, rows.Err()
}

// CountReceipts returns the total number of receipts, ignoring any filter. It is
// a cheap COUNT(*) used for the UI counters, which must stay accurate beyond the
// list query's DefaultLimit.
func (d *DB) CountReceipts(ctx context.Context) (int, error) {
	var n int
	if err := d.sql.QueryRowContext(ctx, `SELECT COUNT(*) FROM receipts`).Scan(&n); err != nil {
		return 0, fmt.Errorf("sqlite: count receipts: %w", err)
	}
	return n, nil
}

// ExportAll returns a portable dump of all receipts with tags (by name) and
// attachments (by object key) for the move/merge story.
func (d *DB) ExportAll(ctx context.Context) (receipt.Export, error) {
	lean, err := d.ListReceipts(ctx, receipt.ReceiptQuery{Limit: 1 << 30})
	if err != nil {
		return receipt.Export{}, err
	}
	exp := receipt.Export{Version: receipt.ExportVersion, ExportedAt: time.Now().UTC()}
	for i := range lean {
		full, err := d.GetReceipt(ctx, lean[i].ID)
		if err != nil {
			return receipt.Export{}, err
		}
		er := receipt.ExportReceipt{
			ID: full.ID, Title: full.Title, Description: full.Description, Merchant: full.Merchant,
			PurchaseDate: full.PurchaseDate, AmountMinor: full.Amount.AmountMinor, Currency: full.Amount.Currency,
			Note: full.Note, UploaderEmail: full.UploaderEmail,
			CreatedAt: full.CreatedAt, UpdatedAt: full.UpdatedAt,
		}
		for _, t := range full.Tags {
			er.Tags = append(er.Tags, t.Name)
		}
		for _, a := range full.Attachments {
			er.Attachments = append(er.Attachments, receipt.ExportAttachment{
				ObjectKey: a.ObjectKey, Filename: a.Filename, ContentType: a.ContentType,
				Kind: string(a.Kind), SizeBytes: a.SizeBytes,
			})
		}
		exp.Receipts = append(exp.Receipts, er)
	}
	return exp, nil
}

// ---- Attachments (metadata only; bytes live in the object store) ----

// Attach records attachment metadata.
func (d *DB) Attach(ctx context.Context, a *receipt.Attachment) error {
	_, err := d.sql.ExecContext(ctx, `
		INSERT INTO attachments
			(id, receipt_id, object_key, filename, content_type, kind, size_bytes, created_at)
		VALUES (?,?,?,?,?,?,?,?)`,
		a.ID, a.ReceiptID, a.ObjectKey, a.Filename, a.ContentType, string(a.Kind), a.SizeBytes, fmtTime(a.CreatedAt))
	if err != nil {
		return fmt.Errorf("sqlite: insert attachment: %w", err)
	}
	return nil
}

// Detach removes an attachment row and returns its object key for byte cleanup.
func (d *DB) Detach(ctx context.Context, attachmentID string) (objectKey string, err error) {
	var key string
	err = d.sql.QueryRowContext(ctx,
		`SELECT object_key FROM attachments WHERE id=?`, attachmentID).Scan(&key)
	if errors.Is(err, sql.ErrNoRows) {
		return "", receipt.ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("sqlite: lookup attachment: %w", err)
	}
	if _, err := d.sql.ExecContext(ctx, `DELETE FROM attachments WHERE id=?`, attachmentID); err != nil {
		return "", fmt.Errorf("sqlite: delete attachment: %w", err)
	}
	return key, nil
}

// GetAttachment fetches one attachment, scoped to its receipt.
func (d *DB) GetAttachment(ctx context.Context, receiptID, attID string) (*receipt.Attachment, error) {
	row := d.sql.QueryRowContext(ctx, `
		SELECT id, receipt_id, object_key, filename, content_type, kind, size_bytes, created_at
		FROM attachments WHERE id=? AND receipt_id=?`, attID, receiptID)
	a, err := scanAttachment(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, receipt.ErrNotFound
		}
		return nil, fmt.Errorf("sqlite: get attachment: %w", err)
	}
	return a, nil
}

// ListAttachments returns a receipt's attachments oldest first.
func (d *DB) ListAttachments(ctx context.Context, receiptID string) ([]receipt.Attachment, error) {
	rows, err := d.sql.QueryContext(ctx, `
		SELECT id, receipt_id, object_key, filename, content_type, kind, size_bytes, created_at
		FROM attachments WHERE receipt_id=? ORDER BY created_at`, receiptID)
	if err != nil {
		return nil, fmt.Errorf("sqlite: list attachments: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []receipt.Attachment
	for rows.Next() {
		a, err := scanAttachment(rows)
		if err != nil {
			return nil, fmt.Errorf("sqlite: scan attachment: %w", err)
		}
		out = append(out, *a)
	}
	return out, rows.Err()
}

// AttachmentSummaries returns a per-receipt rollup (count + first image) for the
// given receipt IDs in a single query, so list cards render without hydrating
// every attachment. Receipts with no attachments are absent from the map.
func (d *DB) AttachmentSummaries(ctx context.Context, receiptIDs []string) (map[string]receipt.AttachmentSummary, error) {
	out := make(map[string]receipt.AttachmentSummary, len(receiptIDs))
	if len(receiptIDs) == 0 {
		return out, nil
	}
	var sb strings.Builder
	sb.WriteString(`
		SELECT a.receipt_id, COUNT(*),
			(SELECT i.id FROM attachments i
			 WHERE i.receipt_id = a.receipt_id AND i.kind = 'image'
			 ORDER BY i.created_at LIMIT 1)
		FROM attachments a WHERE a.receipt_id IN (`)
	args := make([]any, 0, len(receiptIDs))
	for i, rid := range receiptIDs {
		if i > 0 {
			sb.WriteString(",")
		}
		sb.WriteString("?")
		args = append(args, rid)
	}
	sb.WriteString(`) GROUP BY a.receipt_id`)

	rows, err := d.sql.QueryContext(ctx, sb.String(), args...)
	if err != nil {
		return nil, fmt.Errorf("sqlite: attachment summaries: %w", err)
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var (
			receiptID string
			count     int
			firstImg  sql.NullString
		)
		if err := rows.Scan(&receiptID, &count, &firstImg); err != nil {
			return nil, fmt.Errorf("sqlite: scan attachment summary: %w", err)
		}
		out[receiptID] = receipt.AttachmentSummary{Count: count, FirstImageID: firstImg.String}
	}
	return out, rows.Err()
}

// ---- Tags ----

// EnsureTags normalizes names (lower + trim), upserts each, and returns the
// resolved tags with their IDs. Blank and duplicate names are skipped.
func (d *DB) EnsureTags(ctx context.Context, names []string) ([]receipt.Tag, error) {
	tx, err := d.sql.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("sqlite: begin ensure tags: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	seen := make(map[string]bool, len(names))
	var out []receipt.Tag
	for _, raw := range names {
		name := normalizeTag(raw)
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO tags (id, name) VALUES (?, ?) ON CONFLICT(name) DO NOTHING`,
			id.New(), name); err != nil {
			return nil, fmt.Errorf("sqlite: upsert tag %q: %w", name, err)
		}
		var t receipt.Tag
		if err := tx.QueryRowContext(ctx, `SELECT id, name FROM tags WHERE name=?`, name).Scan(&t.ID, &t.Name); err != nil {
			return nil, fmt.Errorf("sqlite: resolve tag %q: %w", name, err)
		}
		out = append(out, t)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("sqlite: commit ensure tags: %w", err)
	}
	return out, nil
}

// ListTags returns all tags ordered by name.
func (d *DB) ListTags(ctx context.Context) ([]receipt.Tag, error) {
	rows, err := d.sql.QueryContext(ctx, `SELECT id, name FROM tags ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("sqlite: list tags: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []receipt.Tag
	for rows.Next() {
		var t receipt.Tag
		if err := rows.Scan(&t.ID, &t.Name); err != nil {
			return nil, fmt.Errorf("sqlite: scan tag: %w", err)
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// TagCounts returns every tag with the number of receipts carrying it, ordered
// by frequency (most-used first), then name. Tags with no receipts are included
// with a count of zero.
func (d *DB) TagCounts(ctx context.Context) ([]receipt.TagCount, error) {
	rows, err := d.sql.QueryContext(ctx, `
		SELECT t.id, t.name, COUNT(rt.receipt_id)
		FROM tags t
		LEFT JOIN receipt_tags rt ON rt.tag_id = t.id
		GROUP BY t.id, t.name
		ORDER BY COUNT(rt.receipt_id) DESC, t.name`)
	if err != nil {
		return nil, fmt.Errorf("sqlite: tag counts: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []receipt.TagCount
	for rows.Next() {
		var tc receipt.TagCount
		if err := rows.Scan(&tc.ID, &tc.Name, &tc.Count); err != nil {
			return nil, fmt.Errorf("sqlite: scan tag count: %w", err)
		}
		out = append(out, tc)
	}
	return out, rows.Err()
}

// DeleteTag removes a tag (cascading its receipt links). Returns ErrNotFound if absent.
func (d *DB) DeleteTag(ctx context.Context, tagID string) error {
	res, err := d.sql.ExecContext(ctx, `DELETE FROM tags WHERE id=?`, tagID)
	if err != nil {
		return fmt.Errorf("sqlite: delete tag: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return receipt.ErrNotFound
	}
	return nil
}

// SetReceiptTags replaces a receipt's tag set with the given tag IDs and
// returns the resulting tags.
func (d *DB) SetReceiptTags(ctx context.Context, receiptID string, tagIDs []string) ([]receipt.Tag, error) {
	tx, err := d.sql.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("sqlite: begin set tags: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `DELETE FROM receipt_tags WHERE receipt_id=?`, receiptID); err != nil {
		return nil, fmt.Errorf("sqlite: clear receipt tags: %w", err)
	}
	if err := linkTags(ctx, tx, receiptID, tagsFromIDs(tagIDs)); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("sqlite: commit set tags: %w", err)
	}
	return d.tagsForReceipt(ctx, receiptID)
}

func (d *DB) tagsForReceipt(ctx context.Context, receiptID string) ([]receipt.Tag, error) {
	rows, err := d.sql.QueryContext(ctx, `
		SELECT t.id, t.name FROM tags t
		JOIN receipt_tags rt ON rt.tag_id = t.id
		WHERE rt.receipt_id = ?
		ORDER BY t.name`, receiptID)
	if err != nil {
		return nil, fmt.Errorf("sqlite: tags for receipt: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []receipt.Tag
	for rows.Next() {
		var t receipt.Tag
		if err := rows.Scan(&t.ID, &t.Name); err != nil {
			return nil, fmt.Errorf("sqlite: scan tag: %w", err)
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// ---- helpers ----

type scanner interface {
	Scan(dest ...any) error
}

func scanReceipt(s scanner) (*receipt.Receipt, error) {
	var (
		r           receipt.Receipt
		purchase    string
		created     string
		updated     string
		currency    string
		amountMinor int64
	)
	if err := s.Scan(&r.ID, &r.Title, &r.Description, &r.Merchant, &purchase,
		&amountMinor, &currency, &r.Note, &r.UploaderEmail, &created, &updated); err != nil {
		return nil, err
	}
	var err error
	if r.PurchaseDate, err = parseTime(purchase); err != nil {
		return nil, fmt.Errorf("parse purchase_date: %w", err)
	}
	if r.CreatedAt, err = parseTime(created); err != nil {
		return nil, fmt.Errorf("parse created_at: %w", err)
	}
	if r.UpdatedAt, err = parseTime(updated); err != nil {
		return nil, fmt.Errorf("parse updated_at: %w", err)
	}
	r.Amount = receipt.Money{AmountMinor: amountMinor, Currency: currency}
	return &r, nil
}

func scanAttachment(s scanner) (*receipt.Attachment, error) {
	var (
		a       receipt.Attachment
		kind    string
		created string
	)
	if err := s.Scan(&a.ID, &a.ReceiptID, &a.ObjectKey, &a.Filename, &a.ContentType,
		&kind, &a.SizeBytes, &created); err != nil {
		return nil, err
	}
	a.Kind = receipt.AttachmentKind(kind)
	var err error
	if a.CreatedAt, err = parseTime(created); err != nil {
		return nil, fmt.Errorf("parse created_at: %w", err)
	}
	return &a, nil
}

func linkTags(ctx context.Context, tx *sql.Tx, receiptID string, tags []receipt.Tag) error {
	for _, t := range tags {
		if t.ID == "" {
			continue
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT OR IGNORE INTO receipt_tags (receipt_id, tag_id) VALUES (?, ?)`,
			receiptID, t.ID); err != nil {
			return fmt.Errorf("sqlite: link tag %s: %w", t.ID, err)
		}
	}
	return nil
}

func tagsFromIDs(ids []string) []receipt.Tag {
	tags := make([]receipt.Tag, 0, len(ids))
	for _, id := range ids {
		if strings.TrimSpace(id) == "" {
			continue
		}
		tags = append(tags, receipt.Tag{ID: id})
	}
	return tags
}

func normalizeTag(s string) string { return strings.ToLower(strings.TrimSpace(s)) }

func fmtTime(t time.Time) string { return t.UTC().Format(time.RFC3339) }

func parseTime(s string) (time.Time, error) {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}, err
	}
	return t.UTC(), nil
}

// Snapshot writes a consistent single-file copy of the database at srcPath to
// destPath using VACUUM INTO. It opens its own connection, so it is safe to run
// from a separate process while the server is using the database.
func Snapshot(ctx context.Context, srcPath, destPath string) error {
	db, err := sql.Open("sqlite", fmt.Sprintf("file:%s?_pragma=busy_timeout(10000)", srcPath))
	if err != nil {
		return fmt.Errorf("sqlite: open for snapshot: %w", err)
	}
	defer func() { _ = db.Close() }()

	// VACUUM INTO does not accept a bound parameter for the path; the path is
	// ours (not user input), so quote it with SQL-string escaping.
	stmt := "VACUUM INTO '" + strings.ReplaceAll(destPath, "'", "''") + "'"
	if _, err := db.ExecContext(ctx, stmt); err != nil {
		return fmt.Errorf("sqlite: vacuum into %q: %w", destPath, err)
	}
	return nil
}
