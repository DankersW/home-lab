#!/usr/bin/env bash
#
# deploy.sh — sync this repo into a persistent deploy dir on the host and
# (re)launch the stack. Run by the self-hosted GitHub Actions runner on the Pi
# (.github/workflows/deploy.yml); also safe to run by hand on the Pi.
#
set -euo pipefail

readonly REPO_URL="https://github.com/DankersW/home-lab"
readonly DEPLOY_DIR="${DEPLOY_DIR:-$HOME/home-lab}"
readonly REQUIRED_SECRETS=(cloudflared_credentials minio_access_key minio_secret_key)

die() { echo "error: $*" >&2; exit 1; }

# Persistent checkout: gitignored secrets/ and data/ live here and survive
# `git reset --hard`, which only touches tracked files.
sync_repo() {
  if [ ! -d "$DEPLOY_DIR/.git" ]; then
    echo "Cloning $REPO_URL -> $DEPLOY_DIR"
    git clone "$REPO_URL" "$DEPLOY_DIR"
  fi
  echo "Syncing $DEPLOY_DIR to ${GITHUB_SHA:-origin/main} ..."
  git -C "$DEPLOY_DIR" fetch --prune origin main
  git -C "$DEPLOY_DIR" reset --hard "${GITHUB_SHA:-FETCH_HEAD}"
}

check_secrets() {
  local missing=() s
  for s in "${REQUIRED_SECRETS[@]}"; do
    [ -s "$DEPLOY_DIR/infra/secrets/$s" ] || missing+=("$s")
  done
  [ "${#missing[@]}" -eq 0 ] || die "missing secrets in $DEPLOY_DIR/infra/secrets: ${missing[*]}
Run once on the Pi:
  cd $DEPLOY_DIR && make bootstrap
  # then add your tunnel credentials to infra/secrets/cloudflared_credentials (see README.md)"
}

deploy() {
  cd "$DEPLOY_DIR"
  echo "Building and starting the stack ..."
  docker compose up -d --build --remove-orphans
  docker image prune -f
  docker compose ps
}

main() {
  command -v git >/dev/null 2>&1 || die "git is required"
  command -v docker >/dev/null 2>&1 || die "docker is required"
  sync_repo
  check_secrets
  deploy
  echo "Deploy complete."
}

main "$@"
