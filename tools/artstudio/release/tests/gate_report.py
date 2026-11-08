#!/usr/bin/env python3
"""Bind successful CI logs to exact artifact bytes. This report is an integrity
record, NOT a signature. Only trust it from the existing protected CI runner."""
import argparse, hashlib, json
from pathlib import Path
p = argparse.ArgumentParser()
p.add_argument('--artifact', required=True, type=Path)
p.add_argument('--evidence', required=True, type=Path)
a = p.parse_args()
raw = (a.artifact / 'manifest.json').read_bytes()
m = json.loads(raw)
if not m['release']:
    raise RuntimeError('A snapshot cannot produce production gate evidence')
names = ['core', 'native-build', 'native-user-scenario', 'wasm-parity', 'chromium', 'firefox', 'docs', 'component-try', 'budgets', 'regression', 'source-check', 'http-bytes']
gates = []
for name in names:
    code = int((a.evidence / (name + '.exit')).read_text())
    if not code == 0:
        raise RuntimeError(f'{name} failed')
    log = (a.evidence / (name + '.log')).read_bytes()
    if not log:
        raise RuntimeError(f'{name} produced no evidence')
    gates.append({'name': name, 'exitCode': code, 'logSHA256': hashlib.sha256(log).hexdigest()})
for browser in ['chromium', 'firefox']:
    report = json.loads((a.evidence / (browser + '-browser') / 'browser.json').read_text())
    if not (report['status'] == 'PASS' and report['transport'] == 'real-http'):
        raise RuntimeError('Required validation failed')
    if not report['manifestSHA256'] == hashlib.sha256(raw).hexdigest():
        raise RuntimeError('Required validation failed')
report = {'schema': 1, 'buildID': m['buildID'], 'commit': m['commit'], 'sourceSHA256': m['sourceSHA256'], 'manifestSHA256': hashlib.sha256(raw).hexdigest(), 'gates': gates}
with (a.evidence / 'gate.json').open('x') as f:
    json.dump(report, f, indent=2)
    f.write('\n')
print('gate.json written for', m['buildID'])
