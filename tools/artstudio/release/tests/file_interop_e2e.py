#!/usr/bin/env python3
"""Literal SDL-saved file -> browser file input/paint/download -> SDL reopen.

Run after stage125-check.sh artstudio-native, against the same served, verified artifact.
The native producer is the existing authoring/Game View workflow. Browser
state and hashes are read-only observations; edits use the actual controls.
Each return project retains the producer's exact .meta, scene and AssetRefs.
"""
from __future__ import annotations

import argparse
from contextlib import ExitStack
import hashlib
import json
import os
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile
import time
import traceback
import urllib.error
import urllib.parse
import urllib.request

from browser_context import browser_context

ROOT = Path(__file__).resolve().parents[4]
NATIVE_TEST = 'TestArtSceneAuthoringWorkflowNative'


def digest(data):
    return hashlib.sha256(data).hexdigest()


def read_state(page):
    # aegoArt.hash(frameID, channel) is the shared Go Document.Render hash,
    # including every frame even when another frame is currently selected.
    return page.evaluate('''async () => {
        const result=JSON.parse(aegoArt.request('{"action":"state"}'));
        if(result.error)throw new Error(result.error);
        const document=result.state.document, bytes=AegoStudioHost.capture().bytes;
        return {document,sourceSHA256:await AegoStudioProtocol.sha256(bytes),
            frameHashes:Object.fromEntries(document.frames.map(frame=>[frame.id,
                Object.fromEntries(['albedo','normal','height','emission'].map(channel=>
                    [channel,aegoArt.hash(frame.id,channel)]))]))};
    }''')


def topology(document):
    return {key:value for key,value in document.items() if key != 'revision'}


def checked(command, log, env=None):
    with log.open('x') as output:
        result = subprocess.run(command, cwd=ROOT, env=env, stdout=output, stderr=subprocess.STDOUT)
    log.with_suffix(log.suffix+'.exit').write_text(str(result.returncode)+'\n')
    if result.returncode:
        raise RuntimeError(f'Command exited {result.returncode}: {command}; log={log}')


def main():
    if not __debug__:
        raise RuntimeError('File interoperability assertions require normal Python (no -O)')
    parser=argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--artifact',type=Path,required=True)
    parser.add_argument('--app-url',required=True)
    parser.add_argument('--native-evidence',type=Path,required=True)
    parser.add_argument('--evidence',type=Path,required=True)
    parser.add_argument('--chromium-executable',default='')
    parser.add_argument('--firefox-executable',default='')
    parser.add_argument('--allow-unborn',action='store_true')
    parser.add_argument('--serve-artifact',action='store_true',help='Start an owned local artifact server; --app-url must use an explicit loopback address and unprivileged port')
    args=parser.parse_args()
    artifact=args.artifact.resolve(strict=True)
    native=args.native_evidence.resolve(strict=True)
    args.evidence.mkdir(parents=True,exist_ok=False)
    out=args.evidence.resolve(strict=True)
    raw_manifest=(artifact/'manifest.json').read_bytes()
    manifest=json.loads(raw_manifest)
    app=args.app_url.rstrip('/')+'/'
    report={'schema':1,'status':'FAIL','buildID':manifest['buildID'],
            'manifestSHA256':digest(raw_manifest),'sourceSHA256':manifest['sourceSHA256'],
            'browsers':[],'limits':['Actual Linux SDL/GL33 and browser file input/download; OS file picker and other native platforms are separate gates.']}
    server=None
    server_log=None
    server_tool=None
    snapshot=[sys.executable,str(ROOT/'tools/ci/component_try_evidence.py'),'snapshot','--root',str(ROOT)]
    if args.allow_unborn:
        snapshot.append('--allow-unborn')
    try:
        checked(snapshot+['--output',str(out/'source-before.json')],out/'source-before.log')
        before=json.loads((out/'source-before.json').read_text())
        native_summary=json.loads((native/'summary.json').read_text())
        assert native_summary['passed'] and native_summary['required'][NATIVE_TEST]=='PASS'
        assert native_summary['sourceFingerprint']==before['worktreeSHA256'], 'Native producer was tested on another source tree'
        report['sourceFingerprint']=before['worktreeSHA256']
        report['nativeSummarySHA256']=digest((native/'summary.json').read_bytes())
        release_command=['go','run','./tools/artstudio/release']
        if args.serve_artifact:
            address=urllib.parse.urlsplit(app)
            assert address.scheme=='http' and address.hostname in ('127.0.0.1','::1') and address.port and 1024<=address.port<=65535, 'Owned server requires an explicit HTTP loopback URL and unprivileged port'
            assert not address.username and not address.password and address.path=='/' and not address.query and not address.fragment, 'Owned server URL must identify only its loopback origin'
            binary_root=ROOT/'.cache/bin'
            binary_root.mkdir(parents=True,exist_ok=True)
            server_tool=tempfile.TemporaryDirectory(prefix='art-file-interop.',dir=binary_root)
            binary=Path(server_tool.name)/'artstudio-release'
            build=['go','build','-o',str(binary),'./tools/artstudio/release']
            (out/'app-server-build-command.json').write_text(json.dumps(build)+'\n')
            checked(build,out/'app-server-build.log')
            release_command=[str(binary)]
            report['serverToolSHA256']=digest(binary.read_bytes())
        checked(release_command+['check-source','-root',str(ROOT),'-dir',str(artifact)],out/'artifact-source.log')
        if args.serve_artifact:
            bind=f'[{address.hostname}]:{address.port}' if address.hostname=='::1' else f'{address.hostname}:{address.port}'
            command=release_command+['serve','-dir',str(artifact),'-addr',bind,'-docs-addr=']
            (out/'app-server-command.json').write_text(json.dumps(command)+'\n')
            server_log=(out/'app-server.log').open('x')
            server=subprocess.Popen(command,cwd=ROOT,stdout=server_log,stderr=subprocess.STDOUT)
            report['ownedServer']={'pid':server.pid,'origin':app.rstrip('/'),'stopRequested':False}
            deadline=time.monotonic()+30
            opener=urllib.request.build_opener(urllib.request.ProxyHandler({}))
            while True:
                if server.poll() is not None:
                    raise RuntimeError(f'Owned artifact server exited before readiness: {server.returncode}')
                try:
                    with opener.open(app+'manifest.json',timeout=2) as response:
                        served=response.read()
                    if served!=raw_manifest:
                        raise RuntimeError('Loopback port serves a different manifest')
                    break
                except (ConnectionError,TimeoutError,urllib.error.URLError) as error:
                    if time.monotonic()>=deadline:
                        raise TimeoutError('Owned artifact server was not ready within 30s') from error
                    time.sleep(.05)
            report['ownedServer']['readyManifestSHA256']=digest(served)
        with urllib.request.urlopen(app+'manifest.json',timeout=30) as response:
            assert response.read()==raw_manifest, 'HTTP server is serving another artifact'
        producer=native/'interoperability'
        producer_raw=(producer/'native-source.json').read_bytes()
        witness=json.loads(producer_raw)
        assert witness['schema']==1 and len(witness['state']['document']['frames'])==8
        assert witness['sourceAsset'] and witness['sceneRefs']['SpriteRenderer.sprite']==witness['sourceAsset']
        assert witness['sceneRefs']['AnimationPlayer.clip'] and len(witness['clipFrames'])==8
        for relative,expected in witness['projectFiles'].items():
            path=Path(relative)
            assert not path.is_absolute() and '..' not in path.parts and (producer/'project'/path).is_file()
            assert digest((producer/'project'/path).read_bytes())==expected, relative
        source=producer/'project'/witness['sourcePath']
        assert digest(source.read_bytes())==witness['state']['sourceSHA256']
        report['nativeSourceSHA256']=witness['state']['sourceSHA256']
        report['nativeWitnessSHA256']=digest(producer_raw)

        from playwright.sync_api import sync_playwright
        with sync_playwright() as playwright, ExitStack() as browsers:
            for name in ('chromium','firefox'):
                directory=out/name
                directory.mkdir()
                (directory/'native-source.json').write_bytes(producer_raw)
                browser_report={'schema':1,'status':'FAIL','browser':name,
                                'inputSHA256':witness['state']['sourceSHA256'],
                                'buildID':manifest['buildID'],'manifestSHA256':report['manifestSHA256'],
                                'sourceFingerprint':report['sourceFingerprint'],'consoleErrors':[]}
                report['browsers'].append(browser_report)
                options={'headless':True}
                executable=getattr(args,name+'_executable')
                if executable:
                    options['executable_path']=executable
                if name=='chromium':
                    options['args']=['--mute-audio','--no-sandbox','--disable-dev-shm-usage']
                else:
                    options['firefox_user_prefs']={'media.volume_scale':'0.0'}
                    if os.environ.get('MOZ_DISABLE_CONTENT_SANDBOX')=='1':
                        options['firefox_user_prefs']['security.sandbox.content.level']=0
                context,browser_report['browserVersion']=browsers.enter_context(browser_context(
                    playwright,name,options,{'viewport':{'width':1440,'height':1000},'accept_downloads':True},
                    urls=[app]))
                try:
                    page=context.new_page()
                    page.on('pageerror',lambda error:browser_report['consoleErrors'].append(str(error)))
                    page.on('console',lambda message:browser_report['consoleErrors'].append(message.text) if message.type=='error' else None)
                    responses=[]
                    page.on('response',lambda response:responses.append(response) if urllib.parse.urlparse(response.url).path.endswith('/'+manifest['wasm']['path']) else None)
                    page.goto(app+'editor/?lang=en',wait_until='load')
                    page.wait_for_function('() => window.artStudioSurfaceReady === true',timeout=60000)
                    assert len(responses)==1 and responses[0].status==200 and digest(responses[0].body())==manifest['wasm']['sha256'], 'Executed WASM differs from artifact'
                    browser_report['wasmSHA256']=manifest['wasm']['sha256']
                    handle=page.evaluate('() => AegoStudioHost.summary().handle')
                    page.locator('#file-input').set_input_files(str(source))
                    page.wait_for_function('(handle) => AegoStudioHost.summary().handle !== handle',arg=handle,timeout=30000)
                    loaded=read_state(page)
                    assert loaded==witness['state'], 'Native saved file changed bytes, topology, palette, pivots, durations or channel pixels on browser import'
                    page.screenshot(path=str(directory/'browser-opened-native.png'))
                    shared={}
                    for cel in loaded['document']['cels']:
                        if cel['channel']=='albedo':
                            shared.setdefault(cel['image'],[]).append(cel)
                    linked=next((cels for cels in shared.values() if len(cels)>1),None)
                    assert linked and len(linked)==2, 'Native producer did not retain exactly one linked pair'
                    target=linked[-1]
                    page.locator(f'#timeline-table tbody button[data-cel="{target["id"]}"]').click()
                    page.locator('#hex-color').fill('#26c7a3' if name=='chromium' else '#db4e83')
                    page.locator('#hex-color').press('Tab')
                    page.locator('[data-tool=pencil]').click()
                    page.locator('#fit').click()
                    box=page.locator('#canvas').bounding_box()
                    canvas=loaded['document']['canvas']
                    point=lambda x,y:(box['x']+(x+.5)*box['width']/canvas['width'],box['y']+(y+.5)*box['height']/canvas['height'])
                    page.mouse.move(*point(4,4));page.mouse.down()
                    page.mouse.move(*point(6,4),steps=2);page.mouse.up()
                    page.wait_for_function('() => !JSON.parse(aegoArt.request(\'{"action":"state"}\')).state.gesture')
                    painted=read_state(page)
                    assert topology(painted['document'])==topology(loaded['document']), 'Painting changed document structure'
                    changed={frame for frame in painted['frameHashes'] if painted['frameHashes'][frame]!=loaded['frameHashes'][frame]}
                    assert changed=={cel['frame'] for cel in linked}, 'Stroke did not update exactly the linked cels'
                    for frame in painted['frameHashes']:
                        for channel in ('normal','height','emission'):
                            assert painted['frameHashes'][frame][channel]==loaded['frameHashes'][frame][channel]
                    assert page.evaluate('() => JSON.parse(aegoArt.request(\'{"action":"state"}\')).state.undo')==1
                    page.locator('#undo').click()
                    assert read_state(page)['frameHashes']==loaded['frameHashes'], 'Web Undo lost native pixels'
                    page.locator('#redo').click()
                    returned=read_state(page)
                    assert returned['frameHashes']==painted['frameHashes'] and topology(returned['document'])==topology(loaded['document'])
                    with page.expect_download() as download:
                        page.locator('#studio-download-source').click()
                    downloaded=directory/'browser-return.aeart'
                    download.value.save_as(downloaded)
                    assert digest(downloaded.read_bytes())==returned['sourceSHA256'], 'Browser download is not the exact edited source'
                    page.screenshot(path=str(directory/'browser-edited-native.png'))
                    assert not browser_report['consoleErrors'], browser_report['consoleErrors']
                    browser_report.update({'status':'PASS','state':returned,'downloadSHA256':digest(downloaded.read_bytes()),
                                           'changedFrames':sorted(changed),'oneGestureUndoRedo':True})
                finally:
                    browsers.close()
                    (directory/'browser.json').write_text(json.dumps(browser_report,indent=2)+'\n')
                # Only the normal local-file return is installed in a disposable
                # copy. Original .meta and scene bytes are copied verbatim.
                shutil.copytree(producer/'project',directory/'project')
                (directory/'project'/witness['sourcePath']).write_bytes((directory/'browser-return.aeart').read_bytes())
                environment=os.environ.copy()
                environment['AEGO_ART_INTEROP_RETURN']=str(directory)
                environment.setdefault('SDL_VIDEODRIVER','offscreen')
                environment.setdefault('SDL_AUDIO_DRIVER','dummy')
                command=['go','test','-p','1','-json','-count=1','-tags','aego_gltest','-run','^'+NATIVE_TEST+'$','./app/edit']
                (directory/'native-command.json').write_text(json.dumps(command)+'\n')
                checked(command,directory/'native-events.jsonl',environment)
                events=[]
                for line in (directory/'native-events.jsonl').read_text().splitlines():
                    try:
                        events.append(json.loads(line))
                    except json.JSONDecodeError:
                        pass
                assert any(event.get('Test')==NATIVE_TEST and event.get('Action')=='pass' for event in events)
                assert not any(event.get('Action') in ('skip','fail') for event in events)
                returned_native=json.loads((directory/'native-return.json').read_text())
                assert returned_native['status']=='PASS' and returned_native['commonUndo']
                assert returned_native['witness']['state']==browser_report['state']
                browser_report['nativeReturn']='PASS'
                browser_report['nativeReturnSHA256']=digest((directory/'native-return.json').read_bytes())
                print(f'PASS {name}: SDL source -> real file input/linked paint/Undo/Redo/download -> SDL Assets reopen/common Undo; source/frame AssetRefs preserved',flush=True)
        checked(snapshot+['--output',str(out/'source-after.json')],out/'source-after.log')
        after=json.loads((out/'source-after.json').read_text())
        assert before['worktreeSHA256']==after['worktreeSHA256'] and before['files']==after['files'], 'Source changed during interoperability gate'
        report['status']='PASS'
    except Exception as error:
        report['error']=f'{type(error).__name__}: {error}'
        report['traceback']=traceback.format_exc()
        print(report['error'],file=sys.stderr)
    finally:
        if server is not None:
            was_running=server.poll() is None
            report['ownedServer']['stopRequested']=was_running
            try:
                if was_running:
                    server.terminate()
                    try:
                        server.wait(timeout=10)
                    except subprocess.TimeoutExpired:
                        # Stop only this Popen child; never use broad process
                        # names or unrelated PIDs from another namespace.
                        report['ownedServer']['forcedStop']=True
                        server.kill()
                        server.wait(timeout=10)
            except (OSError,subprocess.TimeoutExpired) as error:
                report['status']='FAIL'
                report['ownedServer']['cleanupError']=f'{type(error).__name__}: {error}'
            report['ownedServer']['exitCode']=server.returncode
            report['ownedServer']['closed']=server.poll() is not None
            (out/'app-server.exit').write_text(('UNVERIFIED' if server.returncode is None else str(server.returncode))+'\n')
            if not was_running and report['status']=='PASS':
                report['status']='FAIL'
                report['error']='Owned server exited unexpectedly before cleanup'
        if server_log is not None:
            server_log.close()
        if server_tool is not None and (server is None or server.poll() is not None):
            server_tool.cleanup()
        (out/'result.json').write_text(json.dumps(report,indent=2)+'\n')
    return 0 if report['status']=='PASS' else 1


if __name__=='__main__':
    raise SystemExit(main())
