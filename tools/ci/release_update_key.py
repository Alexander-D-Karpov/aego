#!/usr/bin/env python3
"""Retrieve the established public update key and verify release binaries."""

import argparse
import base64
import hashlib
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
import json
import os
from pathlib import Path, PurePosixPath
import re
import stat
import subprocess
import tarfile
import tempfile
import threading
from urllib.request import Request, urlopen


BASE = "https://git.akarpov.ru/sanspie/aego/releases/download/v0.12.1/"
ARCHIVE_NAME = "aego-0.12.1-linux-amd64.tar.gz"
ARCHIVE_SHA256 = "6506aa1336276fc6dd16607fa6557155bbdd47a74ab0828796b129258121623a"
MANIFEST_SHA256 = "f061e00684f7df97d47dd563fbad120a452a8831fb24fae3f71d042b9a207f75"
SIGNATURE_SHA256 = "9290362d9e79725400373f616887c6eb54038c359a7d6f06dd4637df64b39202"
TRUSTED_KEY_SHA256 = "06096a4da6dac0b1ee5a8aecc665700bb6a98066f2dffdb30e4d3096877da59e"
BINARY_NAME = "aego-0.12.1-linux-amd64/bin/aego"
MAX_ARCHIVE = 100 * 1024**2
MAX_BINARY = 256 * 1024**2
CHUNK = 1024 * 1024


def require(value, message):
    if not value:
        raise ValueError(message)


def validate_key(value):
    require(isinstance(value, str) and re.fullmatch(r"[A-Za-z0-9+/]{43}=", value), "invalid update key encoding")
    try:
        raw = base64.b64decode(value, validate=True)
    except ValueError as exc:
        raise ValueError("invalid update key encoding") from exc
    require(len(raw) == 32 and base64.b64encode(raw).decode("ascii") == value and
            hashlib.sha256(raw).hexdigest() == TRUSTED_KEY_SHA256, "update key does not match the trusted key")
    return value


def download(name, expected, limit, dest):
    url = BASE + name
    digest = hashlib.sha256()
    size = 0
    with urlopen(Request(url, headers={"Accept": "application/octet-stream"}), timeout=60) as response:
        require(response.status == 200 and response.geturl() == url, "unexpected release asset response")
        while chunk := response.read(CHUNK):
            size += len(chunk)
            require(size <= limit, "release asset exceeds size limit")
            digest.update(chunk)
            dest.write(chunk)
    require(size > 0 and digest.hexdigest() == expected, "release asset checksum differs")
    dest.seek(0)


def find_key(stream):
    pattern = re.compile(rb"[A-Za-z0-9+/]{43}=")
    found = set()
    tail = b""
    size = 0
    while chunk := stream.read(CHUNK):
        size += len(chunk)
        require(size <= MAX_BINARY, "stable executable exceeds size limit")
        data = tail + chunk
        for match in pattern.finditer(data):
            candidate = match.group().decode("ascii")
            try:
                found.add(validate_key(candidate))
            except ValueError:
                pass
        tail = data[-43:]
    require(size > 0 and len(found) == 1, "trusted key is absent or ambiguous in stable executable")
    return found.pop()


def key_from_archive(stream):
    names = set()
    result = None
    total = 0
    with tarfile.open(fileobj=stream, mode="r|gz") as archive:
        for entry in archive:
            require(len(names) < 1000 and entry.name not in names, "duplicate or excessive stable archive entries")
            names.add(entry.name)
            path = PurePosixPath(entry.name)
            require(not path.is_absolute() and all(part not in ("", ".", "..") for part in path.parts),
                    "unsafe stable archive path")
            total += entry.size
            require(0 <= entry.size <= MAX_BINARY and total <= 1024**3, "stable archive exceeds size limit")
            if entry.name == BINARY_NAME:
                require(entry.isfile() and 0 < entry.size <= MAX_BINARY and result is None,
                        "stable executable is missing or unsafe")
                with archive.extractfile(entry) as binary:
                    result = find_key(binary)
    require(result is not None, "stable executable is missing")
    return result


def public_key():
    with tempfile.TemporaryFile() as archive:
        download(ARCHIVE_NAME, ARCHIVE_SHA256, MAX_ARCHIVE, archive)
        return key_from_archive(archive)


def verify_binary(path, key):
    key = validate_key(key).encode("ascii")
    path = Path(path)
    before = path.lstat()
    require(stat.S_ISREG(before.st_mode) and 0 < before.st_size <= MAX_BINARY, "binary is missing or unsafe")
    identity = lambda value: (value.st_dev, value.st_ino, value.st_size, value.st_mtime_ns, value.st_ctime_ns)
    shared = lambda value: (value.st_dev, value.st_ino, value.st_size, value.st_mtime_ns)
    found = False
    tail = b""
    with path.open("rb") as stream:
        opened = os.fstat(stream.fileno())
        require(stat.S_ISREG(opened.st_mode) and shared(before) == shared(opened), "binary changed before reading")
        while chunk := stream.read(CHUNK):
            data = tail + chunk
            found = found or key in data
            tail = data[-(len(key) - 1):]
        current = os.fstat(stream.fileno())
    after = path.lstat()
    # Windows may report creation time for lstat ctime and change time for fstat ctime.
    require(found and identity(before) == identity(after) and identity(opened) == identity(current) and
            shared(current) == shared(after), "binary lacks the trusted key or changed")


def stable_documents():
    values = []
    for name, expected, limit in (("release.json", MANIFEST_SHA256, 1024**2),
                                  ("release.json.sig", SIGNATURE_SHA256, 4096)):
        with tempfile.TemporaryFile() as stream:
            download(name, expected, limit, stream)
            values.append(stream.read())
    manifest = json.loads(values[0])
    require(manifest.get("version") == "0.12.1" and manifest.get("channel") == "stable",
            "stable manifest identity differs")
    return values


def smoke(binary, manifest, signature):
    documents = {"/stable/release.json": manifest, "/stable/release.json.sig": signature}
    seen = set()

    class Handler(BaseHTTPRequestHandler):
        def do_GET(self):
            data = documents.get(self.path)
            if data is None:
                self.send_error(404)
                return
            seen.add(self.path)
            self.send_response(200)
            self.send_header("Content-Length", str(len(data)))
            self.end_headers()
            self.wfile.write(data)

        def log_message(self, *_args):
            pass

    server = ThreadingHTTPServer(("127.0.0.1", 0), Handler)
    thread = threading.Thread(target=server.serve_forever, kwargs={"poll_interval": 0.1}, daemon=True)
    thread.start()
    try:
        with tempfile.TemporaryDirectory(prefix="aego-update-smoke-") as temporary:
            root = Path(temporary)
            for tampered in (False, True):
                home = root / ("tampered" if tampered else "signed")
                config, cache = home / "config", home / "cache"
                config.mkdir(parents=True)
                cache.mkdir()
                env = dict(os.environ, HOME=str(home), USERPROFILE=str(home), XDG_CONFIG_HOME=str(config),
                           XDG_CACHE_HOME=str(cache), APPDATA=str(config), LOCALAPPDATA=str(cache),
                           AEGO_UPDATE_URL=f"http://127.0.0.1:{server.server_port}",
                           SDL_AUDIO_DRIVER="dummy", SDL_AUDIODRIVER="dummy", PULSE_SERVER="unix:/dev/null",
                           HTTP_PROXY="", HTTPS_PROXY="", ALL_PROXY="", NO_PROXY="127.0.0.1,localhost",
                           http_proxy="", https_proxy="", all_proxy="", no_proxy="127.0.0.1,localhost")
                documents["/stable/release.json"] = manifest + (b" " if tampered else b"")
                seen.clear()
                proc = subprocess.run([str(Path(binary).resolve()), "update", "check", "--channel", "stable"],
                                      stdout=subprocess.PIPE, stderr=subprocess.PIPE, timeout=40, env=env)
                require(seen == set(documents), "update check did not fetch both fixture documents")
                if tampered:
                    require(proc.returncode != 0 and b"the release is not signed, or not by the key this build trusts" in proc.stderr,
                            "tampered signed manifest was not rejected by signature verification")
                else:
                    require(proc.returncode == 0, "signed update check failed")
    finally:
        server.shutdown()
        thread.join(timeout=5)
        server.server_close()


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    mode = parser.add_mutually_exclusive_group(required=True)
    mode.add_argument("--value", action="store_true", help="print the verified public key for the linker")
    mode.add_argument("--fingerprint", action="store_true", help="verify the supplied key and print its fingerprint")
    mode.add_argument("--check-binary", metavar="PATH", help="verify the trusted public key is present")
    mode.add_argument("--smoke", metavar="PATH", help="verify a CLI against a signed local update manifest")
    args = parser.parse_args()
    if args.value:
        print(public_key())
        return 0
    key = validate_key(os.environ.pop("AEGO_CI_UPDATE_PUBLIC_KEY", ""))
    if args.fingerprint:
        print(TRUSTED_KEY_SHA256)
    elif args.check_binary:
        verify_binary(args.check_binary, key)
        print("Trusted public update key is present in the binary.")
    else:
        verify_binary(args.smoke, key)
        manifest, signature = stable_documents()
        smoke(args.smoke, manifest, signature)
        print("Signed update check passed and the tampered manifest was rejected.")
    return 0


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except (ValueError, OSError, subprocess.SubprocessError, tarfile.TarError, json.JSONDecodeError) as exc:
        raise SystemExit("Release update key check failed: " + str(exc))
