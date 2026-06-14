-- 0001_init.sql  (pure DDL; the runner wraps each statement in one transaction.
-- FK enforcement and WAL are connection pragmas set in the DSN, not here.)

CREATE TABLE receipts (
    id              TEXT    PRIMARY KEY NOT NULL,        -- UUID
    title           TEXT    NOT NULL DEFAULT '',
    description     TEXT    NOT NULL DEFAULT '',
    merchant        TEXT    NOT NULL DEFAULT '',
    purchase_date   TEXT    NOT NULL,                    -- ISO-8601 UTC
    amount_minor    INTEGER NOT NULL DEFAULT 0,          -- integer minor units (cents)
    currency        TEXT    NOT NULL DEFAULT 'EUR',
    note            TEXT    NOT NULL DEFAULT '',
    warranty_until  TEXT,                                -- NULLABLE ISO-8601 UTC; NULL = none
    uploader_email  TEXT    NOT NULL,
    created_at      TEXT    NOT NULL,                    -- ISO-8601 UTC
    updated_at      TEXT    NOT NULL                     -- ISO-8601 UTC
);

CREATE TABLE tags (
    id   TEXT PRIMARY KEY NOT NULL,
    name TEXT NOT NULL                                   -- normalized: lower(trim(name))
);

CREATE UNIQUE INDEX ux_tags_name ON tags(name);

CREATE TABLE receipt_tags (
    receipt_id TEXT NOT NULL,
    tag_id     TEXT NOT NULL,
    PRIMARY KEY (receipt_id, tag_id),
    FOREIGN KEY (receipt_id) REFERENCES receipts(id) ON DELETE CASCADE,
    FOREIGN KEY (tag_id)     REFERENCES tags(id)     ON DELETE CASCADE
);

CREATE INDEX ix_receipt_tags_tag ON receipt_tags(tag_id);

CREATE TABLE attachments (
    id           TEXT    PRIMARY KEY NOT NULL,           -- UUID
    receipt_id   TEXT    NOT NULL,
    object_key   TEXT    NOT NULL,                       -- "<receiptID>/<attachmentID>"
    filename     TEXT    NOT NULL DEFAULT '',
    content_type TEXT    NOT NULL DEFAULT '',
    kind         TEXT    NOT NULL,                       -- 'image' | 'pdf'
    size_bytes   INTEGER NOT NULL DEFAULT 0,
    created_at   TEXT    NOT NULL,
    FOREIGN KEY (receipt_id) REFERENCES receipts(id) ON DELETE CASCADE
);

CREATE UNIQUE INDEX ux_attachments_object_key ON attachments(object_key);
CREATE INDEX ix_attachments_receipt ON attachments(receipt_id);

CREATE INDEX ix_receipts_purchase_date  ON receipts(purchase_date);
CREATE INDEX ix_receipts_warranty_until ON receipts(warranty_until);
CREATE INDEX ix_receipts_uploader       ON receipts(uploader_email);
CREATE INDEX ix_receipts_created_at     ON receipts(created_at);
