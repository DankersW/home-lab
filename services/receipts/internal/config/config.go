// Package config loads runtime configuration from the environment, including
// Docker secrets supplied via the *_FILE convention.
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Config holds all runtime configuration for the service and backup commands.
type Config struct {
	ListenAddr     string // e.g. ":8080"
	SQLitePath     string // e.g. "/data/receipts.db"
	TmpDir         string // multipart spill directory, e.g. "/data/tmp"; "" uses os.TempDir
	RequireAuth    bool   // when true, reject requests lacking the Cloudflare Access header
	DevUserEmail   string // fallback identity used only when RequireAuth is false
	MaxUploadBytes int64  // per-file upload cap
	MaxFiles       int    // max files accepted per receipt create

	Minio  MinioConfig
	Backup BackupConfig
}

// MinioConfig describes how to reach the object store.
type MinioConfig struct {
	Endpoint  string
	Bucket    string
	AccessKey string
	SecretKey string
	UseSSL    bool
}

// BackupConfig is consumed only by the backup command.
type BackupConfig struct {
	Dir        string
	RetainDays int
}

// Load reads configuration from the environment, resolves *_FILE secrets from
// disk, and fails fast on an unsafe or incomplete configuration.
func Load() (Config, error) {
	cfg := Config{
		ListenAddr:     envOr("LISTEN_ADDR", ":8080"),
		SQLitePath:     envOr("SQLITE_PATH", "./data/receipts.db"),
		TmpDir:         strings.TrimSpace(os.Getenv("TMPDIR")),
		RequireAuth:    envBool("REQUIRE_AUTH", true),
		DevUserEmail:   strings.TrimSpace(os.Getenv("DEV_USER_EMAIL")),
		MaxUploadBytes: envInt64("MAX_UPLOAD_BYTES", 25<<20),
		MaxFiles:       int(envInt64("MAX_FILES", 10)),
		Minio: MinioConfig{
			Endpoint: envOr("MINIO_ENDPOINT", "localhost:9000"),
			Bucket:   envOr("MINIO_BUCKET", "receipts"),
			UseSSL:   envBool("MINIO_USE_SSL", false),
		},
		Backup: BackupConfig{
			Dir:        envOr("BACKUP_DIR", "/backup"),
			RetainDays: int(envInt64("RETAIN_DAYS", 30)),
		},
	}

	var err error
	if cfg.Minio.AccessKey, err = readKeyOrFile("MINIO_ACCESS_KEY", "MINIO_ACCESS_KEY_FILE"); err != nil {
		return Config{}, err
	}
	if cfg.Minio.SecretKey, err = readKeyOrFile("MINIO_SECRET_KEY", "MINIO_SECRET_KEY_FILE"); err != nil {
		return Config{}, err
	}

	if !cfg.RequireAuth && cfg.DevUserEmail == "" {
		return Config{}, fmt.Errorf("config: REQUIRE_AUTH=false requires DEV_USER_EMAIL to be set (refusing to run with a synthetic identity)")
	}
	if cfg.Minio.AccessKey == "" || cfg.Minio.SecretKey == "" {
		return Config{}, fmt.Errorf("config: MINIO_ACCESS_KEY and MINIO_SECRET_KEY (or their *_FILE variants) are required")
	}
	return cfg, nil
}

// readKeyOrFile prefers the *_FILE variant (a Docker secret) and falls back to
// the plain environment variable (convenient for local development).
func readKeyOrFile(envName, fileEnvName string) (string, error) {
	if path := strings.TrimSpace(os.Getenv(fileEnvName)); path != "" {
		b, err := os.ReadFile(path)
		if err != nil {
			return "", fmt.Errorf("config: read %s (%s): %w", fileEnvName, path, err)
		}
		return strings.TrimSpace(string(b)), nil
	}
	return strings.TrimSpace(os.Getenv(envName)), nil
}

func envOr(name, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(name)); v != "" {
		return v
	}
	return fallback
}

func envBool(name string, fallback bool) bool {
	v := strings.TrimSpace(os.Getenv(name))
	if v == "" {
		return fallback
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return fallback
	}
	return b
}

func envInt64(name string, fallback int64) int64 {
	v := strings.TrimSpace(os.Getenv(name))
	if v == "" {
		return fallback
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return fallback
	}
	return n
}
