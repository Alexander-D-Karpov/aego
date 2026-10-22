#!/bin/sh
# Attaches files to an existing Gitea release, for builds made on another machine (the macOS .dmg).
# Usage: tools/release/upload.sh <vX.Y.Z|nightly> <file>...
# Needs GITEA_TOKEN and GITEA_API (https://host/api/v1/repos/owner/name).
set -eu
tag=${1:?usage: upload.sh <tag> <file>...}
shift
: "${GITEA_TOKEN:?GITEA_TOKEN is not set}"
: "${GITEA_API:?GITEA_API is not set}"
api() {
	method=$1
	shift
	curl -fsS -m 1800 -X "$method" -H "Authorization: token $GITEA_TOKEN" "$@"
}
id=$(api GET "$GITEA_API/releases/tags/$tag" | sed -n 's/^{"id":\([0-9]*\),.*/\1/p')
[ -n "$id" ] || { echo "upload: no release $tag" >&2; exit 1; }
for f in "$@"; do
	n=$(basename "$f")
	old=$(api GET "$GITEA_API/releases/$id/assets" | tr '{' '\n' | sed -n "s/^\"id\":\([0-9]*\),\"name\":\"$n\".*/\1/p")
	[ -n "$old" ] && api DELETE "$GITEA_API/releases/$id/assets/$old" >/dev/null
	api POST "$GITEA_API/releases/$id/assets?name=$n" -F "attachment=@$f" >/dev/null
	echo "upload: attached $n to $tag"
done
