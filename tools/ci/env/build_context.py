#!/usr/bin/env python3
"""Create a new, minimal CI-image build context without copying user files."""

import argparse
import hashlib
import json
from pathlib import Path
import shutil
import sys

import preflight


def build_context(root, output):
    root = root.resolve(strict=True)
    if not output.is_absolute() or output.name in ('', '.', '..'):
        raise ValueError('context must have a new absolute path')
    parent = output.parent.resolve(strict=True)
    destination = parent / output.name
    if destination == root or root in destination.parents:
        raise ValueError('context must be outside the checkout')
    if destination.exists() or destination.is_symlink():
        raise ValueError('context path already exists')
    lock_path = root / 'tools/ci/env/lock.json'
    if lock_path.is_symlink() or not lock_path.is_file():
        raise ValueError('environment lock must be a regular file')
    lock_bytes = lock_path.read_bytes()
    lock = json.loads(lock_bytes)
    preflight.check_input_set(root, lock)
    preflight.check_go_directives(root, lock['tools']['go'])
    destination.mkdir(mode=0o700)
    paths = sorted(preflight.INPUTS) + ['tools/ci/env/lock.json']
    for name in paths:
        source = root / name
        target = destination / name
        target.parent.mkdir(parents=True, exist_ok=True)
        with source.open('rb') as source_stream, target.open('xb') as target_stream:
            shutil.copyfileobj(source_stream, target_stream)
        if name == 'tools/ci/env/lock.json':
            if target.read_bytes() != lock_bytes:
                raise ValueError('environment lock changed during context creation')
        elif preflight.digest(target) != lock['inputs'][name]:
            raise ValueError('input changed during context creation: ' + name)
    preflight.check_input_set(root, lock)
    if lock_path.read_bytes() != lock_bytes:
        raise ValueError('environment lock changed during context creation')
    return {'status': 'PASS', 'context': str(destination), 'files': paths,
            'lock_sha256': hashlib.sha256(lock_bytes).hexdigest(),
            'note': 'context identity only; not an image build or CI result'}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--root', type=Path, default=Path(__file__).resolve().parents[3])
    parser.add_argument('--out', type=Path, required=True)
    args = parser.parse_args()
    try:
        print(json.dumps(build_context(args.root, args.out), indent=2))
    except (OSError, ValueError, KeyError, TypeError) as error:
        print('CI image context failed: ' + str(error), file=sys.stderr)
        return 1
    return 0


if __name__ == '__main__':
    raise SystemExit(main())
