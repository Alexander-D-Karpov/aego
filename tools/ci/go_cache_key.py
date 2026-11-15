#!/usr/bin/env python3
"""Identify a CI Go build cache from its compiler, native inputs and host image."""

import hashlib
import json
import os
from pathlib import Path
import platform
import shlex
import shutil
import stat
import subprocess
import sys


GO_FIELDS = ('GOVERSION', 'GOROOT', 'GOOS', 'GOARCH', 'GOAMD64', 'GOEXPERIMENT',
             'CGO_ENABLED', 'CC', 'CXX', 'CGO_CFLAGS', 'CGO_CPPFLAGS',
             'CGO_CXXFLAGS', 'CGO_FFLAGS', 'CGO_LDFLAGS', 'GOFLAGS', 'GOCACHE', 'GOMODCACHE')
ENV_FIELDS = ('ImageOS', 'ImageVersion', 'PKG_CONFIG', 'PKG_CONFIG_PATH',
              'PKG_CONFIG_LIBDIR', 'SDKROOT', 'MACOSX_DEPLOYMENT_TARGET',
              'LD_LIBRARY_PATH', 'DYLD_LIBRARY_PATH')


def command(args):
    process = subprocess.run(args, stdout=subprocess.PIPE, stderr=subprocess.PIPE, timeout=30)
    if process.returncode:
        raise ValueError(args[0] + ' failed with exit ' + str(process.returncode))
    return process.stdout.decode('utf-8').strip()


def file_hash(path):
    if path.is_symlink() or not path.is_file():
        raise ValueError('expected a regular file: ' + str(path))
    digest = hashlib.sha256()
    with path.open('rb') as stream:
        for chunk in iter(lambda: stream.read(1024 * 1024), b''):
            digest.update(chunk)
    return digest.hexdigest()


def fail_walk(error):
    raise error


def native_tree(root):
    if root.is_symlink() or not root.is_dir():
        raise ValueError('native SDL prefix is missing or linked')
    records = []
    for directory, dirs, files in os.walk(root, followlinks=False, onerror=fail_walk):
        for name in sorted(dirs + files):
            path = Path(directory) / name
            info = path.lstat()
            relative = path.relative_to(root).as_posix()
            if stat.S_ISDIR(info.st_mode):
                records.append((relative, 'dir', stat.S_IMODE(info.st_mode), ''))
            elif stat.S_ISREG(info.st_mode):
                records.append((relative, 'file', stat.S_IMODE(info.st_mode), file_hash(path)))
            else:
                raise ValueError('native SDL tree contains a link or special file: ' + relative)
    if not records:
        raise ValueError('native SDL tree is empty')
    return sorted(records)


def compiler_identity(value):
    args = shlex.split(value, posix=os.name != 'nt')
    if len(args) != 1:
        raise ValueError('compiler wrappers or extra arguments require a cold cache')
    path = shutil.which(args[0])
    if not path:
        raise ValueError('compiler not found: ' + args[0])
    binary = Path(path).resolve(strict=True)
    return {'path': str(binary), 'sha256': file_hash(binary), 'version': command([str(binary), '--version'])}


def identity(root):
    root = root.resolve(strict=True)
    go = json.loads(command(['go', 'env', '-json', *GO_FIELDS]))
    if set(go) != set(GO_FIELDS) or go['CGO_ENABLED'] != '1':
        raise ValueError('unexpected Go environment or CGO disabled')
    if go['GOOS'] not in ('linux', 'darwin', 'windows') or go['GOARCH'] not in ('amd64', 'arm64'):
        raise ValueError('unsupported cache platform')
    caches = []
    for name in ('GOCACHE', 'GOMODCACHE'):
        cache = Path(go[name])
        if not cache.is_absolute() or cache == Path(cache.anchor) or '\n' in str(cache) or '\r' in str(cache):
            raise ValueError('unsafe ' + name + ' directory')
        if cache.is_symlink() or (cache.exists() and not cache.is_dir()):
            raise ValueError(name + ' is linked or not a directory')
        caches.append(str(cache))
    if caches[0] == caches[1]:
        raise ValueError('Go build and module caches must be separate')
    host = platform.system()
    native = root / '.cache/native' / (go['GOOS'] + '_' + go['GOARCH'])
    environment = {name: os.environ.get(name, '') for name in ENV_FIELDS}
    if host == 'Linux':
        packages = command(['dpkg-query', '-W', '-f=${binary:Package}|${db:Status-Status}|${Version}\n'])
        if not packages or '|installed|' not in packages:
            raise ValueError('Debian package inventory unavailable')
        host_info = {'platform': platform.platform(), 'packages': sorted(packages.splitlines())}
    elif host == 'Darwin':
        if not environment['ImageVersion']:
            raise ValueError('hosted image version is missing')
        host_info = {'platform': platform.platform(), 'sdk': command(['xcrun', '--show-sdk-version']),
                     'sdk_path': command(['xcrun', '--show-sdk-path'])}
    elif host == 'Windows':
        if not environment['ImageVersion']:
            raise ValueError('hosted image version is missing')
        host_info = {'platform': platform.platform()}
    else:
        raise ValueError('unsupported host OS')
    contents = {'schema': 1, 'workspace': str(root), 'go': go, 'environment': environment,
                'host': host_info, 'cc': compiler_identity(go['CC']), 'cxx': compiler_identity(go['CXX']),
                'sdl': native_tree(native)}
    encoded = json.dumps(contents, sort_keys=True, separators=(',', ':'), ensure_ascii=True).encode()
    return hashlib.sha256(encoded).hexdigest(), *caches


def main():
    try:
        output = Path(os.environ['GITHUB_OUTPUT'])
        if not output.is_absolute() or output.is_symlink() or not output.is_file():
            raise ValueError('GITHUB_OUTPUT must be an existing regular file')
        key, cache, modules = identity(Path.cwd())
        with output.open('a', encoding='utf-8') as stream:
            stream.write('fingerprint=' + key + '\npath=' + cache + '\nmodules=' + modules + '\n')
        print('Go build cache environment fingerprint: ' + key)
        return 0
    except (OSError, ValueError, KeyError, UnicodeError, json.JSONDecodeError, subprocess.SubprocessError) as error:
        print('Go cache unavailable; tests must run with a cold cache: ' + str(error), file=sys.stderr)
        return 1


if __name__ == '__main__':
    raise SystemExit(main())
