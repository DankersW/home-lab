// Command receipts runs the receipt-tracker HTTP server.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/dankers/home-lab/services/receipts/internal/auth"
	"github.com/dankers/home-lab/services/receipts/internal/config"
	"github.com/dankers/home-lab/services/receipts/internal/db/bucket"
	"github.com/dankers/home-lab/services/receipts/internal/db/sqlite"
	"github.com/dankers/home-lab/services/receipts/internal/web"
	"github.com/dankers/home-lab/services/receipts/internal/web/api"
	"github.com/dankers/home-lab/services/receipts/internal/web/ui"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if err := run(logger); err != nil {
		logger.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	ctx := context.Background()

	if cfg.TmpDir != "" {
		if err := os.MkdirAll(cfg.TmpDir, 0o755); err != nil {
			return fmt.Errorf("create tmp dir %q: %w", cfg.TmpDir, err)
		}
	}

	store, err := sqlite.Open(ctx, cfg.SQLitePath)
	if err != nil {
		return err
	}
	defer func() { _ = store.Close() }()

	objects, err := dialBucket(ctx, cfg, logger)
	if err != nil {
		return err
	}

	deps := web.Deps{
		Store:          store,
		Objects:        objects,
		Logger:         logger,
		MaxUploadBytes: cfg.MaxUploadBytes,
		MaxFiles:       cfg.MaxFiles,
	}

	srv := web.NewServer(cfg.ListenAddr, buildHandler(cfg, deps, logger))
	return serve(srv, logger)
}

// dialBucket connects to MinIO with a bounded retry, since depends_on only
// waits for the container to start, not for the S3 API to be ready.
func dialBucket(ctx context.Context, cfg config.Config, logger *slog.Logger) (*bucket.Store, error) {
	bcfg := bucket.Config{
		Endpoint:  cfg.Minio.Endpoint,
		AccessKey: cfg.Minio.AccessKey,
		SecretKey: cfg.Minio.SecretKey,
		Bucket:    cfg.Minio.Bucket,
		UseSSL:    cfg.Minio.UseSSL,
	}
	const attempts = 5
	var lastErr error
	for attempt := 1; attempt <= attempts; attempt++ {
		dialCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		store, err := bucket.New(dialCtx, bcfg)
		cancel()
		if err == nil {
			return store, nil
		}
		lastErr = err
		logger.Warn("object store not ready, retrying", "attempt", attempt, "err", err)
		time.Sleep(2 * time.Second)
	}
	return nil, fmt.Errorf("connect object store after %d attempts: %w", attempts, lastErr)
}

func buildHandler(cfg config.Config, deps web.Deps, logger *slog.Logger) http.Handler {
	uiHandler := ui.New(deps).Routes()
	apiHandler := api.New(deps).Routes()

	app := http.NewServeMux()
	app.Handle("/api/", http.StripPrefix("/api", apiHandler))
	app.Handle("/", uiHandler)

	maxRequest := deps.MaxUploadBytes*int64(deps.MaxFiles) + (1 << 20)
	authed := auth.Middleware(cfg.RequireAuth, cfg.DevUserEmail, logger, web.MaxBytes(maxRequest, app))

	root := http.NewServeMux()
	root.Handle("GET /healthz", web.Health(deps))
	root.Handle("GET /static/", ui.StaticHandler())
	root.Handle("/", authed)

	return web.Recover(logger, web.RequestLog(logger, web.SecurityHeaders(root)))
}

func serve(srv *http.Server, logger *slog.Logger) error {
	errCh := make(chan error, 1)
	go func() {
		logger.Info("listening", "addr", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	select {
	case err := <-errCh:
		return err
	case <-stop:
		logger.Info("shutting down")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return srv.Shutdown(shutdownCtx)
	}
}
