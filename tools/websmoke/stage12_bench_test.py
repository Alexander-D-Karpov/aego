import hashlib
from pathlib import Path
import tarfile
import tempfile
import unittest

import stage12_bench as bench


class Stage12BenchmarkFixtureTest(unittest.TestCase):
    def test_reviewed_fixture_replays_original_tracked_project(self):
        with tempfile.TemporaryDirectory(prefix='aego-stage12-fixture-test-') as temp:
            destination = Path(temp)
            bench.extract_fixture(bench.FIXTURE, destination)
            files = [p for p in destination.rglob('*') if p.is_file()]
            self.assertEqual(len(files), 126)
            self.assertEqual(bench.digest(destination / 'replay.aeinput'), bench.REPLAY_SHA256)
            self.assertTrue((destination / 'project/go.mod').is_file())
            self.assertFalse((destination / 'project/.aego').exists())
            self.assertEqual(bench.digest(bench.BASELINE), bench.BASELINE_SHA256)

    def test_tampered_fixture_and_nonempty_destination_are_not_overwritten(self):
        with tempfile.TemporaryDirectory(prefix='aego-stage12-fixture-test-') as temp:
            root = Path(temp)
            archive = root / 'bad.tar.gz'
            archive.write_bytes(bench.FIXTURE.read_bytes() + b'tampered')
            dest = root / 'dest'
            dest.mkdir()
            with self.assertRaisesRegex(ValueError, 'archive'):
                bench.extract_fixture(archive, dest)
            self.assertEqual(list(dest.iterdir()), [])
            marker = dest / 'user-data'
            marker.write_text('do not overwrite')
            with self.assertRaisesRegex(ValueError, 'empty'):
                bench.extract_fixture(bench.FIXTURE, dest)
            self.assertEqual(marker.read_text(), 'do not overwrite')

    def test_unsafe_archive_members_are_rejected(self):
        for name in ('../outside', '/absolute', 'project/../outside', r'project/..\outside', 'project//double', 'other/file'):
            with self.subTest(name=name):
                member = tarfile.TarInfo(name)
                member.mode = 0o644
                with self.assertRaises(ValueError):
                    bench.validate_member(member)
        link = tarfile.TarInfo('project/link')
        link.mode = 0o644
        link.type = tarfile.SYMTYPE
        with self.assertRaises(ValueError):
            bench.validate_member(link)
        big = tarfile.TarInfo('project/big')
        big.mode = 0o644
        big.size = bench.MAX_UNPACKED + 1
        with self.assertRaises(ValueError):
            bench.validate_member(big)

    def test_receipts_must_match_both_served_artifacts_and_all_runs(self):
        with tempfile.TemporaryDirectory(prefix='aego-stage12-receipt-test-') as temp:
            demo = Path(temp)
            (demo / 'data').mkdir()
            (demo / 'game.wasm').write_bytes(b'wasm')
            (demo / 'data/manifest.aetext').write_bytes(b'manifest')
            receipts = {'game.wasm': hashlib.sha256(b'wasm').hexdigest(),
                        'manifest.aetext': hashlib.sha256(b'manifest').hexdigest()}
            row = {'frames': 600, 'errors': [], 'receipts': receipts}
            report = {'results': {'chromium': {'runs': [row]}}}
            bench.verify_receipts(report, demo, 1)
            with self.assertRaises(ValueError):
                bench.verify_receipts(report, demo, 5)
            report['results']['chromium']['runs'][0]['receipts'] = {}
            with self.assertRaises(ValueError):
                bench.verify_receipts(report, demo, 1)


if __name__ == '__main__':
    unittest.main()
