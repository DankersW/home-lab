BACKUP_DIR ?= $(CURDIR)/backups

.PHONY: bootstrap
bootstrap:
	mkdir -p secrets
	mkdir -p services/receipts/data
	mkdir -p services/receipts/data/tmp
	mkdir -p minio/data
	mkdir -p homer/assets
	$(MAKE) -s init-secrets

.PHONY: init-secrets
init-secrets:
	@echo "--- Generating secrets (if needed) ---"
	@if [ ! -f "secrets/cloudflared_token" ]; then \
		mkdir -p secrets/cloudflared_token; \
	fi
	@if [ ! -f "secrets/minio_access_key" ]; then \
		openssl rand -hex 16 | tr -d '\n' > secrets/minio_access_key; \
	fi
	@if [ ! -f "secrets/minio_secret_key" ]; then \
		openssl rand -hex 24 | tr -d '\n' > secrets/minio_secret_key; \
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
	bash $(CURDIR)/pi/flash.sh $(DEVICE)
