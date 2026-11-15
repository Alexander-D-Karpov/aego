#!/usr/bin/env python3
"""Strict HTTP E2E on the built artifact. No set_content, route.fulfill or mock
WASM. A missing browser, policy block or failed assertion exits nonzero.
File download uses the browser's real download event; native OS pickers and
physical pen/touch devices are separate acceptance gates.
"""
from __future__ import annotations
import argparse
from contextlib import ExitStack
import hashlib
import json
import os
from pathlib import Path
import sys
import time
import traceback
import urllib.parse
import urllib.request

from browser_context import browser_context


def main() -> int:
    if not __debug__:
        raise SystemExit("Browser acceptance must not run with Python optimizations (-O/PYTHONOPTIMIZE)")
    ap = argparse.ArgumentParser()
    ap.add_argument('--artifact', required=True, type=Path)
    ap.add_argument('--app-url', required=True)
    ap.add_argument('--docs-url', required=True)
    ap.add_argument('--docs-prefix', default='docs', choices=['docs', 'asset-studio'])
    ap.add_argument('--browser', required=True, choices=['chromium', 'firefox'])
    ap.add_argument('--executable', default='')
    ap.add_argument('--evidence', required=True, type=Path)
    args = ap.parse_args()
    args.evidence.mkdir(parents=True, exist_ok=False)
    raw = (args.artifact / 'manifest.json').read_bytes()
    manifest = json.loads(raw)
    app = args.app_url.rstrip('/') + '/'
    docs = args.docs_url.rstrip('/') + '/'
    report = {'schema': 1, 'status': 'FAIL', 'browser': args.browser,
              'buildID': manifest['buildID'], 'sourceSHA256': manifest['sourceSHA256'],
              'manifestSHA256': hashlib.sha256(raw).hexdigest(), 'checks': [], 'tutorials': [], 'layouts': [],
              'transport': 'real-http', 'consoleErrors': [], 'httpErrors': [], 'limits': [
                  'OS save picker not tested; actual browser download/reimport is tested.',
                  'Physical touch/pen, WebGL previews and native integration are separate gates.',
                  'Blocking site data exercises unavailable recovery, not actual disk or quota exhaustion.']}
    def checked(name):
        report['checks'].append(name)
        print('PASS', name, flush=True)
    def summary(page):
        return page.evaluate('() => AegoStudioHost.summary()')
    def frame_hashes(page):
        # History may change the active frame. Compare every frame by stable
        # ID instead of accidentally comparing different selections.
        return page.evaluate('''() => {
            const state = JSON.parse(aegoArt.request('{"action":"state"}')).state;
            return state.document.frames.map(frame => [frame.id, aegoArt.hash(frame.id)]);
        }''')
    def document_snapshot(page):
        # Compare complete serialized source plus every frame/channel, palette,
        # cel identity/link and duration. Counts alone cannot prove a reset.
        return page.evaluate('''async () => {
            const result=JSON.parse(aegoArt.request('{"action":"state"}'));
            if(result.error)throw new Error(result.error);
            const document=result.state.document, snapshot=AegoStudioHost.capture();
            return {document,sourceSHA256:await AegoStudioProtocol.sha256(snapshot.bytes),
                frames:document.frames.map(frame=>[frame.id,
                    ['albedo','normal','height','emission'].map(channel=>[channel,aegoArt.hash(frame.id,channel)])])};
        }''')
    def assert_reachable(page, selector, samples):
        target=page.locator(selector)
        target.scroll_into_view_if_needed(timeout=10000)
        # Trial uses the browser's ordinary actionability/hit tests and local
        # scrolling. No force-click or DOM event substitutes for pointer input.
        enabled=target.is_enabled()
        if enabled:
            target.click(trial=True,timeout=10000)
        measured=target.evaluate('''element=>{
            const r=element.getBoundingClientRect(),doc=element.ownerDocument,w=doc.defaultView;
            const points=[[.1,.1],[.5,.5],[.9,.9]];
            return {rect:{x:r.x,y:r.y,width:r.width,height:r.height,right:r.right,bottom:r.bottom},
                viewport:{width:w.innerWidth,height:w.innerHeight,scrollX:w.scrollX},
                rootWidth:doc.documentElement.scrollWidth,bodyWidth:doc.body.scrollWidth,
                hits:points.map(([x,y])=>{const hit=doc.elementFromPoint(r.x+r.width*x,r.y+r.height*y);return !!hit&&(hit===element||element.contains(hit))})};
        }''')
        measured.update({'selector':selector,'enabled':enabled})
        samples.append(measured)
        rect,viewport=measured['rect'],measured['viewport']
        assert abs(viewport['scrollX']) < .5, (selector,'root scrolled horizontally',measured)
        assert measured['rootWidth'] <= viewport['width']+1 and measured['bodyWidth'] <= viewport['width']+1, (selector,'root overflow',measured)
        assert rect['width'] > 0 and rect['height'] > 0, (selector,'empty control',measured)
        assert rect['x'] >= -.5 and rect['y'] >= -.5 and rect['right'] <= viewport['width']+.5 and rect['bottom'] <= viewport['height']+.5, (selector,'control clipped by viewport',measured)
        assert all(measured['hits']), (selector,'control clipped/covered by a panel',measured)
    def narrow_authoring(page, width, surface):
        samples=[]
        for selector in ('#toggle-side','#theme','#language','#fullscreen','#studio-download-source','#new','#open','#save','#export-png','#export-sheet'):
            assert_reachable(page,selector,samples)
        if surface=='standalone':
            for selector in ('[data-workspace=art]','[data-workspace=animation]','[data-workspace=vfx]'):
                assert_reachable(page,selector,samples)
            page.locator('[data-workspace=animation]').click()
            assert 'workspace-animation' in page.locator('body').get_attribute('class')
            page.locator('[data-workspace=art]').click()
        # Localized controls must fit in both language configurations.
        original_language=page.locator('html').get_attribute('lang')
        page.locator('#language').click()
        assert page.locator('html').get_attribute('lang') != original_language
        assert_reachable(page,'#fullscreen',samples)
        if surface=='standalone':
            for selector in ('[data-workspace=art]','[data-workspace=animation]','[data-workspace=vfx]'):
                assert_reachable(page,selector,samples)
        page.locator('#language').click()
        assert page.locator('html').get_attribute('lang') == original_language
        page.locator('#toggle-side').click()
        assert 'open' in page.locator('#sidebar').get_attribute('class')
        for selector in ('#mode','#fg-color','#hex-color','#fg-alpha','#swap-colors','#add-layer','#layer-properties'):
            assert_reachable(page,selector,samples)
        color='#12c987' if width==390 else '#ab24ef'
        page.locator('#hex-color').fill(color)
        page.locator('#hex-color').press('Tab')
        assert page.locator('#fg-color').input_value()==color
        page.locator('#fg-alpha').fill('210')
        page.locator('#fg-alpha').press('Tab')
        assert page.locator('#fg-alpha').input_value()=='210'
        page.locator('#fg-alpha').fill('255')
        page.locator('#fg-alpha').press('Tab')
        page.locator('#toggle-side').click()
        assert 'open' not in page.locator('#sidebar').get_attribute('class').split()
        # On narrow screens Tool Options are a separately opened left dock.
        # Exercise its real toggle and hit-test the same brush control before
        # returning to the canvas; do not treat the hidden control as a skip.
        assert_reachable(page,'#toggle-tools',samples)
        page.locator('#toggle-tools').click()
        assert page.locator('#toggle-tools').get_attribute('aria-expanded') == 'true'
        assert_reachable(page,'#brush-size',samples)
        page.locator('#brush-size').fill('1')
        page.locator('#brush-size').press('Tab')
        page.locator('#toggle-tools').click()
        assert page.locator('#toggle-tools').get_attribute('aria-expanded') == 'false'
        for selector in ('#channel','#canvas-background','#grid','#zoom-out','#zoom-in','#fit',
                         '#play','#onion','#add-frame','#duplicate-frame','#link-frame','#unlink','#delete-frame',
                         '#frame-left','#frame-right','#duration','#tags','#add-tag','#pivot','#delete-tag'):
            assert_reachable(page,selector,samples)
        page.locator('#channel').select_option('albedo')
        page.locator('#fit').click()
        prior=frame_hashes(page)
        paint(page,10 if width==390 else 18,10)
        assert frame_hashes(page)!=prior, (surface,width,'narrow pointer stroke did not change pixels')
        page.locator('#undo').click()
        assert frame_hashes(page)==prior, (surface,width,'narrow undo lost pixels')
        page.locator('#redo').click()
        frames=summary(page)['frames']
        page.locator('#add-frame').click()
        assert summary(page)['frames']==frames+1
        page.locator('#undo').click()
        assert summary(page)['frames']==frames
        page.locator('#new').click()
        for selector in ('#new-dialog [name=width]','#new-dialog [name=height]','#new-dialog [value=create]','#new-dialog [value=cancel]'):
            assert_reachable(page,selector,samples)
        page.locator('#new-dialog [value=cancel]').click()
        report['layouts'].append({'surface':surface,'width':width,'samples':samples})
        checked(f'{surface}-{width}px-controls-hit-tested-local-scroll-and-real-authoring')
    def recovery_digests(page):
        # Observe real IndexedDB bytes without importing them into the active
        # document or replacing the storage adapter used by the application.
        return page.evaluate('''async () => Object.fromEntries(await Promise.all(
            (await recoveryStore.list()).map(async record => [record.key, {
                revision:record.revision,
                sha256:Array.from(new Uint8Array(await crypto.subtle.digest('SHA-256',record.bytes)))
                    .map(value=>value.toString(16).padStart(2,'0')).join('')
            }])
        ))''')
    def wait_async_condition(page, predicate, arg, description, timeout_ms=30000):
        # Playwright's wait_for_function treats a Promise as a truthy return
        # before its value resolves. Evaluate the real asynchronous result on
        # each poll; a resolved false must never acknowledge a storage commit.
        deadline = time.monotonic() + timeout_ms / 1000
        while True:
            result = page.evaluate(predicate, arg)
            if result is True:
                return
            if result is not False:
                raise AssertionError(f'{description}: expected a resolved boolean, got {result!r}')
            remaining = deadline - time.monotonic()
            if remaining <= 0:
                raise TimeoutError(f'{description}: condition remained false for {timeout_ms}ms')
            time.sleep(min(.05, remaining))
    def wait_recovery(page, key):
        page.wait_for_function('''() => !JSON.parse(aegoArt.request('{"action":"state"}')).state.gesture''')
        expected = {'key':key,'revision':summary(page)['revision']}
        wait_async_condition(page, '''async ({key,revision}) =>
            (await recoveryStore.list()).some(record=>record.key===key&&record.revision===revision)''',
            expected, f'Recovery commit for {key} revision {expected["revision"]}')
    def paint(page, x=3, y=3):
        page.locator('[data-tool="pencil"]').click()
        box = page.locator('#canvas').bounding_box()
        info = summary(page)
        sx, sy = box['width']/info['width'], box['height']/info['height']
        mouse = page.page.mouse if hasattr(page, 'page') else page.mouse
        mouse.move(box['x']+(x+.5)*sx, box['y']+(y+.5)*sy)
        mouse.down()
        mouse.move(box['x']+(x+4.5)*sx, box['y']+(y+1.5)*sy, steps=8)
        mouse.up()
    try:
        with urllib.request.urlopen(app+'manifest.json', timeout=20) as response:
            assert response.read() == raw, 'HTTP manifest differs from tested artifact'
        checked('served-manifest-matches-artifact')
        from playwright.sync_api import expect, sync_playwright
        with sync_playwright() as p, ExitStack() as browsers:
            options = {'headless': True}
            if args.executable:
                options['executable_path'] = args.executable
            if args.browser == 'chromium':
                options['args'] = ['--mute-audio', '--no-sandbox', '--disable-dev-shm-usage']
            else:
                options['firefox_user_prefs'] = {'media.volume_scale': '0.0'}
                if os.environ.get('MOZ_DISABLE_CONTENT_SANDBOX') == '1':
                    options['firefox_user_prefs']['security.sandbox.content.level'] = 0
            context, report['browserVersion'] = browsers.enter_context(browser_context(
                p, args.browser, options,
                {'viewport':{'width':1440,'height':1000}, 'accept_downloads':True, 'has_touch':True},
                urls=[app, docs]))
            # Observe every actual page, including the transferred editor and
            # each lesson, rather than only the two initially opened tabs.
            def observe_page(opened):
                opened.on('pageerror', lambda error: report['consoleErrors'].append(str(error)))
                opened.on('console', lambda message: report['consoleErrors'].append(
                    f'{message.text} ({message.location})') if message.type == 'error' else None)
                opened.on('response', lambda response: report['httpErrors'].append(
                    {'status':response.status, 'url':response.url}) if response.status >= 400 else None)
            context.on('page', observe_page)
            page = context.new_page()
            # Accept only the explicit reset and the dirty-document unload
            # confirmation exercised below; do not accept unrelated dialogs.
            page.on('dialog', lambda dialog: dialog.accept() if dialog.type in ('confirm', 'beforeunload') else dialog.dismiss())
            page.goto(app+'editor/?new=sprite&lang=en', wait_until='load')
            page.wait_for_function('() => window.artStudioSurfaceReady === true', timeout=60000)
            page.locator('#new-dialog [name=width]').fill('32')
            page.locator('#new-dialog [name=height]').fill('32')
            initial_handle = summary(page)['handle']
            page.locator('#new-dialog [value=create]').click()
            page.wait_for_function('(old) => AegoStudioHost.summary().handle !== old',arg=initial_handle)
            assert summary(page)['width'] == 32
            checked('create-32x32-via-ui')
            from authoring_ui import exercise_export_controls
            source_before_exports = document_snapshot(page)
            exercise_export_controls(page, checked)
            assert document_snapshot(page) == source_before_exports, 'export control presentation changed the editable source'
            # Theme and language are presentation preferences, never artwork
            # edits. Exercise both explicit themes, system-following mode and
            # persistence in another real same-origin page.
            source_before_theme = document_snapshot(page)
            page.locator('#theme').select_option('light')
            assert page.locator('html').get_attribute('data-theme') == 'light'
            light_background = page.evaluate('() => getComputedStyle(document.body).backgroundColor')
            primary_contrast = page.evaluate('''() => {
                const style=getComputedStyle(document.querySelector('#new-dialog .primary'));
                const luminance=value=>{
                    const rgb=value.match(/\\d+/g).slice(0,3).map(Number).map(v=>v/255)
                        .map(v=>v<=0.04045?v/12.92:Math.pow((v+0.055)/1.055,2.4));
                    return .2126*rgb[0]+.7152*rgb[1]+.0722*rgb[2];
                };
                const a=luminance(style.color), b=luminance(style.backgroundColor);
                return (Math.max(a,b)+.05)/(Math.min(a,b)+.05);
            }''')
            assert primary_contrast >= 4.5, ('light-theme primary control contrast', primary_contrast)
            page.screenshot(path=str(args.evidence/'theme-light-en.png'), full_page=True)
            page.locator('#theme').select_option('dark')
            assert page.locator('html').get_attribute('data-theme') == 'dark'
            dark_background = page.evaluate('() => getComputedStyle(document.body).backgroundColor')
            assert light_background != dark_background, 'Light/dark preferences did not change actual UI colors'
            page.screenshot(path=str(args.evidence/'theme-dark-en.png'), full_page=True)
            page.emulate_media(color_scheme='light')
            page.locator('#theme').select_option('auto')
            page.wait_for_function('() => document.documentElement.dataset.theme === "light"')
            page.emulate_media(color_scheme='dark')
            page.wait_for_function('() => document.documentElement.dataset.theme === "dark"')
            if page.locator('html').get_attribute('lang') != 'en':
                page.locator('#language').click()
            assert page.locator('#theme').get_attribute('aria-label') == 'Interface theme'
            channel_before = page.locator('#channel').input_value()
            blend_before = page.locator('#blend').input_value()
            assert page.locator('#channel option[value=normal]').text_content() == 'Normal'
            assert page.locator('#blend option[value=multiply]').text_content() == 'Multiply'
            assert page.locator('#tag-dialog [name=direction] option[value=reverse]').text_content() == 'Reverse'
            assert page.locator('#zoom-in').get_attribute('aria-label') == 'Zoom in'
            page.locator('#language').click()
            assert page.locator('#theme').get_attribute('aria-label') == 'Тема интерфейса'
            assert page.locator('#theme option[value=light]').inner_text() == 'Светлая'
            assert page.locator('[data-workspace=art]').text_content() == 'Рисование'
            assert page.locator('#channel option[value=normal]').text_content() == 'Нормали'
            assert page.locator('#blend option[value=multiply]').text_content() == 'Умножение'
            assert page.locator('#tag-dialog [name=direction] option[value=reverse]').text_content() == 'Назад'
            assert page.locator('#transform-dialog [name=kind] option[value=rotate-cw]').text_content() == 'Повернуть на 90°'
            assert page.locator('#zoom-in').get_attribute('aria-label') == 'Увеличить масштаб'
            assert page.locator('#channel').input_value() == channel_before and page.locator('#blend').input_value() == blend_before
            page.locator('#language').click()
            page.locator('#theme').select_option('light')
            peer_theme = context.new_page()
            peer_theme.goto(app+'editor/?lang=en', wait_until='load')
            peer_theme.wait_for_function('() => window.artStudioSurfaceReady === true', timeout=60000)
            assert peer_theme.locator('#theme').input_value() == 'light'
            assert peer_theme.locator('html').get_attribute('data-theme') == 'light'
            peer_theme.close()
            page.locator('#theme').select_option('auto')
            assert document_snapshot(page) == source_before_theme, 'theme/language changed the editable source'
            checked('light-dark-auto-theme-localized-persistent-without-artwork-changes')
            # The same Go-owned catalog backs the localized native/web chooser.
            # Actual creation, editing, download and real GPU draw of every ID
            # are exercised by the two existing GPU gates.
            catalog = page.evaluate('() => AegoStudioHost.effectTemplates()')
            assert len(catalog) == 8 and {entry['id'] for entry in catalog} == {entry['id'] for entry in manifest['templates']}
            for locale, field in (('en', 'nameEN'), ('ru', 'nameRU')):
                if page.locator('html').get_attribute('lang') != locale:
                    page.locator('#language').click()
                page.locator('#new').click()
                for entry in catalog:
                    option = page.locator('#new-dialog [name=template] option[value="'+entry['id']+'"]')
                    assert option.inner_text() == entry[field]
                page.locator('#new-dialog [value=cancel]').click()
            page.locator('#language').click()
            assert page.locator('html').get_attribute('lang') == 'en'
            checked('eight-shared-effect-template-names-in-en-and-ru')
            before = summary(page)['pixelHash']
            paint(page)
            after = summary(page)['pixelHash']
            assert before != after
            page.locator('#undo').click()
            assert summary(page)['pixelHash'] == before
            page.locator('#redo').click()
            assert summary(page)['pixelHash'] == after
            checked('pointer-stroke-undo-redo')
            page.locator('#add-layer').click()
            page.locator('#layer-dialog [name=name]').fill('Browser layer')
            page.locator('#layer-dialog [value=apply]').click()
            page.wait_for_function('() => AegoStudioHost.summary().layers === 2', timeout=30000)
            assert summary(page)['layers'] == 2
            paint(page, 10, 10)
            checked('layers-via-ui')
            from authoring_ui import exercise_authoring
            exercise_authoring(page, checked, args.browser, args.evidence)
            for _ in range(6):
                page.locator('#duplicate-frame').click()
            page.locator('#link-frame').click()
            assert summary(page)['frames'] == 8 and summary(page)['linkedImages'] > 0
            page.locator('#duration').fill('160')
            page.locator('#duration').press('Tab')
            page.locator('#onion').check()
            assert summary(page)['onion']
            page.locator('#play').click()
            page.wait_for_timeout(400)
            page.locator('#play').click()
            checked('eight-frames-linked-duration-onion-playback')
            pixel_hash = summary(page)['pixelHash']
            # The integration's download command uses the same Go source bytes;
            # no fake writer and no call to mark a download as saved.
            with page.expect_download() as event:
                page.locator('#studio-download-source').click()
            saved = args.evidence/'roundtrip.aeart'
            event.value.save_as(saved)
            assert saved.stat().st_size > 0
            old_handle = summary(page)['handle']
            saved_frames = frame_hashes(page)
            page.locator('#file-input').set_input_files(str(saved))
            page.wait_for_function('(old) => AegoStudioHost.summary().handle !== old',arg=old_handle)
            assert summary(page)['frames'] == 8 and summary(page)['pixelHash'] == pixel_hash
            assert frame_hashes(page) == saved_frames
            checked('real-download-and-file-input-roundtrip')
            with page.expect_download() as event:
                page.locator('#export-png').click()
            png = args.evidence/'export.png';event.value.save_as(png)
            assert png.read_bytes().startswith(b'\x89PNG\r\n\x1a\n')
            checked('real-png-download')
            # Dirty recovery uses real origin storage and a real reload.
            paint(page, 20, 20)
            recovered_hash = summary(page)['pixelHash']
            assert summary(page)['dirty'], 'Recovery must exercise an unsaved document'
            page.wait_for_function('() => AegoStudioHost.storageAvailable()')
            page.wait_for_timeout(1200)
            page.reload()
            page.wait_for_function('() => window.artStudioSurfaceReady === true')
            page.locator('#recovery').wait_for(state='visible')
            recovery_source = page.locator('#recovery-choice').input_value()
            original_recovery = recovery_digests(page)[recovery_source]
            page.locator('#restore').click()
            assert summary(page)['pixelHash'] == recovered_hash
            checked('indexeddb-dirty-recovery-after-reload')
            page.screenshot(path=str(args.evidence/'standalone.png'),full_page=True)
            # Two ordinary same-origin windows may both see the same recovery
            # snapshot. Restoring it must create separate writable copies, and
            # closing one document must not remove the other window's backup.
            peer = context.new_page()
            peer_dialogs = []
            accept_peer_confirm = {'value': True}
            def answer_peer_dialog(dialog):
                peer_dialogs.append({'type':dialog.type, 'message':dialog.message})
                if dialog.type == 'beforeunload' or (dialog.type == 'confirm' and accept_peer_confirm['value']):
                    dialog.accept()
                else:
                    dialog.dismiss()
            peer.on('dialog', answer_peer_dialog)
            peer.goto(app+'editor/?lang=en', wait_until='load')
            peer.wait_for_function('() => window.artStudioSurfaceReady === true')
            peer.locator('#recovery-choice').select_option(recovery_source)
            peer.locator('#restore').click()
            assert summary(peer)['pixelHash'] == recovered_hash
            page_key = page.evaluate('() => recoveryKeys.get(AegoStudioHost.summary().handle)')
            peer_key = peer.evaluate('() => recoveryKeys.get(AegoStudioHost.summary().handle)')
            assert len({page_key,peer_key,recovery_source}) == 3, 'Restored windows share an IndexedDB record'
            for target, color, position in ((page,'#12c987',(2,20)),(peer,'#ab24ef',(18,2))):
                target.bring_to_front()
                target.locator('#hex-color').fill(color)
                target.locator('#hex-color').press('Tab')
                paint(target,*position)
                assert summary(target)['pixelHash'] != recovered_hash
            assert summary(page)['pixelHash'] != summary(peer)['pixelHash']
            wait_recovery(page,page_key);wait_recovery(peer,peer_key)
            peer_hash = summary(peer)['pixelHash']
            peer_recovery = recovery_digests(peer)[peer_key]
            page_handle = summary(page)['handle']
            page.bring_to_front()
            page.locator(f'[data-close-document="{page_handle}"]').click()
            wait_async_condition(page, 'async key => !(await recoveryStore.list()).some(record=>record.key===key)',
                                 page_key, 'Closed-document recovery removal')
            records = recovery_digests(peer)
            assert records[peer_key] == peer_recovery, 'Closing a restored copy changed another window\'s recovery'
            assert records[recovery_source] == original_recovery, 'Restore or close changed the original recovery record'
            assert summary(peer)['pixelHash'] == peer_hash
            peer.reload()
            peer.wait_for_function('() => window.artStudioSurfaceReady === true')
            peer.locator('#recovery-choice').select_option(peer_key)
            peer.locator('#restore').click()
            assert summary(peer)['pixelHash'] == peer_hash
            peer.locator('#recovery-choice').select_option(peer_key)
            # Restore installs the document synchronously, then refreshes the
            # IndexedDB list asynchronously. is_enabled/is_disabled are only
            # instantaneous snapshots and can observe the busy guard instead
            # of the completed restore. Wait for the actual discard action to
            # become available before checking the restored-source guard.
            expect(peer.locator('#dismiss-recovery'), 'Original snapshot has no explicit discard action').to_be_enabled(timeout=30000)
            assert peer.locator('#recovery-choice').input_value() == peer_key
            assert peer.locator('#restore').is_disabled(), 'Same recovery source can be restored repeatedly in one page'
            peer.screenshot(path=str(args.evidence/'independent-recovery.png'),full_page=True)
            checked('same-origin-recovery-copies-remain-independent-through-close-and-reload')
            # Exercise the real explicit discard, including a rejected
            # confirmation, without losing the active copy or its history.
            restored_key = peer.evaluate('() => recoveryKeys.get(AegoStudioHost.summary().handle)')
            assert len({restored_key,peer_key,recovery_source}) == 3
            peer.locator('#hex-color').fill('#31cf72')
            peer.locator('#hex-color').press('Tab')
            paint(peer,3,8)
            edited_hash = summary(peer)['pixelHash']
            assert edited_hash != peer_hash and summary(peer)['dirty']
            wait_recovery(peer,restored_key)
            restored_recovery = recovery_digests(peer)[restored_key]
            accept_peer_confirm['value'] = False
            dialogs_before = len(peer_dialogs)
            peer.locator('#dismiss-recovery').click()
            assert len(peer_dialogs) == dialogs_before+1 and peer_dialogs[-1]['type'] == 'confirm'
            records = recovery_digests(peer)
            assert records[peer_key] == peer_recovery, 'Cancelling discard changed the selected backup'
            assert records[restored_key] == restored_recovery and records[recovery_source] == original_recovery
            accept_peer_confirm['value'] = True
            peer.locator('#dismiss-recovery').click()
            assert len(peer_dialogs) == dialogs_before+2 and peer_dialogs[-1]['type'] == 'confirm'
            wait_async_condition(peer, 'async key => !(await recoveryStore.list()).some(record=>record.key===key)',
                                 peer_key, 'Explicitly discarded recovery removal')
            records = recovery_digests(peer)
            assert records[restored_key] == restored_recovery and records[recovery_source] == original_recovery
            assert summary(peer)['pixelHash'] == edited_hash and summary(peer)['dirty']
            peer.locator('#undo').click()
            assert summary(peer)['pixelHash'] == peer_hash
            peer.locator('#redo').click()
            assert summary(peer)['pixelHash'] == edited_hash
            checked('explicit-recovery-discard-requires-confirmation-and-preserves-active-copy-and-history')
            peer.close()
            # CSP and cross-origin iframe are exercised by ordinary HTTP navigation.
            doc = context.new_page()
            doc.on('dialog', lambda dialog: dialog.accept() if dialog.type=='confirm' else dialog.dismiss())
            doc.goto(docs+f'{args.docs_prefix}/ru/first-sprite/')
            assert len(doc.frames)==1, 'WASM iframe must be lazy'
            doc.locator('[data-action=start]').click()
            doc.wait_for_function("() => !document.querySelector('[data-action=check]').disabled",timeout=60000)
            frame = next(f for f in doc.frames if '/embed/' in f.url)
            first_hash = summary(frame)['pixelHash']
            paint(frame)
            dirty = summary(frame)
            assert dirty['pixelHash'] != first_hash and dirty['dirty']
            frame.locator('#add-frame').click()
            frame.locator('#onion').check()
            dirty = summary(frame)
            dirty_frames = frame_hashes(frame)
            checked('real-docs-iframe-paint-frame-onion')
            with doc.expect_popup() as event:
                frame.locator('#studio-open-full').click()
            full = event.value
            full.wait_for_function('() => window.artStudioSurfaceReady === true',timeout=60000)
            full.wait_for_function("() => !location.search.includes('transfer=')",timeout=35000)
            transferred = summary(full)
            for key in ['width','height','frames','layers','cels','pixelHash']:
                assert transferred[key] == dirty[key], (key, transferred, dirty)
            assert frame_hashes(full) == dirty_frames, 'Transferred frame identities or pixels changed'
            assert transferred['dirty'] and summary(frame)['dirty'], 'Transfer must not mark either copy saved'
            checked('iframe-to-full-ack-preserves-dirty-pixels-layers-frames')
            # Adding the frame was a source edit. Transfer must leave that
            # document's own undo stack intact and the new copy independent.
            assert dirty['frames'] > 1
            frame.locator('#undo').click()
            assert summary(frame)['frames'] == dirty['frames'] - 1
            assert summary(full)['frames'] == transferred['frames']
            frame.locator('#redo').click()
            assert summary(frame)['frames'] == dirty['frames']
            assert frame_hashes(frame) == dirty_frames, 'Undo/redo changed source frame identities or pixels'
            assert frame_hashes(full) == dirty_frames, 'Source history changed the transferred copy'
            checked('iframe-source-undo-redo-after-transfer')
            doc.locator('[data-action=reset]').click()
            # Reset crosses the iframe message boundary; the click returns
            # before the editor has validated and installed the example.
            frame.wait_for_function('(hash) => AegoStudioHost.summary().pixelHash === hash', arg=first_hash, timeout=30000)
            assert summary(frame)['pixelHash'] == first_hash
            assert summary(full)['pixelHash'] == transferred['pixelHash']
            checked('reset-does-not-touch-transferred-document')
            # The transferred editor is a real popup and can leave the docs
            # page backgrounded. Bring it forward before capturing evidence,
            # just as the later narrow-layout captures do.
            doc.bring_to_front()
            doc.screenshot(path=str(args.evidence/'docs-transfer.png'), full_page=True)
            # Load each lesson in both languages, using the one shared artifact.
            for language in ('ru','en'):
                for item in manifest['examples']:
                    tutorial = context.new_page()
                    tutorial.on('dialog', lambda dialog: dialog.accept() if dialog.type=='confirm' else dialog.dismiss())
                    tutorial.goto(docs+f'{args.docs_prefix}/{language}/{item["id"]}/')
                    tutorial.locator('[data-action=start]').click()
                    tutorial.wait_for_function("() => !document.querySelector('[data-action=check]').disabled",timeout=60000)
                    f = next(f for f in tutorial.frames if '/embed/' in f.url)
                    assert summary(f)['buildID'] == manifest['buildID']
                    start = summary(f)
                    lesson = item['id']
                    starting_document=document_snapshot(f)
                    check_kind=tutorial.locator('.aego-studio-embed').get_attribute('data-check')
                    manual_lesson=lesson in ('aseprite','export')
                    assert (check_kind=='manual')==manual_lesson, (language,lesson,check_kind)
                    lesson_evidence={'language':language,'lesson':lesson,'checkKind':check_kind,'initial':starting_document}
                    def check_lesson(expected, phase):
                        tutorial.locator('[data-action=check]').click()
                        if expected=='manual':
                            text={'ru':'Это действие проверяется вручную; автоматический PASS не выставляется.',
                                  'en':'This action requires manual verification; no automatic PASS is recorded.'}[language]
                            tutorial.wait_for_function('(text)=>document.querySelector(".embed-status").textContent===text',arg=text)
                            assert tutorial.locator('.aego-studio-embed').get_attribute('data-check-result')!='observed'
                        else:
                            tutorial.wait_for_function('(expected)=>document.querySelector(".aego-studio-embed").dataset.checkResult===expected',arg=expected)
                        checked(f'tutorial-check-{language}-{lesson}-{phase}-{expected}')
                    check_lesson('manual' if manual_lesson else 'pending','initial')
                    if lesson == 'layers':
                        f.locator('#add-layer').click()
                        f.locator('#layer-dialog [name=name]').fill('Tutorial layer')
                        f.locator('#layer-dialog [value=apply]').click()
                        f.wait_for_function('(old) => AegoStudioHost.summary().layers === old+1',arg=start['layers'])
                        paint(f)
                        assert summary(f)['layers'] == start['layers'] + 1
                    elif lesson == 'animation':
                        f.locator('#add-frame').click()
                        f.locator('#duration').fill('175')
                        f.locator('#duration').press('Tab')
                        assert summary(f)['frames'] == start['frames'] + 1
                    elif lesson == 'linked-cels':
                        f.locator('#link-frame').click()
                        paint(f)
                        assert summary(f)['linkedImages'] > 0
                        # The documented condition is a shared cel. Observe it
                        # while the link exists, before testing independent Unlink.
                        check_lesson('observed','before-unlink')
                        f.locator('#unlink').click()
                        assert summary(f)['frames'] == start['frames'] + 1
                    elif lesson == 'onion-skin':
                        f.locator('#onion').check()
                        assert summary(f)['onion']
                    elif lesson == 'surface-channels':
                        f.locator('#channel').select_option('emission')
                        paint(f)
                        assert 'emission' in summary(f)['channels']
                    elif lesson == 'aseprite':
                        sample = args.artifact/'examples/aseprite/point4frames.aseprite'
                        assert sample.is_file(), 'Genuine import fixture must ship with its license'
                        f.locator('#file-input').set_input_files(str(sample))
                        f.wait_for_function('() => AegoStudioHost.summary().frames === 4')
                        assert summary(f)['layers'] == 2 and summary(f)['mode'] == 'indexed'
                    elif lesson == 'export':
                        with tutorial.expect_download() as source_download:
                            f.locator('#save').click()
                        editable = args.evidence/f'lesson-save-{language}.aeart'
                        source_download.value.save_as(editable)
                        assert editable.stat().st_size > 0
                        with tutorial.expect_download() as download:
                            f.locator('#studio-download-source').click()
                        copy = args.evidence/f'lesson-export-{language}.aeart'
                        download.value.save_as(copy)
                        old_handle = summary(f)['handle']
                        f.locator('#file-input').set_input_files(str(copy))
                        f.wait_for_function('(old) => AegoStudioHost.summary().handle !== old',arg=old_handle)
                        assert summary(f)['pixelHash'] == start['pixelHash']
                    else:
                        if lesson == 'pixel-perfect':
                            f.locator('#pixel-perfect').check()
                            f.locator('#brush-dither').fill('8')
                        f.locator('#swatches button').nth(2).click()
                        paint(f)
                        assert summary(f)['pixelHash'] != start['pixelHash'], (language,lesson)
                        if lesson in ('character','explosion'):
                            assert summary(f)['frames'] >= 8
                    if lesson!='linked-cels':
                        check_lesson('manual' if manual_lesson else 'observed','after-action')
                    lesson_evidence['condition']='manual' if manual_lesson else 'observed'
                    lesson_evidence['afterAction']=document_snapshot(f)
                    checked(f'tutorial-action-{language}-{lesson}')
                    before_reset_handle=summary(f)['handle']
                    tutorial.locator('[data-action=reset]').click()
                    f.wait_for_function('(old)=>AegoStudioHost.summary().handle!==old',arg=before_reset_handle,timeout=30000)
                    ready_text={'ru':'Редактор готов. Рисунок остаётся в браузере.',
                                'en':'Editor ready. Artwork stays in this browser.'}[language]
                    # This is the real docs-side acknowledgement of reset-example,
                    # not only a changed editor tab observed before the RPC reply.
                    tutorial.wait_for_function('(text)=>document.querySelector(".embed-status").textContent===text',arg=ready_text,timeout=30000)
                    reset_document=document_snapshot(f)
                    assert reset_document==starting_document, (language,lesson,'Reset changed source bytes, frame/channel pixels or document structure')
                    assert not summary(f)['dirty'], (language,lesson,'Reset source remains dirty')
                    tabs=f.evaluate('()=>JSON.parse(aegoArt.request(\'{"action":"state"}\')).tabs')
                    assert all(tab['key']!=start['handle'] for tab in tabs), (language,lesson,'Reset retained its original example tab')
                    lesson_evidence['reset']=reset_document
                    lesson_evidence['resetAcknowledged']=True
                    report['tutorials'].append(lesson_evidence)
                    checked(f'tutorial-reset-{language}-{lesson}-complete-source-and-frames')
                    tutorial.close()
            checked('twelve-lessons-ru-en-real-editor-and-actions')
            assert len(report['tutorials'])==24 and sum(item['condition']=='observed' for item in report['tutorials'])==20
            checked('twenty-observed-lesson-conditions-four-explicit-manual-and-twenty-four-resets')
            # Every manual chapter and locale switch is a real served page.
            manual = context.new_page()
            chapters = ['getting-started','pixel-art-tools','layers-palettes','timeline','linked-cels','onion-skin','import-export','aseprite-import','surface-channels','vfx-authoring','using-in-aego','web-standalone','recovery','keyboard-shortcuts','troubleshooting']
            for language in ('en','ru'):
                for chapter in chapters:
                    response = manual.goto(docs+f'{args.docs_prefix}/{language}/guide/{chapter}/')
                    assert response and response.status == 200
                    assert manual.locator('article p').count() >= 4
                    assert manual.locator('nav a').count() == len(chapters)
            manual.close()
            checked('fifteen-manual-chapters-ru-en')
            if args.docs_prefix == 'asset-studio':
                # Verify links from the real component registry pages, not a
                # generated placeholder or a second component Try runtime.
                component_lessons = {
                    'engine.SpriteRenderer': 'first-sprite',
                    'animation.AnimationPlayer': 'animation',
                    'animation.Animator': 'character',
                    'particles.ParticleSystem2D': 'explosion',
                    'engine.MaterialOverride': 'surface-channels',
                    'engine.Light2D': 'surface-channels',
                    'engine.LightOccluder2D': 'surface-channels',
                }
                reference = context.new_page()
                for language in ('ru', 'en'):
                    for component, lesson in component_lessons.items():
                        prefix = 'ru/' if language == 'ru' else ''
                        response = reference.goto(docs+f'{prefix}ref/{component}.html')
                        assert response and response.status == 200, (language, component)
                        assert reference.locator(f'a[href$="/asset-studio/{language}/{lesson}/"]').count() > 0, (language, component, lesson)
                        assert len(reference.frames) == 1, 'Component reference must not eagerly load Studio'
                    # Existing gendocs pager links omit .html. The docs server
                    # must resolve these real navigations as the site's nginx
                    # does, rather than masking 404s in a synthetic page.
                    reference.goto(docs+f'{prefix}ref/engine.SpriteRenderer.html')
                    href = reference.locator('.pager a.next').get_attribute('href')
                    assert href, (language, 'missing next-page link')
                    target = urllib.parse.urljoin(reference.url, href)
                    assert not urllib.parse.urlsplit(target).path.endswith('.html'), target
                    response = reference.goto(target, wait_until='load')
                    assert response and response.status == 200, (language, target, response.status if response else None)
                reference.close()
                checked('real-component-docs-link-localized-lazy-lessons')
                checked('real-component-docs-extensionless-navigation')
            else:
                report['limits'].append('Standalone preview does not contain the real generated component reference pages.')
            # Firefox can stop producing frames for a background tab while the
            # 24 lesson pages are exercised; the final capture must render it.
            for width in (390,320):
                full.bring_to_front()
                full.set_viewport_size({'width':width,'height':844})
                # Retain the old visible assertion, then prove controls really
                # receive input without scrolling the entire editor off-screen.
                assert full.locator('#studio-download-source').is_visible()
                narrow_authoring(full,width,'standalone')
                full.screenshot(path=str(args.evidence/('narrow.png' if width==390 else 'narrow-320.png')),timeout=30000)
                with full.expect_download() as narrow_download:
                    full.locator('#studio-download-source').click()
                downloaded=args.evidence/f'narrow-{width}.aeart'
                narrow_download.value.save_as(downloaded)
                assert hashlib.sha256(downloaded.read_bytes()).hexdigest()==document_snapshot(full)['sourceSHA256']
                checked(f'standalone-{width}px-real-download-matches-current-source')
                doc.bring_to_front()
                doc.set_viewport_size({'width':width,'height':844})
                # Bring the editor itself into the documentation viewport.
                # Chromium's child-frame trial click can scroll its sidebar
                # without scrolling the parent page to the iframe below it.
                doc.locator('iframe').scroll_into_view_if_needed(timeout=10000)
                report['layouts'].append({'surface':'embedded-host','width':width,
                    'parent':doc.evaluate('''() => ({width:innerWidth,height:innerHeight,scrollX,scrollY,
                        iframe:document.querySelector('iframe').getBoundingClientRect().toJSON()})''')})
                narrow_authoring(frame,width,'embedded')
                handoff=document_snapshot(frame)
                assert summary(frame)['dirty']
                transfer_samples=[]
                assert_reachable(frame,'#studio-open-full',transfer_samples)
                with doc.expect_popup() as event:
                    frame.locator('#studio-open-full').click()
                narrow_full=event.value
                narrow_full.wait_for_function('()=>window.artStudioSurfaceReady===true',timeout=60000)
                narrow_full.wait_for_function("()=>!location.search.includes('transfer=')",timeout=35000)
                assert document_snapshot(narrow_full)==handoff, (width,'narrow embedded transfer changed the document')
                assert document_snapshot(frame)==handoff and summary(frame)['dirty'] and summary(narrow_full)['dirty']
                report['layouts'].append({'surface':'embedded-transfer','width':width,'samples':transfer_samples,'sourceSHA256':handoff['sourceSHA256']})
                doc.screenshot(path=str(args.evidence/f'narrow-embed-{width}.png'),timeout=30000)
                narrow_full.close()
                checked(f'embedded-{width}px-open-full-real-popup-preserves-complete-dirty-source')
            checked('390px-resize-controls-visible')
            assert not report['consoleErrors'], report['consoleErrors']
            browsers.close()
            if args.browser == 'firefox':
                # Use a real browser privacy policy, not an injected fake
                # IndexedDB, to verify the recovery-unavailable path.
                blocked_options = {'headless': True, 'firefox_user_prefs': {'network.cookie.cookieBehavior': 2, 'media.volume_scale': '0.0'}}
                if os.environ.get('MOZ_DISABLE_CONTENT_SANDBOX') == '1':
                    blocked_options['firefox_user_prefs']['security.sandbox.content.level'] = 0
                if args.executable:
                    blocked_options['executable_path'] = args.executable
                blocked_context, _ = browsers.enter_context(browser_context(
                    p, args.browser, blocked_options,
                    {'viewport':{'width':1440,'height':1000}, 'accept_downloads':True}, urls=[app, docs]))
                blocked_page = blocked_context.new_page()
                blocked_page.on('pageerror', lambda error: report['consoleErrors'].append(str(error)))
                blocked_page.on('console', lambda message: report['consoleErrors'].append(message.text) if message.type == 'error' else None)
                dialogs = []
                blocked_page.on('dialog', lambda dialog: (dialogs.append(dialog.type), dialog.accept()))
                blocked_page.goto(app+'editor/?lang=en', wait_until='load')
                blocked_page.wait_for_function('() => window.artStudioSurfaceReady === true', timeout=60000)
                assert not blocked_page.evaluate('() => AegoStudioHost.storageAvailable()')
                assert blocked_page.locator('.storage-warning').is_visible()
                checked('site-data-blocked-shows-recovery-warning')
                paint(blocked_page)
                assert summary(blocked_page)['dirty']
                with blocked_page.expect_download() as event:
                    blocked_page.locator('#studio-download-source').click()
                fallback = args.evidence/'storage-blocked.aeart'
                event.value.save_as(fallback)
                assert fallback.stat().st_size > 0 and summary(blocked_page)['dirty']
                blocked_page.reload(timeout=60000)
                assert 'beforeunload' in dialogs, f'Dirty unload guard not observed: {dialogs}'
                checked('storage-blocked-download-remains-dirty-and-guards-unload')
                assert not report['consoleErrors'], report['consoleErrors']
                browsers.close()
        report['status']='PASS'
        return 0
    except Exception as error:
        report['error'] = f'{type(error).__name__}: {error}'
        report['traceback'] = traceback.format_exc()
        print(report['error'],file=sys.stderr)
        return 1
    finally:
        (args.evidence/'browser.json').write_text(json.dumps(report,indent=2,ensure_ascii=False)+'\n')

if __name__ == '__main__':
    raise SystemExit(main())
