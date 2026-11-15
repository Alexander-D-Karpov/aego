#!/usr/bin/env python3
"""Use the verified image Staticcheck or preserve the local go-run behavior."""

import json
import os
from pathlib import Path
import subprocess
import sys

from prepare_ci_workspace import PREFLIGHT


IMAGE = Path('/opt/aego')


def select_command(root, image, version, args, mode):
    if mode == '0':
        return ['go', 'run', 'honnef.co/go/tools/cmd/staticcheck@' + version, *args]
    if mode != '1':
        raise ValueError('invalid AEGO_CI_PREPARED value')
    root = root.resolve(strict=True)
    image = image.resolve(strict=True)
    source_lock = root / 'tools/ci/env/lock.json'
    image_lock = image / 'ci-env/lock.json'
    if source_lock.is_symlink() or image_lock.is_symlink() or source_lock.read_bytes() != image_lock.read_bytes():
        raise ValueError('checkout and prepared image locks differ')
    lock = json.loads(image_lock.read_bytes())
    if lock.get('schema') != 1 or lock.get('platform') != 'linux/amd64':
        raise ValueError('unsupported prepared image lock')
    if version != lock['tools']['staticcheck']:
        raise ValueError('requested Staticcheck version differs from prepared image')
    PREFLIGHT.check_input_set(root, lock)
    PREFLIGHT.check_input_set(image, lock, image=True)
    binary = image / 'bin/staticcheck'
    if binary.is_symlink() or not binary.is_file() or not os.access(binary, os.X_OK):
        raise ValueError('prepared Staticcheck binary is missing or not executable')
    # Verify Go, Staticcheck build metadata and the other pinned tool versions.
    PREFLIGHT.check_tools(lock)
    return [str(binary), *args]


def main():
    if len(sys.argv) < 3 or sys.argv[2] != '--':
        raise SystemExit('usage: staticcheck_runner.py VERSION -- [staticcheck arguments]')
    try:
        argv = select_command(Path.cwd(), IMAGE, sys.argv[1], sys.argv[3:], os.environ.get('AEGO_CI_PREPARED', '0'))
        os.execvp(argv[0], argv)
    except (OSError, ValueError, KeyError, TypeError, subprocess.SubprocessError) as error:
        print('Staticcheck preparation failed: ' + str(error), file=sys.stderr)
        return 1


if __name__ == '__main__':
    raise SystemExit(main())
