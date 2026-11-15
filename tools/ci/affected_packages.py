#!/usr/bin/env python3
"""Select host Go packages conservatively, or fail rather than report an empty pass."""

import argparse
import json
import os
from pathlib import Path
import re
import subprocess
import sys


def run(args, cwd):
    result = subprocess.run(args, cwd=cwd, stdout=subprocess.PIPE, stderr=subprocess.PIPE, timeout=120)
    if result.returncode:
        message = result.stderr.decode(errors='replace').strip()
        raise ValueError(f'{args[0]} failed ({result.returncode}): {message[:600]}')
    return result.stdout


def status_paths(raw):
    fields = raw.split(b'\0')
    if fields[-1] != b'':
        raise ValueError('incomplete Git status output')
    fields.pop()
    paths = []
    while fields:
        status = fields.pop(0).decode('ascii')
        if not re.fullmatch(r'(?:[ACDMRTUXB][0-9]*)', status):
            raise ValueError('unrecognized Git change status: ' + status)
        count = 2 if status[0] in 'RC' else 1
        if len(fields) < count:
            raise ValueError('incomplete Git rename or path')
        paths.extend(os.fsdecode(fields.pop(0)) for _ in range(count))
    return paths


def changed_paths(root, base):
    if run(['git', 'rev-parse', '--is-shallow-repository'], root).strip() != b'false':
        raise ValueError('shallow checkout: run the full CI in a protected snapshot')
    revision = run(['git', 'rev-parse', '--verify', '--end-of-options', base + '^{commit}'], root).decode().strip()
    if not re.fullmatch(r'[0-9a-f]{40}|[0-9a-f]{64}', revision):
        raise ValueError('base is not a commit')
    merge_bases = run(['git', 'merge-base', '--all', revision, 'HEAD'], root).decode().splitlines()
    if len(merge_bases) != 1:
        raise ValueError('missing or ambiguous merge base: run the full CI')
    paths = []
    for args in (['git', 'diff', '--name-status', '-z', '-M', merge_bases[0], 'HEAD', '--'],
                 ['git', 'diff', '--name-status', '-z', '-M', '--'],
                 ['git', 'diff', '--cached', '--name-status', '-z', '-M', '--']):
        paths.extend(status_paths(run(args, root)))
    untracked = run(['git', 'ls-files', '--others', '--exclude-standard', '-z'], root)
    if untracked and not untracked.endswith(b'\0'):
        raise ValueError('incomplete untracked file output')
    paths.extend(os.fsdecode(path) for path in untracked.split(b'\0') if path)
    return sorted(set(paths))


def objects(raw):
    text = raw.decode('utf-8')
    decoder = json.JSONDecoder()
    cursor = 0
    while cursor < len(text):
        while cursor < len(text) and text[cursor].isspace():
            cursor += 1
        if cursor == len(text):
            break
        obj, cursor = decoder.raw_decode(text, cursor)
        if not isinstance(obj, dict):
            raise ValueError('unexpected Go package metadata')
        yield obj


def canonical(name):
    return name.split(' [', 1)[0]


def affected_packages(root, paths):
    if not paths:
        return []
    non_go = [path for path in paths if not path.endswith('.go')]
    if non_go:
        raise ValueError('non-Go changes require a broader plan/full CI; not selecting only Go packages: '
                         + ', '.join(repr(path) for path in non_go[:5]))
    module = run(['go', 'list', '-m'], root).decode().strip()
    if not module or any(ch.isspace() for ch in module):
        raise ValueError('invalid Go module name')
    wanted = set()
    for name in paths:
        path = Path(name)
        if path.is_absolute() or '..' in path.parts:
            raise ValueError('invalid Git path')
        directory = root / path.parent
        if not directory.is_dir():
            raise ValueError('deleted package requires a broader plan/full CI: ' + name)
        current = directory
        while current != root:
            if (current / 'go.mod').exists():
                raise ValueError('nested Go module requires a broader plan/full CI: ' + name)
            current = current.parent
        wanted.add(module if path.parent == Path('.') else module + '/' + path.parent.as_posix())
    metadata = list(objects(run(['go', 'list', '-deps', '-test', '-json', './...'], root)))
    graph = {}
    for package in metadata:
        name = package.get('ImportPath', '')
        owner = package.get('ForTest') or canonical(name)
        if not isinstance(owner, str) or not (owner == module or owner.startswith(module + '/')) or owner.endswith('.test'):
            continue
        if package.get('Error') or package.get('DepsErrors') or package.get('Incomplete'):
            raise ValueError('incomplete Go dependency graph: ' + owner)
        dependencies = graph.setdefault(owner, set())
        for key in ('Imports', 'Deps', 'TestImports', 'XTestImports'):
            for dependency in package.get(key, []):
                if not isinstance(dependency, str):
                    raise ValueError('invalid Go dependency metadata')
                dependencies.add(canonical(dependency))
    missing = wanted - graph.keys()
    if missing:
        raise ValueError('changed package absent from host graph; run broader checks: ' + ', '.join(sorted(missing)))
    affected = set(wanted)
    while True:
        added = {name for name, dependencies in graph.items() if dependencies & affected} - affected
        if not added:
            break
        affected.update(added)
    return sorted(affected)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('base', nargs='?', default='origin/master')
    args = parser.parse_args()
    try:
        root = Path(os.fsdecode(run(['git', 'rev-parse', '--show-toplevel'], Path.cwd())).strip()).resolve(strict=True)
        paths = changed_paths(root, args.base)
        packages = affected_packages(root, paths)
        if packages:
            print('Host Go graph only; platform, browser and full CI gates remain required.', file=sys.stderr)
            print('\n'.join(packages))
        return 0
    except (OSError, ValueError, UnicodeError, json.JSONDecodeError, subprocess.SubprocessError) as error:
        print('affected selector failed: ' + str(error), file=sys.stderr)
        return 2


if __name__ == '__main__':
    raise SystemExit(main())
