#!/usr/bin/env python3
"""Bound optional Go-cache uploads without removing any existing cache."""

import os
from pathlib import Path
import shutil
import stat
import sys


MAX_BYTES = 3 * 1024**3
MIN_FREE_BYTES = 50 * 1024**3


def assess(paths, max_bytes=MAX_BYTES, min_free=MIN_FREE_BYTES):
    roots = []
    for value in paths:
        path = Path(value)
        if not path.is_absolute() or path == Path(path.anchor) or path.is_symlink():
            raise ValueError('unsafe cache path')
        if path.exists() and not path.is_dir():
            raise ValueError('cache path is not a directory')
        roots.append(path.resolve(strict=False))
    if len(roots) != 2 or roots[0] == roots[1] or roots[0] in roots[1].parents or roots[1] in roots[0].parents:
        raise ValueError('cache paths overlap')
    if not roots[0].is_dir():
        return False, 0, 'build cache is missing'
    size = 0
    for root in roots:
        if not root.exists():
            continue
        if shutil.disk_usage(root).free < min_free:
            return False, size, 'insufficient free storage'
        for directory, dirs, files in os.walk(root, followlinks=False):
            for name in dirs + files:
                path = Path(directory) / name
                info = path.lstat()
                if stat.S_ISREG(info.st_mode):
                    size += info.st_size
                    if size > max_bytes:
                        return False, size, 'cache exceeds upload bound'
                elif not stat.S_ISDIR(info.st_mode):
                    raise ValueError('cache contains a link or special file')
    return True, size, 'within upload bound'


def main():
    try:
        output = Path(os.environ['GITHUB_OUTPUT'])
        if not output.is_absolute() or output.is_symlink() or not output.is_file():
            raise ValueError('GITHUB_OUTPUT must be an existing regular file')
        max_bytes = int(os.environ.get('GO_CACHE_MAX_BYTES', MAX_BYTES))
        min_free = int(os.environ.get('GO_CACHE_MIN_FREE_BYTES', MIN_FREE_BYTES))
        if not 0 < max_bytes <= MAX_BYTES or not 2 * 1024**3 <= min_free <= MIN_FREE_BYTES:
            raise ValueError('invalid cache size or free-space bound')
        allowed, size, reason = assess((os.environ['GO_BUILD_CACHE_PATH'], os.environ['GO_MODULE_CACHE_PATH']),
                                       max_bytes=max_bytes, min_free=min_free)
        with output.open('a', encoding='utf-8') as stream:
            stream.write('save=' + ('yes' if allowed else 'no') + '\nbytes=' + str(size) + '\n')
        print('Optional Go cache: ' + reason + ' (' + str(size) + ' bytes checked)')
        return 0
    except (OSError, ValueError, KeyError) as error:
        print('Go cache upload skipped: ' + str(error), file=sys.stderr)
        return 1


if __name__ == '__main__':
    raise SystemExit(main())
