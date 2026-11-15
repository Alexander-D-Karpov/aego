#!/usr/bin/env python3
"""Recommend local checks for changed files without replacing any CI gate."""

import argparse
import json
import os
from pathlib import Path, PurePosixPath
import subprocess
import sys

import affected_packages


# These are advisory starting points, not a complete dependency graph. Unknown
# files, build inputs and toolchain changes must fall back to the complete CI.
RULES = {
    'ci-infrastructure': ('ci-infrastructure-test', 'ci-full'),
    'go': (),
    'docs': ('site-check', 'web-smoke', 'component-try'),
    'web': ('ci-fast', 'web-smoke', 'stage12-release-check', 'component-try'),
    'studio-web': ('studio-web-test', 'web-smoke', 'component-try'),
    'shader': ('shader-validator', 'materials-check', 'test-gl', 'web-materials',
               'web-gpu-smoke', 'web-lighting'),
    'unknown': ('ci-full',),
}


def categories(name):
    path = PurePosixPath(name)
    if not name or path.is_absolute() or any(part in ('', '.', '..') for part in name.split('/')):
        raise ValueError('invalid changed path: ' + repr(name))
    lower = name.lower()
    basename = path.name.lower()
    if (name.startswith(('.github/', 'tools/ci/')) or basename in
            ('makefile', 'go.mod', 'go.sum', 'go.work', 'go.work.sum', 'dockerfile',
             'package.json', 'package-lock.json') or
            basename.endswith(('.mk', '.sh', '.ps1', '.yml', '.yaml'))):
        return ['ci-infrastructure']
    if name.endswith('.go'):
        return ['go']
    if lower.endswith(('.glsl', '.vert', '.frag', '.aesh')):
        return ['shader']
    if name.startswith('tools/artstudio/release/web/') and lower.endswith(('.js', '.mjs', '.cjs', '.css', '.html')):
        return ['studio-web']
    if name.startswith(('tools/websmoke/', 'tools/gendocs/site/static/')) and lower.endswith(
            ('.js', '.mjs', '.cjs', '.css', '.html')):
        return ['web']
    if name.startswith(('docs/', 'engine/res/docs/')) and lower.endswith(('.md', '.markdown')):
        return ['docs']
    return ['unknown']


def plan(root, paths):
    groups = []
    targets = set()
    go_paths = []
    for name in sorted(set(paths)):
        names = categories(name)
        groups.append({'path': name, 'categories': names})
        for category in names:
            targets.update(RULES[category])
        if 'go' in names:
            go_paths.append(name)
    packages = []
    go_graph_error = None
    if go_paths:
        try:
            packages = affected_packages.affected_packages(root, go_paths)
            if not packages:
                raise ValueError('Go changes produced an empty host package graph')
        except (OSError, ValueError, UnicodeError, json.JSONDecodeError, subprocess.SubprocessError) as error:
            go_graph_error = str(error)
            targets.add('ci-full')
    return {
        'schema': 1,
        'scope': 'local advisory only; no checks were run and the full CI remains mandatory',
        'full_ci_required': True,
        'changes': groups,
        'suggested_make_targets': sorted(targets),
        'host_go_packages': packages,
        'host_go_graph_error': go_graph_error,
        'limitations': ['Go packages cover only the current host graph and build configuration',
                        'non-Go suggestions are not a complete dependency graph',
                        'Git and working-tree observations are not atomic'],
    }


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--base', default='origin/master')
    args = parser.parse_args()
    try:
        root = Path(os.fsdecode(affected_packages.run(
            ['git', 'rev-parse', '--show-toplevel'], Path.cwd())).strip()).resolve(strict=True)
        paths = affected_packages.changed_paths(root, args.base)
        result = plan(root, paths)
        result['base'] = args.base
        print(json.dumps(result, indent=2, ensure_ascii=True))
        return 0
    except (OSError, ValueError, UnicodeError, json.JSONDecodeError, subprocess.SubprocessError) as error:
        print('change plan failed: ' + str(error), file=sys.stderr)
        return 2


if __name__ == '__main__':
    raise SystemExit(main())
