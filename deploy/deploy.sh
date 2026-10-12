#!/usr/bin/env bash
set -euo pipefail
ref="${1:-master}"
work="${AEGO_WORK:-$HOME/aego}"
cd "$work"
unset GIT_DIR
git fetch -q origin
git checkout -q -f --detach "origin/$ref"
sha=$(git rev-parse --short HEAD)
docker compose -f deploy/compose.yml build
docker compose -f deploy/compose.yml up -d --wait --remove-orphans
docker image prune -f >/dev/null
echo "deployed $sha"
