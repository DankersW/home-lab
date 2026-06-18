BACKUP_DIR ?= $(CURDIR)/backups

.PHONY: bootstrap
bootstrap:
	mkdir -p infra/secrets
	mkdir -p containers/receipts/data
	mkdir -p containers/receipts/data/tmp
	mkdir -p containers/minio/data
	mkdir -p containers/homer/assets
	$(MAKE) -s init-secrets

.PHONY: init-secrets
init-secrets:
	@echo "--- Generating secrets (if needed) ---"
	@if [ ! -f "infra/secrets/cloudflared_token" ]; then \
		mkdir -p infra/secrets/cloudflared_token; \
	fi
	@if [ ! -f "infra/secrets/minio_access_key" ]; then \
		openssl rand -hex 16 | tr -d '\n' > infra/secrets/minio_access_key; \
	fi
	@if [ ! -f "infra/secrets/minio_secret_key" ]; then \
		openssl rand -hex 24 | tr -d '\n' > infra/secrets/minio_secret_key; \
	fi
	@echo "--- Done with secrets ---"


.PHONY: up
up:
	docker compose up -d --build

stop:
	docker compose down

logs:
	docker compose logs -f


.PHONY: backup
backup:
	mkdir -p $(BACKUP_DIR)
	docker compose run --rm -v $(BACKUP_DIR):/backup receipts /usr/local/bin/backup


.PHONY: flash
flash:
	@[ -n "$(DEVICE)" ] || { echo "usage: make flash DEVICE=/dev/sdX"; exit 1; }
	bash $(CURDIR)/infra/pi/flash.sh $(DEVICE)
