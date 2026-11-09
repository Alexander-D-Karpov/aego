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
names = ['core', 'native-build', 'native-user-scenario', 'file-interoperability', 'remote-examples', 'wasm-parity', 'chromium', 'firefox', 'chromium-gpu', 'firefox-gpu', 'docs', 'component-try', 'budgets', 'regression', 'source-check', 'http-bytes']
gates = []
for name in names:
    code = int((a.evidence / (name + '.exit')).read_text())
    if not code == 0:
        raise RuntimeError(f'{name} failed')
    log = (a.evidence / (name + '.log')).read_bytes()
    if not log:
        raise RuntimeError(f'{name} produced no evidence')
    gates.append({'name': name, 'exitCode': code, 'logSHA256': hashlib.sha256(log).hexdigest()})
remote = json.loads((a.evidence / 'remote-protocol' / 'linux-protocol-result.json').read_text())
if not (remote.get('result') == 'PASS' and remote.get('isolatedXDG') is True
        and remote.get('projectStarted') is False and remote.get('audio') == 'dummy'
        and remote.get('registeredHandler') == 'aego-example.desktop'
        and len(remote.get('argv', [])) == 3 and remote['argv'][:2] == ['edit', '--example-link']
        and remote['argv'][2].startswith('aego://example?')):
    raise RuntimeError('Required isolated Linux protocol validation failed')
interop = json.loads((a.evidence / 'file-interop' / 'result.json').read_text())
if not (interop['status'] == 'PASS'
        and interop['manifestSHA256'] == hashlib.sha256(raw).hexdigest()
        and interop['sourceSHA256'] == m['sourceSHA256']
        and len(interop['browsers']) == 2
        and {r['browser'] for r in interop['browsers']} == {'chromium', 'firefox'}
        and all(r['status'] == 'PASS' and r.get('nativeReturn') == 'PASS'
                and r['inputSHA256'] == interop['nativeSourceSHA256']
                for r in interop['browsers'])):
    raise RuntimeError('Required native/browser file interoperability validation failed')
for browser in ['chromium', 'firefox']:
    report = json.loads((a.evidence / (browser + '-browser') / 'browser.json').read_text())
    if not (report['status'] == 'PASS' and report['transport'] == 'real-http'):
        raise RuntimeError('Required validation failed')
    if not report['manifestSHA256'] == hashlib.sha256(raw).hexdigest():
        raise RuntimeError('Required validation failed')
    gpu = json.loads((a.evidence / (browser + '-gpu-browser') / 'gpu.json').read_text())
    if not (gpu['status'] == 'PASS' and gpu['transport'] == 'real-http-webgl2'
            and gpu['manifestSHA256'] == hashlib.sha256(raw).hexdigest()
            and gpu['sourceSHA256'] == m['sourceSHA256']):
        raise RuntimeError('Required WebGL preview validation failed')
report = {'schema': 1, 'buildID': m['buildID'], 'commit': m['commit'], 'sourceSHA256': m['sourceSHA256'], 'manifestSHA256': hashlib.sha256(raw).hexdigest(), 'gates': gates}
with (a.evidence / 'gate.json').open('x') as f:
    json.dump(report, f, indent=2)
    f.write('\n')
print('gate.json written for', m['buildID'])
