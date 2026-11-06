#!/usr/bin/env python3
"""Mirror published Gitea releases to GitHub without changing git refs."""

import hashlib
import json
import os
from pathlib import Path
import re
import subprocess
import sys
import tempfile
from urllib.parse import quote, urlsplit
from urllib.request import Request, urlopen

REPOSITORY = "Alexander-D-Karpov/aego"
GITEA = "https://git.akarpov.ru/api/v1/repos/sanspie/aego"
GITEA_HOST = "git.akarpov.ru"
NATIVE_JOBS = {
    "windows amd64 native", "darwin arm64 native",
    "darwin amd64 native", "linux amd64 native",
}
TAG = re.compile(r"(?:nightly|stable|v[0-9A-Za-z][0-9A-Za-z.+_-]*)\Z")
STABLE = re.compile(r"v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\Z")
ASSET = re.compile(r"[A-Za-z0-9][A-Za-z0-9._-]*\Z")
SUM = re.compile(r"([0-9a-fA-F]{64}) [ *]([^\r\n]+)\Z")
MAX_ASSET = 1024 * 1024 * 1024


def gh(*args):
    result = subprocess.run(["gh", *args], stdout=subprocess.PIPE, stderr=subprocess.PIPE, check=False)
    if result.returncode:
        raise RuntimeError("GitHub CLI failed: " + " ".join(args[:2]))
    return result.stdout


def github(path):
    return json.loads(gh("api", "repos/" + REPOSITORY + "/" + path))


def source(path):
    req = Request(GITEA + "/" + path, headers={"Accept": "application/json"})
    with urlopen(req, timeout=30) as response:
        return json.load(response)


def pages(fetch, path, key=None):
    items = []
    for page in range(1, 21):
        data = fetch(path + ("&" if "?" in path else "?") + f"per_page=100&limit=100&page={page}")
        batch = data[key] if key else data
        if not isinstance(batch, list):
            raise ValueError("invalid API page")
        items.extend(batch)
        if len(batch) < 100:
            return items
    raise ValueError("too many API pages")


def ci_ready():
    source_sha = source("branches/master")["commit"]["id"]
    github_sha = github("branches/master")["commit"]["sha"]
    if source_sha != github_sha or not re.fullmatch(r"[0-9a-f]{40}", source_sha):
        print("Release mirror paused: master refs have not synced")
        return False
    runs = pages(github, "actions/workflows/github-native.yml/runs?branch=master", "workflow_runs")
    runs = [r for r in runs if r.get("head_sha") == source_sha and r.get("event") in ("push", "workflow_dispatch")]
    if not runs or runs[0].get("status") != "completed" or runs[0].get("conclusion") != "success":
        print("Release mirror paused: current GitHub native CI has not passed")
        return False
    run = runs[0]
    jobs = pages(github, f"actions/runs/{run['id']}/jobs?filter=all", "jobs")
    latest = {}
    attempt = run.get("run_attempt", 1)
    for job in jobs:
        name, number = job.get("name"), job.get("run_attempt", 1)
        if name in NATIVE_JOBS and isinstance(number, int) and number <= attempt:
            if name not in latest or number > latest[name].get("run_attempt", 1):
                latest[name] = job
    if set(latest) != NATIVE_JOBS or any(j.get("status") != "completed" or j.get("conclusion") != "success" for j in latest.values()):
        print("Release mirror paused: not all four native jobs passed")
        return False
    statuses = source("commits/" + source_sha + "/status").get("statuses", [])
    states = {s.get("context"): s.get("status") for s in statuses}
    checks = states.get("ci / checks (push)") == "success"
    builds = [v for k, v in states.items() if isinstance(k, str) and k.startswith("ci / build (") and k.endswith("(push)")]
    if not checks or not builds or any(state != "success" for state in builds):
        print("Release mirror paused: Gitea Linux CI has not passed")
        return False
    return True


def stable_version(release):
    match = STABLE.fullmatch(release["tag_name"])
    if not match or release.get("prerelease"):
        return None
    return tuple(int(x) for x in match.groups())


def is_prerelease(release):
    return bool(release.get("prerelease")) or stable_version(release) is None


def inventory(release):
    assets = release.get("assets", [])
    names = set()
    entries = []
    for a in assets:
        name, size, identity = a.get("name"), a.get("size"), a.get("id")
        if not isinstance(name, str) or not ASSET.fullmatch(name) or name in names:
            raise ValueError("unsafe or duplicate release asset name")
        if not isinstance(size, int) or size < 0 or size > MAX_ASSET or not isinstance(identity, int):
            raise ValueError("invalid release asset metadata")
        names.add(name)
        entries.append([name, size, identity])
    raw = json.dumps(sorted(entries), separators=(",", ":")).encode()
    return hashlib.sha256(raw).hexdigest()


def marker(release, ref):
    return f"<!-- aego-gitea-mirror:v1 tag={release['tag_name']} ref={ref} assets={inventory(release)} -->"


def refs(tag):
    escaped = quote(tag, safe="")
    src = source("git/refs/tags/" + escaped)
    if not isinstance(src, list) or len(src) != 1:
        raise ValueError("missing or ambiguous Gitea tag " + tag)
    dst = github("git/ref/tags/" + escaped)
    if src[0].get("ref") != "refs/tags/" + tag or dst.get("ref") != "refs/tags/" + tag:
        raise ValueError("unexpected tag reference: " + tag)
    a, b = src[0].get("object", {}), dst.get("object", {})
    if a.get("sha") != b.get("sha") or a.get("type") != b.get("type") or not re.fullmatch(r"[0-9a-f]{40}", a.get("sha", "")):
        raise ValueError("mirrored tag differs: " + tag)
    return a["sha"]


def download(asset, dest):
    url = asset.get("browser_download_url", "")
    parsed = urlsplit(url)
    if (parsed.scheme != "https" or parsed.netloc != GITEA_HOST or parsed.username or parsed.password
            or parsed.query or parsed.fragment or not parsed.path.startswith("/sanspie/aego/releases/download/")):
        raise ValueError("unsafe Gitea asset URL")
    total = 0
    digest = hashlib.sha256()
    with urlopen(Request(url, headers={"Accept": "application/octet-stream"}), timeout=120) as response, open(dest, "xb") as f:
        while True:
            part = response.read(1024 * 1024)
            if not part:
                break
            total += len(part)
            if total > asset["size"] or total > MAX_ASSET:
                raise ValueError("asset is larger than advertised")
            digest.update(part)
            f.write(part)
    if total != asset["size"]:
        raise ValueError("asset is shorter than advertised")
    return digest.hexdigest()


def checksums(path):
    sums = {}
    for line in path.read_text(encoding="utf-8").splitlines():
        match = SUM.fullmatch(line)
        if not match or not ASSET.fullmatch(match[2]) or match[2] in sums:
            raise ValueError("invalid SHA256SUMS.txt")
        sums[match[2]] = match[1].lower()
    return sums


def existing_hash(tag, asset, temp):
    digest = asset.get("digest")
    if isinstance(digest, str) and re.fullmatch(r"sha256:[0-9a-f]{64}", digest):
        return digest[7:]
    with tempfile.TemporaryDirectory(prefix="github-asset-", dir=temp) as directory:
        dest = Path(directory)
        gh("release", "download", tag, "--repo", REPOSITORY, "--pattern", asset["name"], "--dir", str(dest))
        digest = hashlib.sha256()
        with (dest / asset["name"]).open("rb") as f:
            for part in iter(lambda: f.read(1024 * 1024), b""):
                digest.update(part)
        return digest.hexdigest()


def github_release(tag):
    # The /releases/tags endpoint only returns published releases; list also
    # includes our draft when authenticated with repository write access.
    matches = [r for r in pages(github, "releases") if r.get("tag_name") == tag]
    if len(matches) != 1:
        raise ValueError("missing or ambiguous GitHub release: " + tag)
    return matches[0]


def sync_release(release, existing, latest):
    tag = release["tag_name"]
    if not TAG.fullmatch(tag):
        raise ValueError("unsafe release tag")
    ref = refs(tag)
    expected_marker = marker(release, ref)
    prerelease = is_prerelease(release)
    if existing and expected_marker not in (existing.get("body") or ""):
        # A user-owned release or a changed nightly must never be replaced.
        if "<!-- aego-gitea-mirror:v1 " in (existing.get("body") or ""):
            raise ValueError("published source snapshot changed; refusing to overwrite " + tag)
        if existing.get("draft"):
            print("Leaving user-owned draft unchanged:", tag)
            return False
        if bool(existing.get("prerelease")) != prerelease:
            gh("release", "edit", tag, "--repo", REPOSITORY, "--prerelease=" + str(prerelease).lower())
        print("Leaving user-owned release and its assets unchanged:", tag)
        return True

    with tempfile.TemporaryDirectory(prefix="aego-release-mirror-") as directory:
        temp = Path(directory)
        source_files = temp / "source"
        source_files.mkdir()
        assets = release.get("assets", [])
        current = {a["name"]: a for a in (existing or {}).get("assets", [])}
        if len(current) != len((existing or {}).get("assets", [])):
            raise ValueError("duplicate GitHub assets")
        sums_asset = next((a for a in assets if a["name"] == "SHA256SUMS.txt"), None)
        sums = {}
        downloaded = {}
        if sums_asset:
            downloaded[sums_asset["name"]] = download(sums_asset, source_files / sums_asset["name"])
            sums = checksums(source_files / sums_asset["name"])
        if tag == "stable" and not {"release.json", "release.json.sig"}.issubset({a["name"] for a in assets}):
            raise ValueError("stable channel manifest or signature is missing")
        if tag != "stable" and (not sums or not set(sums).issubset({a["name"] for a in assets})):
            raise ValueError("release assets or checksums are incomplete: " + tag)
        to_upload = []
        expected_hashes = {}
        for asset in assets:
            name = asset["name"]
            expected = sums.get(name)
            if expected is None and name not in ("SHA256SUMS.txt", "release.json", "release.json.sig"):
                raise ValueError("asset is absent from SHA256SUMS.txt: " + name)
            old = current.get(name)
            if old and old.get("size") != asset["size"]:
                raise ValueError("GitHub asset has a different size: " + name)
            if name not in downloaded and (not old or expected is None):
                downloaded[name] = download(asset, source_files / name)
            actual = downloaded.get(name, expected)
            if expected and actual != expected:
                raise ValueError("Gitea asset checksum mismatch: " + name)
            expected_hashes[name] = actual
            if old:
                if existing_hash(tag, old, temp) != actual:
                    raise ValueError("GitHub asset differs; refusing to replace: " + name)
            else:
                to_upload.append(str(source_files / name))
        if not existing:
            notes = temp / "notes.md"
            notes.write_text((release.get("body") or "") + "\n\nMirrored from Gitea.\n\n" + expected_marker + "\n", encoding="utf-8")
            # The tag already exists. Omitting --target avoids GitHub's
            # workflow-scope restriction for historical target commits.
            args = ["release", "create", tag, "--repo", REPOSITORY, "--verify-tag", "--draft", "--latest=false", "--title", release.get("name") or tag, "--notes-file", str(notes)]
            if prerelease:
                args.append("--prerelease")
            gh(*args)
        if to_upload:
            gh("release", "upload", tag, *to_upload, "--repo", REPOSITORY)
        uploaded = github_release(tag)
        if expected_marker not in (uploaded.get("body") or ""):
            raise ValueError("GitHub release changed during upload: " + tag)
        if bool(uploaded.get("draft")) != (not existing or bool(existing.get("draft"))):
            raise ValueError("GitHub draft state changed during upload: " + tag)
        uploaded_assets = {a["name"]: a for a in uploaded.get("assets", [])}
        if len(uploaded_assets) != len(uploaded.get("assets", [])):
            raise ValueError("duplicate uploaded assets: " + tag)
        for asset in assets:
            name = asset["name"]
            remote = uploaded_assets.get(name)
            if (not remote or remote.get("size") != asset["size"] or remote.get("state") != "uploaded"
                    or existing_hash(tag, remote, temp) != expected_hashes[name]):
                raise ValueError("GitHub asset verification failed: " + name)
        current_source = source("releases/tags/" + quote(tag, safe=""))
        if current_source.get("draft") or inventory(current_source) != inventory(release) or refs(tag) != ref:
            raise ValueError("source release changed during upload: " + tag)
        if not existing or existing.get("draft"):
            if not ci_ready():
                raise ValueError("CI or master refs changed during release upload")
            gh("release", "edit", tag, "--repo", REPOSITORY, "--verify-tag", "--draft=false", "--prerelease=" + str(prerelease).lower(), "--latest=" + str(tag == latest).lower())
        elif bool(existing.get("prerelease")) != prerelease:
            gh("release", "edit", tag, "--repo", REPOSITORY, "--prerelease=" + str(prerelease).lower())
    print("Mirrored release:", tag)
    return True


def main(args):
    if args not in ([], ["--apply"]):
        raise ValueError("expected no arguments or --apply")
    releases = [r for r in pages(source, "releases") if not r.get("draft")]
    for release in releases:
        if not TAG.fullmatch(release.get("tag_name", "")):
            raise ValueError("unsupported Gitea release tag")
        inventory(release)
    versions = [(stable_version(r), r["tag_name"]) for r in releases if stable_version(r) is not None]
    latest = max(versions)[1] if versions else None
    print("Gitea releases:", ", ".join(r["tag_name"] for r in releases), "latest:", latest)
    if not args:
        print("Plan only; no GitHub releases were changed")
        return 0
    if (os.getenv("GITHUB_SERVER_URL") != "https://github.com" or os.getenv("GITHUB_REPOSITORY") != REPOSITORY
            or os.getenv("GITHUB_REF") != "refs/heads/master" or os.getenv("GITHUB_EVENT_NAME") not in ("workflow_run", "schedule", "workflow_dispatch")
            or not os.getenv("GH_TOKEN")):
        raise ValueError("release writes require the trusted GitHub master workflow")
    if not ci_ready():
        return 0
    existing = {r["tag_name"]: r for r in pages(github, "releases")}
    latest_ready = False
    failures = []
    for release in sorted(releases, key=lambda r: r["tag_name"] == latest):
        try:
            ready = sync_release(release, existing.get(release["tag_name"]), latest)
        except (OSError, ValueError, RuntimeError, KeyError) as exc:
            failures.append(release["tag_name"])
            print(f"::error::Release {release['tag_name']} not mirrored: {type(exc).__name__}: {exc}", file=sys.stderr)
            continue
        if release["tag_name"] == latest:
            latest_ready = ready
    if latest and latest_ready and ci_ready():
        gh("release", "edit", latest, "--repo", REPOSITORY, "--verify-tag", "--latest=true")
    return 1 if failures else 0


if __name__ == "__main__":
    try:
        sys.exit(main(sys.argv[1:]))
    except (OSError, ValueError, RuntimeError, KeyError, json.JSONDecodeError) as exc:
        print("::error::Release mirror failed:", type(exc).__name__, str(exc), file=sys.stderr)
        sys.exit(1)
