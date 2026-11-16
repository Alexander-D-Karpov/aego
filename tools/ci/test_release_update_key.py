"""Offline tests for the trusted public update key and packaging checks."""

import base64
import hashlib
import io
import json
import os
from pathlib import Path
import tarfile
import tempfile
import unittest
from unittest import mock

import release_update_key as key


RAW = b"a" * 32
PUBLIC = base64.b64encode(RAW).decode("ascii")
FINGERPRINT = hashlib.sha256(RAW).hexdigest()


def tar_blob(binary, extra=None):
    output = io.BytesIO()
    with tarfile.open(fileobj=output, mode="w:gz") as archive:
        entry = tarfile.TarInfo(key.BINARY_NAME)
        entry.size = len(binary)
        archive.addfile(entry, io.BytesIO(binary))
        if extra:
            extra(archive)
    output.seek(0)
    return output


class KeyTest(unittest.TestCase):
    def setUp(self):
        patch = mock.patch.object(key, "TRUSTED_KEY_SHA256", FINGERPRINT)
        patch.start()
        self.addCleanup(patch.stop)

    def test_validates_encoding_and_trusted_fingerprint(self):
        self.assertEqual(key.validate_key(PUBLIC), PUBLIC)
        for value in ("", PUBLIC + "\n", PUBLIC[:-1], "!" * 43 + "=", base64.b64encode(b"b" * 32).decode()):
            with self.subTest(value=value), self.assertRaises(ValueError):
                key.validate_key(value)

    def test_finds_a_trusted_key_across_stream_chunk_boundaries(self):
        with mock.patch.object(key, "CHUNK", 16):
            self.assertEqual(key.find_key(io.BytesIO(b"~" * 15 + PUBLIC.encode() + b"~")), PUBLIC)
        with self.assertRaises(ValueError):
            key.find_key(io.BytesIO(b"no trusted key"))

    def test_valid_archive_and_rejects_missing_or_duplicate_binary(self):
        self.assertEqual(key.key_from_archive(tar_blob(b"binary:" + PUBLIC.encode())), PUBLIC)
        with self.assertRaises(ValueError):
            key.key_from_archive(tar_blob(b"no key"))
        def duplicate(archive):
            entry = tarfile.TarInfo(key.BINARY_NAME)
            entry.size = len(PUBLIC)
            archive.addfile(entry, io.BytesIO(PUBLIC.encode()))
        with self.assertRaises(ValueError):
            key.key_from_archive(tar_blob(PUBLIC.encode(), duplicate))

    def test_archive_refuses_traversal(self):
        def unsafe(archive):
            entry = tarfile.TarInfo("../../outside")
            entry.size = 1
            archive.addfile(entry, io.BytesIO(b"x"))
        with self.assertRaises(ValueError):
            key.key_from_archive(tar_blob(PUBLIC.encode(), unsafe))

    def test_binary_verification_rejects_missing_key_and_symlink(self):
        with tempfile.TemporaryDirectory(prefix="aego-release-key-test-") as temporary:
            root = Path(temporary)
            binary = root / "aego"
            binary.write_bytes(b"~" * 15 + PUBLIC.encode() + b"~")
            with mock.patch.object(key, "CHUNK", 16):
                key.verify_binary(binary, PUBLIC)
            link = root / "link"
            link.symlink_to(binary)
            with self.assertRaises(ValueError):
                key.verify_binary(link, PUBLIC)
            binary.write_bytes(b"wrong")
            with self.assertRaises(ValueError):
                key.verify_binary(binary, PUBLIC)

    def test_download_pins_url_size_and_hash(self):
        payload = b"fixture data"
        expected = hashlib.sha256(payload).hexdigest()
        class Response(io.BytesIO):
            status = 200
            def geturl(self):
                return key.BASE + "fixture"
        with mock.patch.object(key, "urlopen", return_value=Response(payload)):
            out = io.BytesIO()
            key.download("fixture", expected, 100, out)
            self.assertEqual(out.read(), payload)
        with mock.patch.object(key, "urlopen", return_value=Response(payload)):
            with self.assertRaises(ValueError):
                key.download("fixture", "0" * 64, 100, io.BytesIO())
        with mock.patch.object(key, "urlopen", return_value=Response(payload)):
            with self.assertRaises(ValueError):
                key.download("fixture", expected, 2, io.BytesIO())

    @unittest.skipIf(os.name == "nt", "the synthetic fixture is a Unix executable")
    def test_smoke_serves_only_isolated_loopback_documents(self):
        with tempfile.TemporaryDirectory(prefix="aego-update-smoke-test-") as temporary:
            script = Path(temporary) / "aego"
            script.write_text('''#!/usr/bin/env python3
import os,urllib.request
base=os.environ["AEGO_UPDATE_URL"]+"/stable/"
manifest=urllib.request.urlopen(base+"release.json").read()
assert urllib.request.urlopen(base+"release.json.sig").read()==b"signature"
if manifest != b"manifest":
    import sys
    print("the release is not signed, or not by the key this build trusts", file=sys.stderr)
    raise SystemExit(1)
''')
            script.chmod(0o755)
            key.smoke(script, b"manifest", b"signature")
            script.write_text("#!/usr/bin/env python3\nraise SystemExit(7)\n")
            with self.assertRaises(ValueError):
                key.smoke(script, b"manifest", b"signature")
            script.write_text('''#!/usr/bin/env python3
import os,urllib.request
base=os.environ["AEGO_UPDATE_URL"]+"/stable/"
urllib.request.urlopen(base+"release.json").read()
urllib.request.urlopen(base+"release.json.sig").read()
''')
            with self.assertRaises(ValueError):
                key.smoke(script, b"manifest", b"signature")

    def test_stable_documents_require_expected_manifest_identity(self):
        def fake_download(name, _digest, _limit, dest):
            dest.write(json.dumps({"version": "0.12.1", "channel": "stable"}).encode()
                       if name == "release.json" else b"signature")
            dest.seek(0)
        with mock.patch.object(key, "download", side_effect=fake_download):
            manifest, signature = key.stable_documents()
            self.assertEqual(json.loads(manifest)["version"], "0.12.1")
            self.assertEqual(signature, b"signature")


if __name__ == "__main__":
    unittest.main(verbosity=2)
