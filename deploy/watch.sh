#!/usr/bin/env bash
set -euo pipefail
remote="${AEGO_REMOTE:-https://git.akarpov.ru/sanspie/aego.git}"
api="${AEGO_API:-https://git.akarpov.ru/api/v1/repos/sanspie/aego}"
work="${AEGO_WORK:-$HOME/aego}"
bare="${AEGO_BARE:-$HOME/aego.git}"
state="${XDG_STATE_HOME:-$HOME/.local/state}/aego-deploy"
mkdir -p "$state"
exec 9>"$state/watch.lock"
flock -n 9 || exit 0

sha=$(git ls-remote "$remote" refs/heads/master | cut -f1)
[[ "$sha" =~ ^[0-9a-f]{40}$ ]] || exit 0
[ "$(cat "$state/deployed" 2>/dev/null)" = "$sha" ] && exit 0

ci=$(curl -fsS -m 30 "$api/actions/tasks?limit=50" | python3 -c '
import json, sys
sha = sys.argv[1]
need = ("checks", "build (amd64, linux")
tasks = [t for t in json.load(sys.stdin)["workflow_runs"] if t.get("head_sha") == sha and t.get("workflow_id") == "ci.yml"]
got = {n: [t["status"] for t in tasks if t["name"].startswith(n)] for n in need}
if any(s in ("failure", "cancelled") for v in got.values() for s in v):
    print("failure")
elif all("success" in v for v in got.values()):
    print("success")
else:
    print("pending")
' "$sha")

case "$ci" in
success) ;;
failure)
  if [ "$(cat "$state/failed" 2>/dev/null)" != "$sha" ]; then
    echo "CI failed for ${sha:0:7}, not deploying"
    echo "$sha" >"$state/failed"
  fi
  exit 0
  ;;
*) exit 0 ;;
esac

git -C "$bare" fetch -q "$remote" "+refs/heads/master:refs/heads/master"
[ -d "$work/.git" ] || git clone -q "$bare" "$work"
git -C "$work" fetch -q origin
git -C "$work" checkout -q -f --detach "$sha"
"$work/deploy/deploy.sh" "$sha"
