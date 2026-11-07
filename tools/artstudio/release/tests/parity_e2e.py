#!/usr/bin/env python3
"""Compare each replay prefix on native Go and the served Go/WASM artifact.

Requires ARTSTUDIO_ARTIFACT, ARTSTUDIO_APP_URL and ARTSTUDIO_EVIDENCE. The
runner checks source and served manifest, then compares all frame pixels and
document topology after every operation in both real browsers. It exercises
the replay/API boundary, not the native UI or an OS save handle.
"""
from __future__ import annotations

import copy
import hashlib
import json
import os
from pathlib import Path
import subprocess
import sys
import traceback
import urllib.parse
import urllib.request

ROOT = Path(__file__).resolve().parents[4]


def main() -> int:
    if not __debug__:
        raise RuntimeError('Parity checks must not run with Python optimizations')
    artifact = Path(os.environ['ARTSTUDIO_ARTIFACT']).resolve(strict=True)
    app = os.environ['ARTSTUDIO_APP_URL'].rstrip('/') + '/'
    evidence = Path(os.environ['ARTSTUDIO_EVIDENCE']) / 'parity-replay'
    evidence.mkdir(parents=True, exist_ok=False)
    raw_manifest = (artifact / 'manifest.json').read_bytes()
    manifest = json.loads(raw_manifest)
    fixture_path = ROOT / 'tools/artstudio/tests/parity.json'
    fixture = json.loads(fixture_path.read_text())
    if not fixture.get('operations'):
        raise RuntimeError('Empty replay is not a parity test')
    result = {'status':'FAIL','buildID':manifest['buildID'],
              'manifestSHA256':hashlib.sha256(raw_manifest).hexdigest(),
              'sourceSHA256':manifest['sourceSHA256'],
              'fixtureSHA256':hashlib.sha256(fixture_path.read_bytes()).hexdigest(),
              'browsers':[], 'limits':['Native replay uses the shared Go core, not native UI input or rendering.',
                                      'OS save and physical touch/pen are separate gates.']}
    try:
        subprocess.run(['go','run','./tools/artstudio/release','check-source','-root',str(ROOT),'-dir',str(artifact)],cwd=ROOT,check=True)
        with urllib.request.urlopen(app + 'manifest.json', timeout=30) as response:
            if response.read() != raw_manifest:
                raise RuntimeError('Served manifest differs from artifact')
        native = evidence/'aego-artstudio-replay'
        subprocess.run(['go','build','-o',str(native),'./tools/artstudio'],cwd=ROOT,check=True)
        result['nativeHostSHA256'] = hashlib.sha256(native.read_bytes()).hexdigest()
        result['goVersion'] = subprocess.check_output(['go','version'],cwd=ROOT,text=True).strip()
        native_steps = []
        for count in range(len(fixture['operations'])+1):
            prefix = copy.deepcopy(fixture)
            prefix['operations'] = prefix['operations'][:count]
            prefix.pop('frame',None)
            path = evidence/f'prefix-{count:02}.json'
            path.write_text(json.dumps(prefix,ensure_ascii=False,indent=2)+'\n')
            data = json.loads(subprocess.check_output([str(native),'-replay',str(path)],cwd=ROOT,text=True))
            state = data['state']
            frames = {}
            for frame in state['document']['frames']:
                prefix['frame'] = frame['id']
                path.write_text(json.dumps(prefix,ensure_ascii=False,indent=2)+'\n')
                rendered = json.loads(subprocess.check_output([str(native),'-replay',str(path)],cwd=ROOT,text=True))
                frames[frame['id']] = rendered['pixelSHA256']
            native_steps.append({'state':state,'frames':frames})
        result['nativeSteps'] = native_steps
        from playwright.sync_api import sync_playwright
        with sync_playwright() as playwright:
            for name in ('chromium','firefox'):
                browser_options = {'headless':True}
                executable = os.environ.get('ARTSTUDIO_CHROMIUM_EXECUTABLE','') if name == 'chromium' else os.environ.get('ARTSTUDIO_FIREFOX_EXECUTABLE','')
                if executable:
                    browser_options['executable_path'] = executable
                if name == 'chromium':
                    browser_options['args'] = ['--no-sandbox','--disable-dev-shm-usage']
                browser = getattr(playwright,name).launch(**browser_options)
                browser_result = {'name':name,'version':browser.version,'steps':[]}
                result['browsers'].append(browser_result)
                try:
                    page = browser.new_page(viewport={'width':1440,'height':1000})
                    errors = []
                    wasm_responses = []
                    page.on('pageerror',lambda error:errors.append(str(error)))
                    page.on('response',lambda response: wasm_responses.append(response) if urllib.parse.urlparse(response.url).path.endswith('/'+manifest['wasm']['path']) else None)
                    page.goto(app+'editor/?lang=en',wait_until='load')
                    page.wait_for_function('() => window.artStudioSurfaceReady === true',timeout=60000)
                    if len(wasm_responses)!=1 or wasm_responses[0].status!=200 or hashlib.sha256(wasm_responses[0].body()).hexdigest()!=manifest['wasm']['sha256']:
                        raise RuntimeError(f'{name}: executed WASM response does not match the pinned artifact')
                    browser_result['wasmSHA256'] = manifest['wasm']['sha256']

                    def request(payload):
                        value = page.evaluate('(request) => JSON.parse(aegoArt.request(JSON.stringify(request)))',payload)
                        if value.get('error'):
                            raise RuntimeError(f'{name}: operation failed: {value["error"]}')
                        return value

                    request({'action':'new','width':fixture['width'],'height':fixture['height'],'mode':fixture['mode'],'confirm':True})
                    for count, expected in enumerate(native_steps):
                        if count:
                            request({'action':'apply','operation':fixture['operations'][count-1]})
                        actual = request({'action':'state'})['state']
                        expected_state = copy.deepcopy(expected['state'])
                        actual_state = copy.deepcopy(actual)
                        # Document IDs are intentionally random; entity IDs and
                        # link topology are deterministic within each replay.
                        expected_state['document'].pop('id',None)
                        actual_state['document'].pop('id',None)
                        # Native CLI starts from a clean in-memory document;
                        # Web New explicitly marks its unsaved tab dirty. Both
                        # must be dirty after the first real replay operation.
                        native_dirty = expected_state.pop('dirty')
                        web_dirty = actual_state.pop('dirty')
                        if web_dirty is not True or native_dirty != (count > 0):
                            raise RuntimeError(f'{name} prefix {count}: unexpected native/web dirty state: {native_dirty}/{web_dirty}')
                        if actual_state != expected_state:
                            raise RuntimeError(f'{name} prefix {count}: native/WASM state differs\nnative={json.dumps(expected_state,sort_keys=True)}\nwasm={json.dumps(actual_state,sort_keys=True)}')
                        hashes = page.evaluate('''() => {
                            const state = JSON.parse(aegoArt.request('{"action":"state"}')).state;
                            return Object.fromEntries(state.document.frames.map(frame => [frame.id,aegoArt.hash(frame.id)]));
                        }''')
                        if hashes != expected['frames']:
                            raise RuntimeError(f'{name} prefix {count}: frame pixel mismatch: native={expected["frames"]} wasm={hashes}')
                        browser_result['steps'].append({'prefix':count,'frameHashes':hashes,'nativeDirty':native_dirty,'webDirty':web_dirty})
                        print(f'PASS {name} prefix={count} frames={len(hashes)} topology+pixels',flush=True)
                    if errors:
                        raise RuntimeError(f'{name} page errors: {errors}')
                finally:
                    browser.close()
        result['status']='PASS'
    except Exception as error:
        result['error']=f'{type(error).__name__}: {error}'
        result['traceback']=traceback.format_exc()
        print(result['error'],file=sys.stderr)
    finally:
        (evidence/'result.json').write_text(json.dumps(result,indent=2,ensure_ascii=False)+'\n')
    return 0 if result['status']=='PASS' else 1


if __name__ == '__main__':
    raise SystemExit(main())
