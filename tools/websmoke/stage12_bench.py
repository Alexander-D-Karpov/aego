#!/usr/bin/env python3
"""Run the original Stage 12 workload on the current engine and old baseline.

The fixture is a byte-verified archive of tracked files from Stage 12, not a
checkout of the current platformer.  Build and generated files live in a new
private temporary directory; neither the fixture nor any go.mod is edited.
"""
import argparse
import hashlib
import io
import json
import os
from pathlib import Path, PurePosixPath
import subprocess
import tarfile
import tempfile

ROOT = Path(__file__).resolve().parents[2]
FIXTURE = ROOT / 'tools/websmoke/fixtures/stage12-workload.tar.gz'
BASELINE = ROOT / 'tools/websmoke/bench-stage12-baseline.json'
FIXTURE_SHA256 = 'e9f4b2358b7624835ba2e574a53aaf6eb0add3365288c4005453b897a751f144'
BASELINE_SHA256 = '9bcc3363b068d4552a82e5fae81c3f133c26e0366d62ddb39878ca10334916a7'
REPLAY_SHA256 = '86383d48abadcdb6b91dca38375f0ba9546945c9316f2f500610240189212343'
MAX_ARCHIVE = 4 * 1024 * 1024
MAX_UNPACKED = 16 * 1024 * 1024


def digest(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def validate_member(member):
    name = member.name
    path = PurePosixPath(name)
    if (not member.isfile() or member.mode != 0o644 or member.size < 0 or '\\' in name or
            member.size > MAX_UNPACKED or path.is_absolute() or
            any(part in ('', '.', '..') for part in path.parts) or path.as_posix() != name or
            not (name == 'replay.aeinput' or name.startswith('project/'))):
        raise ValueError('invalid Stage 12 fixture member: ' + name)


def extract_fixture(archive, destination):
    if destination.is_symlink() or not destination.is_dir() or any(destination.iterdir()):
        raise ValueError('fixture destination must be a new empty directory')
    raw = archive.read_bytes()
    if len(raw) > MAX_ARCHIVE or hashlib.sha256(raw).hexdigest() != FIXTURE_SHA256:
        raise ValueError('Stage 12 fixture archive does not match its reviewed SHA-256')
    with tarfile.open(fileobj=io.BytesIO(raw), mode='r:gz') as tar:
        members = tar.getmembers()
        names = set()
        total = 0
        for member in members:
            validate_member(member)
            if member.name in names:
                raise ValueError('duplicate Stage 12 fixture member: ' + member.name)
            names.add(member.name)
            total += member.size
        if len(members) != 126 or total > MAX_UNPACKED or 'replay.aeinput' not in names or 'project/go.mod' not in names:
            raise ValueError('unexpected Stage 12 fixture contents')
        for member in members:
            target = destination.joinpath(*PurePosixPath(member.name).parts)
            target.parent.mkdir(parents=True, exist_ok=True)
            source = tar.extractfile(member)
            if source is None:
                raise ValueError('missing fixture file: ' + member.name)
            with source, target.open('xb') as output:
                while block := source.read(1024 * 1024):
                    output.write(block)
            os.chmod(target, 0o644)
    if digest(destination / 'replay.aeinput') != REPLAY_SHA256:
        raise ValueError('Stage 12 replay differs from the reviewed workload')


def verify_receipts(report, demo, runs):
    expected = {'game.wasm': digest(demo / 'game.wasm'),
                'manifest.aetext': digest(demo / 'data/manifest.aetext')}
    rows = report.get('results', {}).get('chromium', {}).get('runs', [])
    if (len(rows) != runs or any(row.get('frames') != 600 or row.get('errors') != [] or
                                 row.get('receipts') != expected for row in rows)):
        raise ValueError('Stage 12 benchmark HTTP receipts or measurements differ from the exported artifact')


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--runs', type=int, default=1, help='measurement runs; five or more also compare timing')
    args = parser.parse_args()
    if not 1 <= args.runs <= 20:
        parser.error('--runs must be between 1 and 20')
    if digest(BASELINE) != BASELINE_SHA256:
        raise ValueError('historical Stage 12 baseline changed; review the fixture and baseline together')
    cli = ROOT / '.cache/bin/aego'
    if not cli.is_file():
        raise ValueError('build the current CLI first: make cli')
    env = os.environ.copy()
    env['AEGO_CI_REQUIRE_WEB_BASELINE'] = '1'
    with tempfile.TemporaryDirectory(prefix='aego-stage12-bench-') as temp:
        work = Path(temp)
        fixture = work / 'fixture'
        fixture.mkdir()
        extract_fixture(FIXTURE, fixture)
        project = fixture / 'project'
        env['GOWORK'] = 'off'
        subprocess.run(['go', 'work', 'init', str(ROOT), str(project)], cwd=work, env=env, check=True)
        env['GOWORK'] = str(work / 'go.work')
        module = json.loads(subprocess.check_output(['go', 'list', '-m', '-json', 'aego'], cwd=project, env=env))
        if module.get('Path') != 'aego' or not module.get('Main') or Path(module.get('Dir', '')).resolve() != ROOT:
            raise ValueError('historical workload is not using the current engine workspace')
        demo = work / 'demo'
        subprocess.run(['sh', str(ROOT / 'tools/site-demo.sh'), str(demo), str(cli), 'export', str(project),
                        '--target', 'web', '--release', '--engine', str(ROOT)], cwd=ROOT, env=env, check=True)
        result = work / 'bench.json'
        subprocess.run(['node', 'bench.mjs', '--root=' + str(demo), '--replay=' + str(fixture / 'replay.aeinput'),
                        '--out=' + str(result), '--baseline=' + str(BASELINE), '--browsers=chromium',
                        '--runs=' + str(args.runs), '--profile=true', '--receipts=true', '--gate'],
                       cwd=ROOT / 'tools/websmoke', env=env, check=True)
        verify_receipts(json.loads(result.read_text()), demo, args.runs)
        if digest(BASELINE) != BASELINE_SHA256 or digest(FIXTURE) != FIXTURE_SHA256:
            raise ValueError('historical fixture or baseline changed during the benchmark')
        print('Historical Stage 12 workload, baseline and HTTP receipts verified', flush=True)


if __name__ == '__main__':
    main()
