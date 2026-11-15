#!/usr/bin/env python3
"""Verify prepared Node dependencies without installing or changing them."""

import argparse
import json
from pathlib import Path
import subprocess
import sys

from prepare_ci_workspace import PREFLIGHT


IMAGE = Path('/opt/aego')
PACKAGES = ('playwright-core', 'pixelmatch', 'pngjs')


def verify(root, image=IMAGE):
    root = root.resolve(strict=True)
    image = image.resolve(strict=True)
    source_lock = root / 'tools/ci/env/lock.json'
    image_lock = image / 'ci-env/lock.json'
    if source_lock.is_symlink() or image_lock.is_symlink() or source_lock.read_bytes() != image_lock.read_bytes():
        raise ValueError('checkout and prepared image locks differ')
    lock = json.loads(image_lock.read_bytes())
    if lock.get('schema') != 1 or lock.get('platform') != 'linux/amd64':
        raise ValueError('unsupported prepared image lock')
    PREFLIGHT.check_input_set(root, lock)
    PREFLIGHT.check_input_set(image, lock, image=True)
    modules = image / 'websmoke/node_modules'
    link = root / 'tools/websmoke/node_modules'
    if modules.is_symlink() or not modules.is_dir():
        raise ValueError('prepared Node dependencies are missing')
    if not link.is_symlink() or link.resolve(strict=True) != modules.resolve(strict=True):
        raise ValueError('workspace does not use the prepared Node dependency tree')
    package_lock = json.loads((root / 'tools/websmoke/package-lock.json').read_bytes())
    expected = {name: package_lock['packages']['node_modules/' + name]['version'] for name in PACKAGES}
    script = '''const names=['playwright-core','pixelmatch','pngjs'];
(async () => {
  const versions={}, paths={};
  for (const name of names) {
    versions[name]=require(name+'/package.json').version;
    paths[name]=require.resolve(name);
    await import(name);
  }
  console.log(JSON.stringify({node:process.version,versions,paths}));
})().catch(error => { console.error(error); process.exitCode=1; });'''
    result = subprocess.run(['node', '-e', script], cwd=root / 'tools/websmoke',
                            stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True, timeout=30)
    if result.returncode:
        raise ValueError('prepared Node dependency import failed: ' + result.stderr.strip()[:500])
    resolved = json.loads(result.stdout)
    if resolved.get('node') != lock['tools']['node'] or resolved.get('versions') != expected:
        raise ValueError('Node version or prepared package versions differ from the lock')
    base = modules.resolve(strict=True)
    for name in PACKAGES:
        path = Path(resolved['paths'][name]).resolve(strict=True)
        if base not in path.parents:
            raise ValueError('Node resolved a package outside the prepared dependency tree: ' + name)
    return {'status': 'PASS', 'node': resolved['node'], 'packages': expected,
            'note': 'prepared Node dependency verification only; no installation was performed'}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--root', type=Path, required=True)
    args = parser.parse_args()
    try:
        print(json.dumps(verify(args.root), indent=2))
    except (OSError, ValueError, KeyError, TypeError, subprocess.SubprocessError) as error:
        print('prepared Node dependency verification failed: ' + str(error), file=sys.stderr)
        return 1
    return 0


if __name__ == '__main__':
    raise SystemExit(main())
