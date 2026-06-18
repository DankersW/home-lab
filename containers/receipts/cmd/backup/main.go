// Command backup snapshots the receipts database and object store. It is built
// into the same image as the server and invoked on a schedule, e.g.
//
//	docker compose run --rm -v /backup:/backup receipts /usr/local/bin/backup
package main

import (
	"context"
	"log/slog"
	"os"

	"github.com/DankersW/home-lab/containers/receipts/internal/backup"
	"github.com/DankersW/home-lab/containers/receipts/internal/config"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	cfg, err := config.Load()
	if err != nil {
		logger.Error("config", "err", err)
		os.Exit(1)
	}
	if _, err := backup.Run(context.Background(), cfg, logger); err != nil {
		logger.Error("backup failed", "err", err)
		os.Exit(1)
	}
}
