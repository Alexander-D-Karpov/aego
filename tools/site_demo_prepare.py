"""Prepare the website-only platformer in a new directory, without editing its source."""

import os
from pathlib import Path
import shutil
import sys


GROUND3 = '  node "0e82ddf02905a5ee0aab0dfb48bc8039" name="Ground3"'
REPLACEMENTS = (
    ('position=[23, 4]', 'position=[21.5, 4]'),
    ('size=[10, 1]', 'size=[13, 1]'),
    ('points=[[-5, 1.5], [-5, -0.5], [5, -0.5], [5, 1.5]]',
     'points=[[-6.5, 1.5], [-6.5, -0.5], [6.5, -0.5], [6.5, 1.5]]'),
    ('polygon=[[-5, -0.5], [5, -0.5], [5, 0.5], [-5, 0.5]]',
     'polygon=[[-6.5, -0.5], [6.5, -0.5], [6.5, 0.5], [-6.5, 0.5]]'),
)


def close_checkpoint_gap(scene):
    """Extend Ground3 from x=18 to x=15, retaining its x=28 right end."""
    if scene.count(GROUND3) != 1:
        raise ValueError('the platformer Ground3 is missing or ambiguous')
    start = scene.index(GROUND3)
    end = scene.index('\n  node ', start + len(GROUND3))
    block = scene[start:end]
    for before, after in REPLACEMENTS:
        if block.count(before) != 1:
            raise ValueError('the platformer Ground3 has changed: ' + before)
        block = block.replace(before, after, 1)
    return scene[:start] + block + scene[end:]


def prepare(source, dest):
    source = Path(source).resolve(strict=True)
    dest = Path(dest).absolute()
    if dest.exists() or dest.is_symlink():
        raise ValueError('destination must not exist')
    dest_parent = dest.parent.resolve(strict=True)
    dest = dest_parent / dest.name
    if not source.is_dir() or source == dest or source in dest.parents or dest in source.parents:
        raise ValueError('source and destination must be separate directories')
    dest.mkdir()
    for parent, dirs, files in os.walk(source, followlinks=False):
        parent = Path(parent)
        rel = parent.relative_to(source)
        dirs[:] = [d for d in dirs if not (rel == Path('.') and d in {'.aego', '.git', 'exports'})]
        for name in dirs + files:
            if (parent / name).is_symlink():
                raise ValueError('project symlinks are not supported: ' + str(parent / name))
        for name in dirs:
            (dest / rel / name).mkdir()
        for name in files:
            src = parent / name
            if not src.is_file():
                raise ValueError('unsupported project entry: ' + str(src))
            shutil.copy2(src, dest / rel / name)
    scene = dest / 'scenes/main.aescene'
    before = scene.read_bytes()
    after = close_checkpoint_gap(before.decode('utf-8')).encode('utf-8')
    scene.write_bytes(after)


if __name__ == '__main__':
    if len(sys.argv) != 3:
        raise SystemExit('usage: site_demo_prepare.py <project> <new-destination>')
    prepare(sys.argv[1], sys.argv[2])
