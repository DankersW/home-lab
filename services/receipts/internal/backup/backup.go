// Package backup implements the change-gated backup performed by cmd/backup.
// It snapshots the SQLite metadata database and, only when its content has
// changed since the last run, keeps a dated snapshot and additively copies the
// object store. Because every object write is paired with a database row
// mutation, the database content hash is a sound change signal for both stores.
package backup

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/dankers/home-lab/services/receipts/internal/config"
	"github.com/dankers/home-lab/services/receipts/internal/db/bucket"
	"github.com/dankers/home-lab/services/receipts/internal/db/sqlite"
)

// Run performs one backup cycle. It returns whether a new backup was taken
// (false means nothing changed since the previous run).
func Run(ctx context.Context, cfg config.Config, logger *slog.Logger) (bool, error) {
	dir := cfg.Backup.Dir
	objDir := filepath.Join(dir, "objects")
	if err := os.MkdirAll(objDir, 0o755); err != nil {
		return false, fmt.Errorf("backup: create dir %q: %w", dir, err)
	}

	// 1. Consistent snapshot to a temp file.
	tmp := filepath.Join(dir, ".snapshot.tmp")
	_ = os.Remove(tmp)
	if err := sqlite.Snapshot(ctx, cfg.SQLitePath, tmp); err != nil {
		return false, err
	}
	defer func() { _ = os.Remove(tmp) }()

	// 2. Change gate on the snapshot's content hash.
	sum, err := sha256File(tmp)
	if err != nil {
		return false, err
	}
	lastPath := filepath.Join(dir, "last.sha")
	if prev, _ := os.ReadFile(lastPath); strings.TrimSpace(string(prev)) == sum {
		logger.Info("backup: no change since last run; skipping")
		return false, nil
	}

	// 3. Keep the dated snapshot and record its hash.
	stamp := time.Now().UTC().Format("20060102T150405Z")
	dest := filepath.Join(dir, "receipts-"+stamp+".db")
	if err := moveOrCopy(tmp, dest); err != nil {
		return false, err
	}
	if err := os.WriteFile(lastPath, []byte(sum), 0o644); err != nil {
		return false, fmt.Errorf("backup: write last.sha: %w", err)
	}

	// 4. Additive object copy (never deletes locally, so accidental deletes stay recoverable).
	store, err := bucket.New(ctx, bucket.Config{
		Endpoint:  cfg.Minio.Endpoint,
		AccessKey: cfg.Minio.AccessKey,
		SecretKey: cfg.Minio.SecretKey,
		Bucket:    cfg.Minio.Bucket,
		UseSSL:    cfg.Minio.UseSSL,
	})
	if err != nil {
		return false, err
	}
	copied, err := mirrorObjects(ctx, store, objDir)
	if err != nil {
		return false, err
	}

	// 5. Prune snapshots beyond the retention window.
	pruned, err := prune(dir, cfg.Backup.RetainDays)
	if err != nil {
		return false, err
	}

	logger.Info("backup: complete",
		"snapshot", filepath.Base(dest), "objects_copied", copied, "snapshots_pruned", pruned)
	return true, nil
}

// mirrorObjects copies every bucket object missing from (or differing in size
// from) the local mirror. It returns the number of objects copied.
func mirrorObjects(ctx context.Context, store *bucket.Store, objDir string) (int, error) {
	metas, err := store.List(ctx)
	if err != nil {
		return 0, err
	}
	copied := 0
	for _, m := range metas {
		if strings.Contains(m.Key, "..") {
			return copied, fmt.Errorf("backup: refusing suspicious object key %q", m.Key)
		}
		dest := filepath.Join(objDir, filepath.FromSlash(m.Key))
		if fi, err := os.Stat(dest); err == nil && fi.Size() == m.Size {
			continue
		}
		if err := copyObject(ctx, store, m.Key, dest); err != nil {
			return copied, err
		}
		copied++
	}
	return copied, nil
}

func copyObject(ctx context.Context, store *bucket.Store, key, dest string) error {
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return fmt.Errorf("backup: mkdir for %q: %w", key, err)
	}
	rc, _, err := store.Get(ctx, key)
	if err != nil {
		return err
	}
	defer func() { _ = rc.Close() }()

	f, err := os.Create(dest)
	if err != nil {
		return fmt.Errorf("backup: create %q: %w", dest, err)
	}
	_, copyErr := io.Copy(f, rc)
	closeErr := f.Close()
	if copyErr != nil {
		return fmt.Errorf("backup: copy object %q: %w", key, copyErr)
	}
	if closeErr != nil {
		return fmt.Errorf("backup: close %q: %w", dest, closeErr)
	}
	return nil
}

func prune(dir string, retainDays int) (int, error) {
	if retainDays <= 0 {
		return 0, nil
	}
	cutoff := time.Now().Add(-time.Duration(retainDays) * 24 * time.Hour)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0, fmt.Errorf("backup: read dir: %w", err)
	}
	pruned := 0
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasPrefix(name, "receipts-") || !strings.HasSuffix(name, ".db") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		if info.ModTime().Before(cutoff) {
			if err := os.Remove(filepath.Join(dir, name)); err == nil {
				pruned++
			}
		}
	}
	return pruned, nil
}

func sha256File(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("backup: open %q: %w", path, err)
	}
	defer func() { _ = f.Close() }()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", fmt.Errorf("backup: hash %q: %w", path, err)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// moveOrCopy renames src to dst, falling back to a copy across filesystems.
func moveOrCopy(src, dst string) error {
	if err := os.Rename(src, dst); err == nil {
		return nil
	}
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("backup: open snapshot: %w", err)
	}
	defer func() { _ = in.Close() }()
	out, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("backup: create snapshot %q: %w", dst, err)
	}
	_, copyErr := io.Copy(out, in)
	closeErr := out.Close()
	if copyErr != nil {
		return fmt.Errorf("backup: write snapshot: %w", copyErr)
	}
	if closeErr != nil {
		return fmt.Errorf("backup: close snapshot: %w", closeErr)
	}
	return nil
}
