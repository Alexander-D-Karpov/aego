#!/usr/bin/env python3
"""Release-local budgets. Existing Demo/Playground payload regression is checked
by the repository make ci gate, not inferred from the standalone file size."""
import argparse, json
from pathlib import Path
p = argparse.ArgumentParser()
p.add_argument('--artifact', required=True, type=Path)
a = p.parse_args()
m = json.loads((a.artifact / 'manifest.json').read_text())
w = m['wasm']
files = {v['path']: v for v in m['files']}
if not w['bytes'] <= 8 * 1024 * 1024:
    raise RuntimeError('WASM > 8 MiB')
if not files[w['path'] + '.gz']['bytes'] <= 3 * 1024 * 1024:
    raise RuntimeError('WASM gzip > 3 MiB')
allowed = {'aego/art', 'aego/art/internal/aseprite', 'aego/tools/artstudio/host', 'aego/tools/artstudio/wasm'}
for dep in (a.artifact / 'dependency-audit.txt').read_text().splitlines():
    if not (not dep.startswith('aego/') or dep in allowed):
        raise RuntimeError(f'unexpected engine dependency: {dep}')
preview = m.get('previewWasm')
if not preview or preview['bytes'] > 32 * 1024 * 1024 or files[preview['path'] + '.gz']['bytes'] > 10 * 1024 * 1024:
    raise RuntimeError('Missing or oversized lazy GPU preview')
for dep in (a.artifact / 'preview-dependency-audit.txt').read_text().splitlines():
    if dep.startswith(('aego/app/', 'aego/editor/', 'aego/project/', 'aego/asset/pipeline', 'aego/asset/importers/')):
        raise RuntimeError(f'unexpected desktop dependency in GPU preview: {dep}')
print(json.dumps({'standaloneBudget': 'PASS', 'wasmBytes': w['bytes'], 'gzipBytes': files[w['path'] + '.gz']['bytes'], 'previewWasmBytes': preview['bytes'], 'previewGzipBytes': files[preview['path'] + '.gz']['bytes'], 'previousDemoPlaygroundBudget': 'Required in separate full make ci gate; not inferred here.'}))
