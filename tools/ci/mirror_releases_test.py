import contextlib
import hashlib
import io
import os
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch

import mirror_releases as mirror


SHA = "a" * 40


def release(tag="v1.2.3", prerelease=False):
    payload = b"release binary\n"
    checksum = hashlib.sha256(payload).hexdigest()
    sums = f"{checksum}  aego.zip\n".encode()
    return {
        "tag_name": tag,
        "name": "Aego",
        "body": "Source release notes",
        "draft": False,
        "prerelease": prerelease,
        "assets": [
            {"id": 1, "name": "aego.zip", "size": len(payload)},
            {"id": 2, "name": "SHA256SUMS.txt", "size": len(sums)},
        ],
    }, {"aego.zip": payload, "SHA256SUMS.txt": sums}


def silence():
    return contextlib.redirect_stdout(io.StringIO())


def uploaded_release(item, data):
    assets = [{"name": a["name"], "size": a["size"], "state": "uploaded",
               "digest": "sha256:" + hashlib.sha256(data[a["name"]]).hexdigest()} for a in item["assets"]]
    return {"body": mirror.marker(item, SHA), "draft": True, "assets": assets}


class ReleaseMirrorTests(unittest.TestCase):
    def test_version_and_prerelease_classification(self):
        self.assertEqual(mirror.stable_version(release("v12.3.4")[0]), (12, 3, 4))
        for tag in ("nightly", "stable", "v1.2.3-beta.1"):
            self.assertIsNone(mirror.stable_version(release(tag)[0]))
            self.assertTrue(mirror.is_prerelease(release(tag)[0]))
        self.assertTrue(mirror.is_prerelease(release("v1.2.3", True)[0]))
        self.assertFalse(mirror.is_prerelease(release("v1.2.3")[0]))

    def test_inventory_is_order_independent_and_rejects_unsafe_names(self):
        item, _ = release()
        other = {**item, "assets": list(reversed(item["assets"]))}
        self.assertEqual(mirror.inventory(item), mirror.inventory(other))
        other["assets"][0] = {**other["assets"][0], "name": "../escape"}
        with self.assertRaises(ValueError):
            mirror.inventory(other)

    def test_source_and_github_tags_must_match(self):
        src = [{"ref": "refs/tags/v1.2.3", "object": {"type": "commit", "sha": SHA}}]
        dst = {"ref": "refs/tags/v1.2.3", "object": {"type": "commit", "sha": "b" * 40}}
        with patch.object(mirror, "source", return_value=src), patch.object(mirror, "github", return_value=dst):
            with self.assertRaises(ValueError):
                mirror.refs("v1.2.3")

    def test_checksums_reject_duplicates_and_paths(self):
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "SHA256SUMS.txt"
            path.write_text(("a" * 64) + "  ../bad\n", encoding="utf-8")
            with self.assertRaises(ValueError):
                mirror.checksums(path)
            path.write_text((("a" * 64) + "  file.zip\n") * 2, encoding="utf-8")
            with self.assertRaises(ValueError):
                mirror.checksums(path)

    def test_new_release_is_draft_until_assets_and_ci_are_verified(self):
        item, data = release()

        def download(asset, path):
            payload = data[asset["name"]]
            path.write_bytes(payload)
            return hashlib.sha256(payload).hexdigest()

        with patch.object(mirror, "refs", return_value=SHA), patch.object(mirror, "download", side_effect=download), \
                patch.object(mirror, "source", return_value=item), patch.object(mirror, "ci_ready", return_value=True), \
                patch.object(mirror, "github_release", return_value=uploaded_release(item, data)), patch.object(mirror, "gh") as gh, silence():
            self.assertTrue(mirror.sync_release(item, None, "v1.2.3"))
        calls = gh.call_args_list
        self.assertEqual([c.args[1] for c in calls], ["create", "upload", "edit"])
        self.assertIn("--draft", calls[0].args)
        self.assertIn("--verify-tag", calls[0].args)
        self.assertNotIn("--target", calls[0].args)
        self.assertNotIn("--clobber", calls[1].args)
        self.assertIn("--draft=false", calls[2].args)
        self.assertIn("--latest=true", calls[2].args)
        self.assertIn("--prerelease=false", calls[2].args)

    def test_nightly_is_prerelease_and_not_latest(self):
        item, data = release("nightly")

        def download(asset, path):
            payload = data[asset["name"]]
            path.write_bytes(payload)
            return hashlib.sha256(payload).hexdigest()

        with patch.object(mirror, "refs", return_value=SHA), patch.object(mirror, "download", side_effect=download), \
                patch.object(mirror, "source", return_value=item), patch.object(mirror, "ci_ready", return_value=True), \
                patch.object(mirror, "github_release", return_value=uploaded_release(item, data)), patch.object(mirror, "gh") as gh, silence():
            self.assertTrue(mirror.sync_release(item, None, "v1.2.3"))
        self.assertIn("--prerelease", gh.call_args_list[0].args)
        self.assertIn("--prerelease=true", gh.call_args_list[-1].args)
        self.assertIn("--latest=false", gh.call_args_list[-1].args)

    def test_bad_checksum_never_creates_a_release(self):
        item, data = release()
        data["aego.zip"] = b"tampered data\n"

        def download(asset, path):
            payload = data[asset["name"]]
            path.write_bytes(payload)
            return hashlib.sha256(payload).hexdigest()

        with patch.object(mirror, "refs", return_value=SHA), patch.object(mirror, "download", side_effect=download), \
                patch.object(mirror, "gh") as gh:
            with self.assertRaises(ValueError):
                mirror.sync_release(item, None, "v1.2.3")
            gh.assert_not_called()

    def test_uploaded_checksum_mismatch_leaves_release_as_draft(self):
        item, data = release()
        remote = uploaded_release(item, data)
        remote["assets"][0]["digest"] = "sha256:" + ("0" * 64)

        def download(asset, path):
            payload = data[asset["name"]]
            path.write_bytes(payload)
            return hashlib.sha256(payload).hexdigest()

        with patch.object(mirror, "refs", return_value=SHA), patch.object(mirror, "download", side_effect=download), \
                patch.object(mirror, "github_release", return_value=remote), patch.object(mirror, "gh") as gh:
            with self.assertRaises(ValueError):
                mirror.sync_release(item, None, "v1.2.3")
        self.assertEqual([c.args[1] for c in gh.call_args_list], ["create", "upload"])

    def test_failed_ci_after_upload_never_publishes_draft(self):
        item, data = release()

        def download(asset, path):
            payload = data[asset["name"]]
            path.write_bytes(payload)
            return hashlib.sha256(payload).hexdigest()

        with patch.object(mirror, "refs", return_value=SHA), patch.object(mirror, "download", side_effect=download), \
                patch.object(mirror, "source", return_value=item), patch.object(mirror, "ci_ready", return_value=False), \
                patch.object(mirror, "github_release", return_value=uploaded_release(item, data)), patch.object(mirror, "gh") as gh:
            with self.assertRaises(ValueError):
                mirror.sync_release(item, None, "v1.2.3")
        self.assertEqual([c.args[1] for c in gh.call_args_list], ["create", "upload"])

    def test_download_rejects_untrusted_url(self):
        asset = {"size": 0, "browser_download_url": "https://other.example/aego.zip"}
        with tempfile.TemporaryDirectory() as directory:
            with self.assertRaises(ValueError):
                mirror.download(asset, Path(directory) / "file")

    def test_draft_lookup_uses_release_listing(self):
        draft = {"tag_name": "v1.2.3", "draft": True}
        with patch.object(mirror, "pages", return_value=[draft]) as pages:
            self.assertEqual(mirror.github_release("v1.2.3"), draft)
            pages.assert_called_once_with(mirror.github, "releases")

    def test_incomplete_source_is_not_published(self):
        item, data = release()
        item["assets"] = item["assets"][1:]

        def download(asset, path):
            payload = data[asset["name"]]
            path.write_bytes(payload)
            return hashlib.sha256(payload).hexdigest()

        with patch.object(mirror, "refs", return_value=SHA), patch.object(mirror, "download", side_effect=download), \
                patch.object(mirror, "gh") as gh:
            with self.assertRaises(ValueError):
                mirror.sync_release(item, None, "v1.2.3")
            gh.assert_not_called()

    def test_changed_nightly_is_never_overwritten(self):
        item, _ = release("nightly")
        old = {"body": "<!-- aego-gitea-mirror:v1 tag=nightly ref=old assets=old -->", "draft": False}
        with patch.object(mirror, "refs", return_value=SHA), patch.object(mirror, "gh") as gh:
            with self.assertRaises(ValueError):
                mirror.sync_release(item, old, None)
            gh.assert_not_called()

    def test_user_draft_is_never_modified(self):
        item, _ = release()
        with patch.object(mirror, "refs", return_value=SHA), patch.object(mirror, "gh") as gh, silence():
            self.assertFalse(mirror.sync_release(item, {"body": "mine", "draft": True}, "v1.2.3"))
            gh.assert_not_called()

    def test_user_release_only_gets_requested_prerelease_flag(self):
        item, _ = release("nightly")
        with patch.object(mirror, "refs", return_value=SHA), patch.object(mirror, "gh") as gh, silence():
            self.assertTrue(mirror.sync_release(item, {"body": "mine", "draft": False, "prerelease": False}, None))
            gh.assert_called_once_with("release", "edit", "nightly", "--repo", mirror.REPOSITORY, "--prerelease=true")

    def test_current_ci_must_pass_on_both_servers(self):
        run = {"id": 42, "head_sha": SHA, "status": "completed", "conclusion": "success", "event": "push", "run_attempt": 1}
        jobs = [{"name": n, "run_attempt": 1, "status": "completed", "conclusion": "success"} for n in mirror.NATIVE_JOBS]
        statuses = [{"context": "ci / checks (push)", "status": "success"}, {"context": "ci / build (linux) (push)", "status": "success"}]

        def source(path):
            return {"commit": {"id": SHA}} if path == "branches/master" else {"statuses": statuses}

        with patch.object(mirror, "source", side_effect=source), patch.object(mirror, "github", return_value={"commit": {"sha": SHA}}), \
                patch.object(mirror, "pages", side_effect=[[run], jobs]), silence():
            self.assertTrue(mirror.ci_ready())
        jobs[0]["conclusion"] = "failure"
        with patch.object(mirror, "source", side_effect=source), patch.object(mirror, "github", return_value={"commit": {"sha": SHA}}), \
                patch.object(mirror, "pages", side_effect=[[run], jobs]), silence():
            self.assertFalse(mirror.ci_ready())

    def test_release_writes_require_trusted_environment(self):
        item, _ = release()
        with patch.dict(os.environ, {}, clear=True), patch.object(mirror, "pages", return_value=[item]), \
                patch.object(mirror, "gh") as gh, silence():
            with self.assertRaises(ValueError):
                mirror.main(["--apply"])
            gh.assert_not_called()

    def test_changed_nightly_does_not_block_a_separate_stable_release(self):
        nightly, _ = release("nightly")
        stable, _ = release("v1.2.3")
        env = {"GITHUB_SERVER_URL": "https://github.com", "GITHUB_REPOSITORY": mirror.REPOSITORY,
               "GITHUB_REF": "refs/heads/master", "GITHUB_EVENT_NAME": "workflow_dispatch", "GH_TOKEN": "dummy"}
        with patch.dict(os.environ, env, clear=True), patch.object(mirror, "pages", side_effect=[[nightly, stable], []]), \
                patch.object(mirror, "ci_ready", return_value=True), \
                patch.object(mirror, "sync_release", side_effect=[ValueError("changed snapshot"), True]), \
                patch.object(mirror, "gh") as gh, silence(), contextlib.redirect_stderr(io.StringIO()):
            self.assertEqual(mirror.main(["--apply"]), 1)
        gh.assert_called_once_with("release", "edit", "v1.2.3", "--repo", mirror.REPOSITORY, "--verify-tag", "--latest=true")


if __name__ == "__main__":
    unittest.main()
