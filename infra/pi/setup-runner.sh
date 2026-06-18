#!/usr/bin/env bash
#
# setup-runner.sh — install & register a GitHub Actions self-hosted runner on
# this Pi so pushes to main auto-deploy the stack (.github/workflows/deploy.yml).
#
# Run this ON THE PI. First get a registration token (valid ~1h) from:
#   https://github.com/DankersW/home-lab/settings/actions/runners/new
# then:
#   ./setup-runner.sh <REGISTRATION_TOKEN>
#
set -euo pipefail

readonly REPO_URL="https://github.com/DankersW/home-lab"
readonly RUNNER_DIR="${RUNNER_DIR:-$HOME/actions-runner}"
readonly RUNNER_NAME="${RUNNER_NAME:-homelab-pi}"
readonly RUNNER_LABELS="${RUNNER_LABELS:-self-hosted,Linux,ARM64}"

die() { echo "error: $*" >&2; exit 1; }

token="${1:-${RUNNER_TOKEN:-}}"
[ -n "$token" ] || die "usage: setup-runner.sh <REGISTRATION_TOKEN>
get one at ${REPO_URL}/settings/actions/runners/new"

[ "$(id -u)" -ne 0 ] || die "run as your normal user (wouter), not root — the runner refuses to run as root"

echo "Installing prerequisites (git, make) ..."
sudo apt-get update -y
sudo apt-get install -y git make

if [ ! -x "$RUNNER_DIR/config.sh" ]; then
  ver="$(curl -fsSL https://api.github.com/repos/actions/runner/releases/latest \
        | grep -oP '"tag_name":\s*"v\K[^"]+')"
  [ -n "$ver" ] || die "could not determine latest runner version"
  echo "Installing actions-runner v$ver -> $RUNNER_DIR"
  mkdir -p "$RUNNER_DIR"
  curl -fsSL -o /tmp/actions-runner.tar.gz \
    "https://github.com/actions/runner/releases/download/v${ver}/actions-runner-linux-arm64-${ver}.tar.gz"
  tar -xzf /tmp/actions-runner.tar.gz -C "$RUNNER_DIR"
  rm -f /tmp/actions-runner.tar.gz
fi

cd "$RUNNER_DIR"

# --replace makes re-running idempotent (re-registers the same runner name).
./config.sh --unattended --replace \
  --url "$REPO_URL" \
  --token "$token" \
  --name "$RUNNER_NAME" \
  --labels "$RUNNER_LABELS"

# Run as a service (survives reboots) under the current docker-group user.
sudo ./svc.sh install "$USER"
sudo ./svc.sh start
sudo ./svc.sh status || true

echo
echo "Runner '$RUNNER_NAME' registered and running."
echo "Verify at ${REPO_URL}/settings/actions/runners, then push to main to deploy."
