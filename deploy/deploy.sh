#!/usr/bin/env bash
set -euo pipefail
ref="${1:-master}"
work="${AEGO_WORK:-$HOME/aego}"
state="${XDG_STATE_HOME:-$HOME/.local/state}/aego-deploy"
mkdir -p "$state"
exec 8>"$state/deploy.lock"
flock 8
cd "$work"
unset GIT_DIR
git fetch -q origin
if [[ "$ref" =~ ^[0-9a-f]{40}$ ]]; then
  git checkout -q -f --detach "$ref"
else
  git checkout -q -f --detach "origin/$ref"
fi
sha=$(git rev-parse HEAD)
docker compose -f deploy/compose.yml build
docker compose -f deploy/compose.yml up -d --wait --remove-orphans
docker image prune -f >/dev/null
echo "$sha" >"$state/deployed"
echo "deployed ${sha:0:7}"
