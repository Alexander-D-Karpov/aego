#!/usr/bin/env python3
"""Strict HTTP E2E on the built artifact. No set_content, route.fulfill or mock
WASM. A missing browser, policy block or failed assertion exits nonzero.
File download uses the browser's real download event; native OS pickers and
physical pen/touch devices are separate acceptance gates.
"""
from __future__ import annotations
import argparse
import hashlib
import json
import os
from pathlib import Path
import sys
import time
import traceback
import urllib.request


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
              'manifestSHA256': hashlib.sha256(raw).hexdigest(), 'checks': [],
              'transport': 'real-http', 'consoleErrors': [], 'limits': [
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
        from playwright.sync_api import sync_playwright
        with sync_playwright() as p:
            browser_type = getattr(p, args.browser)
            options = {'headless': True}
            if args.executable:
                options['executable_path'] = args.executable
            if args.browser == 'chromium':
                options['args'] = ['--no-sandbox', '--disable-dev-shm-usage']
            browser = browser_type.launch(**options)
            report['browserVersion'] = browser.version
            context = browser.new_context(viewport={'width':1440,'height':1000}, accept_downloads=True)
            # Observe every actual page, including the transferred editor and
            # each lesson, rather than only the two initially opened tabs.
            context.on('page', lambda opened: opened.on('pageerror', lambda error: report['consoleErrors'].append(str(error))))
            page = context.new_page()
            # Accept only the explicit reset and the dirty-document unload
            # confirmation exercised below; do not accept unrelated dialogs.
            page.on('dialog', lambda dialog: dialog.accept() if dialog.type in ('confirm', 'beforeunload') else dialog.dismiss())
            page.goto(app+'editor/?new=sprite&lang=en', wait_until='load')
            page.wait_for_function('() => window.artStudioSurfaceReady === true', timeout=60000)
            page.locator('#new-dialog [name=width]').fill('32')
            page.locator('#new-dialog [name=height]').fill('32')
            page.locator('#new-dialog [value=create]').click()
            assert summary(page)['width'] == 32
            checked('create-32x32-via-ui')
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
            page.locator('#file-input').set_input_files(str(saved))
            page.wait_for_function('() => AegoStudioHost.summary().frames === 8')
            assert summary(page)['pixelHash'] == pixel_hash
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
            page.locator('#restore').click()
            assert summary(page)['pixelHash'] == recovered_hash
            checked('indexeddb-dirty-recovery-after-reload')
            page.screenshot(path=str(args.evidence/'standalone.png'),full_page=True)
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
            doc.screenshot(path=str(args.evidence/'docs-transfer.png'), full_page=True)
            # Load each lesson in both languages, using the one shared artifact.
            for language in ('ru','en'):
                for item in manifest['examples']:
                    tutorial = context.new_page()
                    tutorial.goto(docs+f'{args.docs_prefix}/{language}/{item["id"]}/')
                    tutorial.locator('[data-action=start]').click()
                    tutorial.wait_for_function("() => !document.querySelector('[data-action=check]').disabled",timeout=60000)
                    f = next(f for f in tutorial.frames if '/embed/' in f.url)
                    assert summary(f)['buildID'] == manifest['buildID']
                    tutorial.close()
            checked('twelve-lessons-ru-en-real-editor')
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
                reference.close()
                checked('real-component-docs-link-localized-lazy-lessons')
            else:
                report['limits'].append('Standalone preview does not contain the real generated component reference pages.')
            # Firefox can stop producing frames for a background tab while the
            # 24 lesson pages are exercised; the final capture must render it.
            full.bring_to_front()
            full.set_viewport_size({'width':390,'height':844})
            assert full.locator('#studio-download-source').is_visible()
            full.screenshot(path=str(args.evidence/'narrow.png'), timeout=30000)
            checked('390px-resize-controls-visible')
            assert not report['consoleErrors'], report['consoleErrors']
            context.close();browser.close()
            if args.browser == 'firefox':
                # Use a real browser privacy policy, not an injected fake
                # IndexedDB, to verify the recovery-unavailable path.
                blocked_options = {'headless': True, 'firefox_user_prefs': {'network.cookie.cookieBehavior': 2}}
                if args.executable:
                    blocked_options['executable_path'] = args.executable
                blocked_browser = browser_type.launch(**blocked_options)
                blocked_context = blocked_browser.new_context(viewport={'width':1440,'height':1000}, accept_downloads=True)
                blocked_page = blocked_context.new_page()
                blocked_page.on('pageerror', lambda error: report['consoleErrors'].append(str(error)))
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
                blocked_context.close();blocked_browser.close()
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
