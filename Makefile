APP_SERVICE_NAME=paperless_app

.PHONY: bootstrap
bootstrap:
	mkdir -p secrets
	mkdir -p pgdata
	mkdir -p paperless/data
	mkdir -p paperless/media
	mkdir -p paperless/consume
	mkdir -p paperless/export
	mkdir -p homer/assets
	$(MAKE) -s init-secrets

.PHONY: init-secrets
init-secrets: 
	@echo "--- Generating secrets (if needed) ---"
	@if [ ! -f "secrets/db_password" ]; then \
		openssl rand -hex 16 | tr -d '\n' > secrets/db_password; \
	fi
	@if [ ! -f "secrets/paperless_secret_key" ]; then \
		openssl rand -hex 32 | tr -d '\n' > secrets/paperless_secret_key; \
	fi
	@if [ ! -f "secrets/cloudflared_token" ]; then \
		mkdir -p secrets/cloudflared_token; \
	fi
	@if [ ! -f "secrets/paperless_admin_password" ]; then \
		openssl rand -hex 24 | tr -d '\n' > secrets/paperless_admin_password; \
	fi
	@echo "--- Done with secrets ---"


.PHONY: create-user
create-user:
	@echo "--- Creating Paperless Superuser ---"
	@echo "Please follow the prompts to create your admin account."
	@echo "(If this hangs or fails, wait ~30 seconds for the database to be ready and try again)"
	docker compose exec $(APP_SERVICE_NAME) createsuperuser


.PHONY: up
up:
	docker compose up -d

stop:
	docker compose down

logs:
	docker compose logs -f
