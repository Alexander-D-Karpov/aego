#!/usr/bin/env python3
"""Attach prepared CI dependencies to a fresh checkout without replacing files."""

import argparse
import hashlib
import importlib.util
import json
import os
from pathlib import Path
import shutil
import stat
import sys


SCRIPT = Path(__file__).resolve().parent / 'env/preflight.py'
SPEC = importlib.util.spec_from_file_location('aego_ci_env_preflight', SCRIPT)
PREFLIGHT = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(PREFLIGHT)
IMAGE = Path('/opt/aego')


def tree(root):
    if root.is_symlink() or not root.is_dir():
        raise ValueError('expected a real SDL directory: ' + str(root))
    records = []
    for directory, dirs, files in os.walk(root, followlinks=False):
        for name in sorted(dirs + files):
            path = Path(directory) / name
            info = path.lstat()
            relative = path.relative_to(root).as_posix()
            if stat.S_ISDIR(info.st_mode):
                records.append((relative, 'dir', stat.S_IMODE(info.st_mode), ''))
            elif stat.S_ISREG(info.st_mode):
                records.append((relative, 'file', stat.S_IMODE(info.st_mode), PREFLIGHT.digest(path)))
            else:
                raise ValueError('SDL tree contains a symlink or special file: ' + str(path))
    return sorted(records)


def existing_directory(path, *, required=False):
    if path.is_symlink() or (path.exists() and not path.is_dir()):
        raise ValueError('workspace directory is a symlink or non-directory: ' + str(path))
    if required and not path.is_dir():
        raise ValueError('workspace directory is missing: ' + str(path))


def prepare(root, image=IMAGE):
    if root.is_symlink():
        raise ValueError('checkout root must not be a symlink')
    root = root.resolve(strict=True)
    image = image.resolve(strict=True)
    lock_file = image / 'ci-env/lock.json'
    source_lock = root / 'tools/ci/env/lock.json'
    if lock_file.is_symlink() or source_lock.is_symlink() or lock_file.read_bytes() != source_lock.read_bytes():
        raise ValueError('checkout and image locks differ')
    lock = json.loads(lock_file.read_bytes())
    if lock.get('schema') != 1 or lock.get('platform') != 'linux/amd64':
        raise ValueError('unsupported environment lock')
    PREFLIGHT.check_input_set(image, lock, image=True)
    PREFLIGHT.check_input_set(root, lock)
    PREFLIGHT.check_go_directives(root, lock['tools']['go'])

    for path in (root / 'tools', root / 'tools/websmoke'):
        existing_directory(path, required=True)
    for path in (root / '.cache', root / '.cache/native'):
        existing_directory(path)
    source_sdl = image / 'native/linux_amd64'
    expected = tree(source_sdl)
    if not expected:
        raise ValueError('prepared SDL tree is empty')
    target_sdl = root / '.cache/native/linux_amd64'
    if target_sdl.exists() or target_sdl.is_symlink():
        if tree(target_sdl) != expected:
            raise ValueError('existing SDL tree differs; it has not been modified')
        copy_sdl = False
    else:
        copy_sdl = True

    modules = image / 'websmoke/node_modules'
    if modules.is_symlink() or not modules.is_dir():
        raise ValueError('prepared Node dependency tree is missing or linked')
    link = root / 'tools/websmoke/node_modules'
    if link.exists() or link.is_symlink():
        if not link.is_symlink() or link.resolve(strict=True) != modules.resolve(strict=True):
            raise ValueError('existing Node dependencies differ; they have not been modified')
        create_link = False
    else:
        create_link = True

    for path in (root / '.cache', root / '.cache/native'):
        if not path.exists():
            path.mkdir(mode=0o755)
        existing_directory(path, required=True)
    if copy_sdl:
        shutil.copytree(source_sdl, target_sdl, symlinks=False)
        if tree(target_sdl) != expected:
            raise ValueError('copied SDL tree differs; it was retained for inspection')
    if create_link:
        os.symlink(str(modules), link, target_is_directory=True)
    return {'status': 'PASS', 'sdl_copied': copy_sdl, 'node_link_created': create_link,
            'lock_sha256': hashlib.sha256(lock_file.read_bytes()).hexdigest(),
            'note': 'workspace preparation only; run the full image/workspace preflight next'}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--root', type=Path, required=True)
    args = parser.parse_args()
    try:
        print(json.dumps(prepare(args.root), indent=2))
    except (OSError, ValueError, KeyError, TypeError) as error:
        print('CI workspace preparation failed: ' + str(error), file=sys.stderr)
        return 1
    return 0


if __name__ == '__main__':
    raise SystemExit(main())
