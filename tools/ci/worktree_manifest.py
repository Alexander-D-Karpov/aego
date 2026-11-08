#!/usr/bin/env python3
"""Record the current Git worktree's tracked and non-ignored untracked bytes.

This is a local evidence inventory, not a Git tree object or a signature. It
includes missing tracked paths and symlink targets, but excludes ignored files,
the Git index and files outside the checkout. Capture it before and after a
test; equal inventories cannot rule out changes made and reverted in between.
"""
import argparse
import hashlib
import json
import os
from pathlib import Path
import stat
import subprocess


def git(root, *args):
    return subprocess.check_output(['git', '-C', str(root), *args])


def fingerprint(info):
    return (info.st_dev, info.st_ino, info.st_mode, info.st_size,
            info.st_mtime_ns, info.st_ctime_ns)


def inventory(root):
    root = Path(root).resolve(strict=True)
    head = git(root, 'rev-parse', 'HEAD').decode('ascii').strip()
    names = git(root, 'ls-files', '--cached', '--others', '--exclude-standard', '-z')
    paths = sorted(set(filter(None, names.split(b'\0'))))
    records = []
    for raw in paths:
        name = os.fsdecode(raw)
        file = root / name
        try:
            before = file.lstat()
        except FileNotFoundError:
            records.append({'path': name, 'kind': 'missing'})
            continue
        mode = stat.S_IMODE(before.st_mode)
        if stat.S_ISLNK(before.st_mode):
            target = os.fsencode(os.readlink(file))
            checksum = hashlib.sha256(target).hexdigest()
            kind = 'symlink'
        elif stat.S_ISREG(before.st_mode):
            h = hashlib.sha256()
            flags = os.O_RDONLY | getattr(os, 'O_NOFOLLOW', 0)
            fd = os.open(file, flags)
            with os.fdopen(fd, 'rb') as stream:
                if fingerprint(os.fstat(stream.fileno())) != fingerprint(before):
                    raise RuntimeError(f'file changed before reading: {name}')
                for chunk in iter(lambda: stream.read(1024 * 1024), b''):
                    h.update(chunk)
                if fingerprint(os.fstat(stream.fileno())) != fingerprint(before):
                    raise RuntimeError(f'file changed while reading: {name}')
            checksum = h.hexdigest()
            kind = 'file'
        else:
            raise RuntimeError(f'unsupported worktree entry, including submodules: {name}')
        if fingerprint(file.lstat()) != fingerprint(before):
            raise RuntimeError(f'worktree entry changed while reading: {name}')
        records.append({'path': name, 'kind': kind, 'mode': mode, 'sha256': checksum})
    canonical = json.dumps(records, ensure_ascii=True, sort_keys=True,
                           separators=(',', ':')).encode('ascii')
    return {
        'schema': 1,
        'head': head,
        'scope': 'tracked and non-ignored untracked worktree paths; ignored files and Git index excluded',
        'algorithm': 'SHA-256 of ASCII JSON files array, sorted keys, compact separators',
        'fileCount': len(records),
        'worktreeSHA256': hashlib.sha256(canonical).hexdigest(),
        'files': records,
    }


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--root', required=True, type=Path)
    parser.add_argument('--output', required=True, type=Path)
    args = parser.parse_args()
    root = args.root.resolve(strict=True)
    output = args.output.parent.resolve(strict=True) / args.output.name
    if output == root or root in output.parents:
        raise SystemExit('inventory output must be outside the checkout')
    result = inventory(root)
    with output.open('x', encoding='utf-8') as f:
        json.dump(result, f, ensure_ascii=True, indent=2)
        f.write('\n')
    print(f"worktree SHA-256 {result['worktreeSHA256']} ({result['fileCount']} paths)")


if __name__ == '__main__':
    main()
