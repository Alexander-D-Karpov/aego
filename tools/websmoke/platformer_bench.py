#!/usr/bin/env python3
"""Gate the original tracked platformer against its existing web baseline.

The website demo deliberately adjusts one floor; it is a different workload.
Only tracked project inputs are copied to a private directory. Nothing is
exported into the user's project or into the site directory.
"""
import argparse
import hashlib
import json
import os
from pathlib import Path, PurePosixPath
import subprocess
import tempfile


ROOT = Path(__file__).resolve().parents[2]
PROJECT = 'examples/platformer'
BASELINE = ROOT / 'tools/websmoke/bench-platformer-head-baseline.json'
BASELINE_SHA256 = '15eaf504785fb0b4c4468ee18a52a7fee4a6060dcce3091ceab64f2f38d6bea5'
REPLAY = ROOT / 'tests/integration/testdata/platformer_run.aeinput'
REPLAY_SHA256 = 'a59af4a9456873767ff6b84dd3944e585849b99a9c7726983d960ffd7acafeb1'
# This tracked input set is byte-identical to the platformer at a46347e2,
# where the original platformer reference was recorded.
PROJECT_SHA256 = '8b4ceeff7b06ca3e7a4eb043d21ba8f9d89f63f7e5e5c38284bef677880d6ce7'
MAX_FILE_BYTES = 64 * 1024 * 1024
MAX_PROJECT_BYTES = 256 * 1024 * 1024
MAX_FILES = 10000


def digest(path, limit=None):
    h = hashlib.sha256()
    size = 0
    with path.open('rb') as source:
        for chunk in iter(lambda: source.read(1024 * 1024), b''):
            size += len(chunk)
            if limit is not None and size > limit:
                raise ValueError('file exceeds the copy budget: ' + str(path))
            h.update(chunk)
    return h.hexdigest()


def copy_tracked_project(root, dest):
    """Copy only clean, regular tracked files; reject staged edits and symlinks."""
    root = Path(root).resolve(strict=True)
    dest = Path(dest)
    if dest.is_symlink() or not dest.is_dir() or any(dest.iterdir()):
        raise ValueError('project destination must be a new empty directory')
    dest = dest.resolve(strict=True)
    project = root / PROJECT
    if not project.is_dir() or project.is_symlink() or dest == project or project in dest.parents or dest in project.parents:
        raise ValueError('source and destination must be separate directories')
    for args in (['git', 'diff', '--quiet', 'HEAD', '--', PROJECT],
                 ['git', 'diff', '--cached', '--quiet', 'HEAD', '--', PROJECT]):
        diff = subprocess.run(args, cwd=root, stdin=subprocess.DEVNULL, stdout=subprocess.DEVNULL, stderr=subprocess.PIPE)
        if diff.returncode:
            raise ValueError('tracked platformer inputs must match HEAD')
    raw = subprocess.check_output(['git', 'ls-files', '-s', '-z', '--', PROJECT], cwd=root)
    entries = [entry for entry in raw.split(b'\0') if entry]
    if not entries or len(entries) > MAX_FILES:
        raise ValueError('unexpected tracked platformer inventory')
    copied = {}
    total = 0
    for entry in entries:
        info, separator, raw_name = entry.partition(b'\t')
        fields = info.split(b' ')
        if not separator or len(fields) != 3 or fields[0] not in (b'100644', b'100755') or fields[2] != b'0':
            raise ValueError('tracked project entries must be regular stage-zero files')
        name = os.fsdecode(raw_name)
        path = PurePosixPath(name)
        prefix = PurePosixPath(PROJECT)
        if (path.is_absolute() or path.as_posix() != name or prefix not in path.parents or
                any(part in ('', '.', '..') for part in path.parts)):
            raise ValueError('invalid tracked project path')
        relative = path.relative_to(prefix)
        source = root
        for part in path.parts:
            source = source / part
            if source.is_symlink():
                raise ValueError('tracked project inputs cannot contain symlinks')
        if not source.is_file():
            raise ValueError('tracked project input is missing: ' + name)
        length = source.stat().st_size
        total += length
        if length > MAX_FILE_BYTES or total > MAX_PROJECT_BYTES:
            raise ValueError('tracked project exceeds the copy budget')
        target = dest.joinpath(*relative.parts)
        target.parent.mkdir(parents=True, exist_ok=True)
        before = digest(source, MAX_FILE_BYTES)
        with source.open('rb') as src, target.open('xb') as out:
            written = 0
            while chunk := src.read(64 * 1024):
                written += len(chunk)
                if written > MAX_FILE_BYTES or total - length + written > MAX_PROJECT_BYTES:
                    raise ValueError('tracked project exceeds the copy budget')
                out.write(chunk)
        if digest(target, MAX_FILE_BYTES) != before or digest(source, MAX_FILE_BYTES) != before:
            raise ValueError('tracked project changed while being copied: ' + name)
        os.chmod(target, 0o755 if fields[0] == b'100755' else 0o644)
        copied[name] = before
    if 'examples/platformer/go.mod' not in copied or 'examples/platformer/scenes/main.aescene' not in copied:
        raise ValueError('tracked platformer inputs are incomplete')
    return copied


def inventory_digest(copied):
    h = hashlib.sha256()
    for name, value in sorted(copied.items()):
        h.update(name.encode('utf-8'))
        h.update(b'\0')
        h.update(value.encode('ascii'))
        h.update(b'\n')
    return h.hexdigest()


def verify_receipts(report, demo, runs):
    expected = {'game.wasm': digest(demo / 'game.wasm'),
                'manifest.aetext': digest(demo / 'data/manifest.aetext')}
    rows = report.get('results', {}).get('chromium', {}).get('runs', [])
    if (len(rows) != runs or any(row.get('frames') != 600 or row.get('errors') != [] or
                                 row.get('receipts') != expected for row in rows)):
        raise ValueError('platformer benchmark receipts do not match the exported artifact')
    return expected


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--runs', type=int, default=1, help='five runs also compare timing')
    args = parser.parse_args()
    if not 1 <= args.runs <= 20:
        parser.error('--runs must be between 1 and 20')
    if digest(BASELINE) != BASELINE_SHA256 or digest(REPLAY) != REPLAY_SHA256:
        raise ValueError('original platformer baseline or replay changed; review it independently')
    cli = ROOT / '.cache/bin/aego'
    if not cli.is_file():
        raise ValueError('build the current CLI first: make cli')
    env = dict(os.environ, AEGO_CI_REQUIRE_WEB_BASELINE='1', GOWORK='off')
    with tempfile.TemporaryDirectory(prefix='aego-platformer-bench-') as temp:
        work = Path(temp)
        project = work / 'project'
        project.mkdir()
        copied = copy_tracked_project(ROOT, project)
        if inventory_digest(copied) != PROJECT_SHA256:
            raise ValueError('tracked platformer differs from the original baseline workload')
        subprocess.run(['go', 'work', 'init', str(ROOT), str(project)], cwd=work, env=env, check=True)
        env['GOWORK'] = str(work / 'go.work')
        module = json.loads(subprocess.check_output(['go', 'list', '-m', '-json', 'aego'], cwd=project, env=env))
        if module.get('Path') != 'aego' or not module.get('Main') or Path(module.get('Dir', '')).resolve() != ROOT:
            raise ValueError('original workload is not using the current engine workspace')
        demo = work / 'demo'
        subprocess.run(['sh', str(ROOT / 'tools/site-demo.sh'), str(demo), str(cli), 'export', str(project),
                        '--target', 'web', '--release', '--engine', str(ROOT)], cwd=ROOT, env=env, check=True)
        report = work / 'bench.json'
        subprocess.run(['node', 'bench.mjs', '--root=' + str(demo), '--replay=' + str(REPLAY),
                        '--out=' + str(report), '--baseline=' + str(BASELINE), '--browsers=chromium',
                        '--runs=' + str(args.runs), '--profile=true', '--receipts=true', '--gate'],
                       cwd=ROOT / 'tools/websmoke', env=env, check=True)
        receipts = verify_receipts(json.loads(report.read_text()), demo, args.runs)
        if (digest(BASELINE) != BASELINE_SHA256 or digest(REPLAY) != REPLAY_SHA256 or
                any(digest(ROOT / name, MAX_FILE_BYTES) != h for name, h in copied.items())):
            raise ValueError('original baseline, replay or tracked project changed during the benchmark')
        print('Original tracked platformer, baseline and HTTP receipts verified: ' + json.dumps({
            'runs': args.runs, 'baselineSHA256': BASELINE_SHA256, 'replaySHA256': REPLAY_SHA256,
            'projectSHA256': PROJECT_SHA256, 'sceneSHA256': copied['examples/platformer/scenes/main.aescene'],
            'receipts': receipts,
        }, sort_keys=True), flush=True)


if __name__ == '__main__':
    main()
