"""Targeted local Art Studio layout and file-workflow checks.

Build the current WASM separately, then pass --wasm. Serves only the checked-out
web shell, Go's wasm_exec.js and that WASM on loopback. File handles are mocks;
this is not an OS picker, physical GPU, or filesystem-atomicity test.
"""
import argparse
from functools import partial
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path
import subprocess
import threading
import xml.etree.ElementTree as ET

from playwright.sync_api import sync_playwright

ROOT = Path(__file__).resolve().parents[3]
WEB = ROOT / 'tools/artstudio/web'


class Handler(BaseHTTPRequestHandler):
    def __init__(self, *args, wasm, runtime, **kwargs):
        self.wasm, self.runtime = wasm, runtime
        super().__init__(*args, **kwargs)

    def do_GET(self):
        name = self.path.split('?', 1)[0].lstrip('/') or 'index.html'
        allowed = {'index.html', 'studio.css', 'studio.js', 'recovery.js', 'gpu-preview.js'}
        if name == 'artstudio.wasm':
            path = self.wasm
        elif name == 'wasm_exec.js':
            path = self.runtime
        elif name in allowed:
            path = WEB / name
        else:
            self.send_error(404)
            return
        body = path.read_bytes()
        self.send_response(200)
        self.send_header('Content-Type', 'application/wasm' if name.endswith('.wasm') else
                         'text/html; charset=utf-8' if name.endswith('.html') else
                         'text/css; charset=utf-8' if name.endswith('.css') else 'text/javascript; charset=utf-8')
        self.send_header('Content-Length', str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def log_message(self, *_args):
        pass


MOCKS = """() => {
  class MockFile {
    constructor(name, bytes) { this.name=name; this.kind='file'; this.bytes=new Uint8Array(bytes); this.writes=0; this.aborts=0; this.denied=false; this.fail=false; }
    async isSameEntry(other) { return this===other; }
    async getFile() { return new File([this.bytes],this.name); }
    async requestPermission() { return this.denied ? 'denied' : 'granted'; }
    async createWritable() {
      this.writes++;
      let next;
      return {write:async bytes=>{if(this.fail)throw Error('mock write failure');next=new Uint8Array(bytes)},
              close:async()=>{this.bytes=next}, abort:async()=>{this.aborts++}};
    }
  }
  window.MockFile=MockFile;
  window.picks=0;
  window.openHandle=new MockFile('original.aeart',aegoArt.exportBytes('recovery').bytes);
  window.saveHandle=new MockFile('copy.aeart',[]);
  window.showOpenFilePicker=async()=>[window.openHandle];
  window.showSaveFilePicker=async()=>{window.picks++;return window.saveHandle};
}"""


def info(page):
    return page.evaluate('JSON.parse(aegoArt.request(\'{"action":"state"}\'))')


def check(name):
    print('PASS', name, flush=True)


def hit_test_sidebar(page):
    assert page.locator('#sidebar').bounding_box()['width'] >= 200
    for selector in ('#mode', '#fg-color', '#hex-color', '#fg-alpha', '#swap-colors', '#add-layer', '#layer-properties',
                     '#opacity', '#blend', '#alpha-lock', '#layer-up', '#layer-down', '#merge-down', '#isolate-layer'):
        control = page.locator(selector)
        control.scroll_into_view_if_needed()
        if control.is_enabled():
            control.click(trial=True)
        hits = control.evaluate('''element => {
          const r=element.getBoundingClientRect(), doc=element.ownerDocument;
          return [[.1,.1],[.5,.5],[.9,.9]].map(([x,y]) => {
            const hit=doc.elementFromPoint(r.x+r.width*x,r.y+r.height*y);
            return {ok:hit === element || element.contains(hit), hit:hit?.outerHTML?.slice(0,150), rect:r.toJSON()};
          });
        }''')
        assert all(hit['ok'] for hit in hits), (selector, hits)


def main():
    if not __debug__:
        raise SystemExit('Browser checks must not run with Python optimizations (-O/PYTHONOPTIMIZE)')
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--wasm', required=True, type=Path)
    parser.add_argument('--engine', choices=['chromium', 'firefox'], default='chromium')
    parser.add_argument('--chromium', help='optional Playwright-compatible Chromium executable')
    parser.add_argument('--firefox', help='optional Playwright-patched Firefox executable')
    parser.add_argument('--screenshots', type=Path)
    args = parser.parse_args()
    goroot = Path(subprocess.check_output(['go', 'env', 'GOROOT'], cwd=ROOT, text=True).strip())
    runtime = goroot / 'lib/wasm/wasm_exec.js'
    if not runtime.exists():
        runtime = goroot / 'misc/wasm/wasm_exec.js'
    server = ThreadingHTTPServer(('127.0.0.1', 0), partial(Handler, wasm=args.wasm.resolve(), runtime=runtime))
    threading.Thread(target=server.serve_forever, daemon=True).start()
    url = f'http://127.0.0.1:{server.server_port}/'
    try:
        with sync_playwright() as p:
            browser = (p.chromium.launch(executable_path=args.chromium, headless=True, args=['--mute-audio'])
                       if args.engine == 'chromium' else
                       p.firefox.launch(executable_path=args.firefox, headless=True, firefox_user_prefs={'media.volume_scale':'0.0'}))
            context = browser.new_context(viewport={'width': 1440, 'height': 900}, accept_downloads=True)
            page = context.new_page()
            errors = []
            page.on('pageerror', lambda error: errors.append(str(error)))
            page.goto(url)
            page.wait_for_function('window.artStudioReady === true', timeout=30000)
            page.locator('#theme').select_option('dark')
            left = page.locator('.left-dock').bounding_box()
            tools = page.locator('.tools-panel').bounding_box()
            options = page.locator('.options-panel').bounding_box()
            canvas = page.locator('.document-area').bounding_box()
            right = page.locator('.sidebar').bounding_box()
            timeline = page.locator('.timeline').bounding_box()
            assert left['x'] < canvas['x'] < right['x'] and options['y'] >= tools['y'] + tools['height']
            assert timeline['y'] >= canvas['y'] + canvas['height'] and page.locator('#viewport').bounding_box()['height'] > 300
            hit_test_sidebar(page)
            for web_name, native_name in {'pencil':'pencil', 'eraser':'eraser', 'fill':'fill', 'eyedropper':'eyedropper',
                                          'line':'line', 'rectangle':'rectangle', 'ellipse':'ellipse', 'gradient':'gradient',
                                          'select-rect':'selection', 'select-ellipse':'select_ellipse', 'lasso':'lasso', 'wand':'wand'}.items():
                icon = ET.parse(ROOT / f'engine/res/icons/src/tool.{native_name}.svg').getroot()
                native_path = icon.find('{http://www.w3.org/2000/svg}path').attrib['d']
                assert page.locator(f'[data-tool="{web_name}"] svg path').get_attribute('d') == native_path
            assert page.locator('#brush-size').is_visible()
            page.locator('[data-tool="eyedropper"]').click()
            assert not page.locator('#brush-size').is_visible()
            page.locator('[data-tool="pencil"]').click()
            assert page.locator('#brush-size').is_visible()
            old_hash = page.evaluate('aegoArt.hash()')
            box = page.locator('#canvas').bounding_box()
            page.mouse.click(box['x'] + box['width'] / 2, box['y'] + box['height'] / 2)
            painted = page.evaluate('aegoArt.hash()')
            assert painted != old_hash
            page.locator('#undo').click()
            assert page.evaluate('aegoArt.hash()') == old_hash
            page.locator('#redo').click()
            assert page.evaluate('aegoArt.hash()') == painted
            for _ in range(7):
                page.locator('#add-frame').click()
            assert len(info(page)['state']['document']['frames']) == 8
            assert page.locator('#timeline-table th button[data-frame]').count() == 8
            assert page.locator('#timeline-table th button[data-frame]').first.bounding_box()['width'] <= 42
            page.evaluate("window.dragBytes=aegoArt.exportBytes('recovery').bytes")
            page.mouse.move(box['x'] + box['width'] / 3, box['y'] + box['height'] / 3)
            page.mouse.down()
            assert info(page)['state']['gesture']
            page.evaluate("""() => {
              const dt=new DataTransfer();dt.items.add(new File([window.dragBytes],'busy.aeart'));
              window.dispatchEvent(new DragEvent('drop',{dataTransfer:dt,bubbles:true,cancelable:true}));
            }""")
            page.wait_for_function('document.querySelector("#status").textContent.includes("завершите")')
            assert info(page)['state']['gesture'] and len(info(page)['tabs']) == 1
            page.mouse.up()
            assert not info(page)['state']['gesture']
            check('dropping a file during a stroke does not cancel or replace the gesture')
            if args.screenshots:
                args.screenshots.mkdir(parents=True, exist_ok=True)
                page.screenshot(path=str(args.screenshots / 'dark.png'))
            page.locator('#theme').select_option('light')
            assert page.locator('html').get_attribute('data-theme') == 'light'
            if args.screenshots:
                page.screenshot(path=str(args.screenshots / 'light.png'))
            page.locator('#language').click()
            assert page.locator('#save-as').inner_text() == 'Save As…'
            assert page.locator('.options-panel .dock-title').inner_text() == 'Tool Options'
            page.locator('#language').click()
            assert page.locator('#save-as').inner_text() == 'Сохранить как…'
            check('desktop native-inspired panel order, contextual options and both themes')

            for width, height in [(1366,768), (1920,1080), (2560,1440), (800,600)]:
                page.set_viewport_size({'width':width, 'height':height})
                assert page.evaluate('document.documentElement.scrollWidth <= innerWidth')
                assert page.locator('#viewport').bounding_box()['height'] > 150
                assert page.locator('#brush-size').is_visible() and page.locator('#swatches').is_visible()
            check('desktop and narrow layouts retain a usable canvas')

            for width in (390, 320):
                page.set_viewport_size({'width':width, 'height':844})
                assert page.evaluate('document.documentElement.scrollWidth <= innerWidth')
                page.locator('#toggle-tools').click()
                assert page.locator('#brush-size').is_visible() and page.locator('#toggle-tools').get_attribute('aria-expanded') == 'true'
                page.locator('#toggle-tools').click()
                page.locator('#toggle-side').click()
                assert page.locator('#sidebar').is_visible() and page.locator('#toggle-side').get_attribute('aria-expanded') == 'true'
                hit_test_sidebar(page)
                page.locator('#toggle-side').click()
            page.set_viewport_size({'width':390, 'height':844})
            if args.screenshots:
                page.screenshot(path=str(args.screenshots / 'mobile.png'))
            # Exercise the release's embedded CSS and extra toolbar without
            # claiming a production build, iframe protocol or CSP test.
            surface = (ROOT / 'tools/artstudio/release/web/surface.css').read_text()
            extra_style = page.add_style_tag(content=surface)
            page.evaluate("""() => {
              document.body.classList.add('studio-embedded');
              const bar=document.createElement('div');bar.className='release-toolbar';bar.textContent='Test toolbar';
              document.querySelector('.menubar').after(bar);
            }""")
            page.locator('#toggle-side').click()
            assert page.locator('#sidebar').is_visible()
            assert abs(page.locator('#sidebar').bounding_box()['y'] - page.locator('.studio').bounding_box()['y']) < 1
            hit_test_sidebar(page)
            page.locator('#toggle-side').click()
            page.locator('#toggle-tools').click()
            assert page.locator('#brush-size').is_visible()
            page.locator('#toggle-tools').click()
            page.evaluate("document.body.classList.remove('studio-embedded');document.querySelector('.release-toolbar').remove()")
            extra_style.evaluate('(style) => style.remove()')
            page.set_viewport_size({'width':1440, 'height':900})
            check('mobile and simulated embed tool options/sidebar remain reachable without root overflow')

            page.evaluate(MOCKS)
            page.locator('#open').click()
            page.wait_for_function('document.querySelectorAll("[data-document]").length === 2')
            key = info(page)['active']
            assert not info(page)['state']['dirty']
            page.locator('#open').click()
            page.wait_for_function('document.querySelector("#status").textContent.includes("уже открыт")')
            assert len(info(page)['tabs']) == 2 and info(page)['active'] == key
            page.locator('#add-frame').click()
            assert info(page)['state']['dirty']
            page.locator('#save').click()
            page.wait_for_function('window.openHandle.writes === 1 && !JSON.parse(aegoArt.request(\'{"action":"state"}\')).state.dirty')
            assert page.evaluate('window.picks') == 0
            page.locator('#add-frame').click()
            page.locator('#brush-size').focus()
            page.keyboard.press('Control+s')
            page.wait_for_function('window.openHandle.writes === 2 && !JSON.parse(aegoArt.request(\'{"action":"state"}\')).state.dirty')
            check('open .aeart, focus duplicate, repeat Save from a field without another picker')

            page.locator('#add-frame').click()
            page.evaluate("""() => {
              window.openHandle.reads=0;
              window.openHandle.getFile=async function(){
                if(++this.reads===2)this.bytes=new Uint8Array([1,2,3]);
                return new File([this.bytes],this.name);
              };
            }""")
            page.locator('#save').click()
            page.wait_for_function('window.openHandle.reads === 2 && document.querySelector("#status").textContent.includes("изменён извне")')
            assert page.evaluate('window.openHandle.writes') == 2 and info(page)['state']['dirty']
            page.evaluate("""() => {
              window.openHandle.getFile=MockFile.prototype.getFile;
              window.showSaveFilePicker=async()=>window.openHandle;
            }""")
            page.locator('#save-as').click()
            page.wait_for_function('!saveBusy')
            assert page.evaluate('window.openHandle.writes') == 2 and info(page)['state']['dirty']
            assert 'изменён извне' in page.locator('#status').inner_text()
            page.evaluate('() => { window.showSaveFilePicker=async()=>window.saveHandle; }')
            page.locator('#save-as').click()
            page.wait_for_function('window.saveHandle.writes === 1 && !JSON.parse(aegoArt.request(\'{"action":"state"}\')).state.dirty')
            assert page.evaluate('Array.from(window.openHandle.bytes).join(",")') == '1,2,3'
            assert next(tab for tab in info(page)['tabs'] if tab['key'] == key)['name'] == 'copy.aeart'
            check('external conflicts, including a change during Save and Save As to the same file, refuse writes')
            check('Save As writes a separate copy without touching the changed original')

            page.locator('#add-frame').click()
            page.evaluate("""() => {
              window.oldExport=aegoArt.exportBytes;
              aegoArt.exportBytes=()=>{throw Error('mock export failure')};
            }""")
            page.locator('#save').click()
            page.wait_for_function('!saveBusy && document.querySelector("#status").textContent.includes("mock export failure")')
            assert info(page)['state']['dirty']
            page.evaluate('() => { aegoArt.exportBytes=window.oldExport; }')
            page.evaluate("() => { window.showSaveFilePicker=async()=>{throw new DOMException('cancelled','AbortError')}; }")
            page.locator('#save-as').click()
            page.wait_for_function('document.querySelector("#status").textContent.includes("отменено")')
            assert info(page)['state']['dirty'] and page.evaluate('window.saveHandle.writes') == 1
            page.evaluate('window.saveHandle.denied=true')
            page.locator('#save').click()
            page.wait_for_function('document.querySelector("#status").textContent.includes("разрешения")')
            assert info(page)['state']['dirty'] and page.evaluate('window.saveHandle.writes') == 1
            page.evaluate('window.saveHandle.denied=false;window.saveHandle.fail=true')
            page.locator('#save').click()
            page.wait_for_function('document.querySelector("#status").textContent.includes("mock write failure")')
            assert info(page)['state']['dirty'] and page.evaluate('window.saveHandle.aborts') == 1
            check('export error, cancel, denied permission and failed writer retain dirty state')

            page.evaluate("""() => {
              window.occupied=new MockFile('occupied.aeart',[9,8,7]);
              window.showSaveFilePicker=async()=>window.occupied;
            }""")
            page.once('dialog', lambda dialog: dialog.dismiss())
            page.locator('#save-as').click()
            page.wait_for_function('!saveBusy && document.querySelector("#status").textContent.includes("отменено")')
            assert info(page)['state']['dirty'] and page.evaluate('window.occupied.writes') == 0
            assert page.evaluate('Array.from(window.occupied.bytes).join(",")') == '9,8,7'
            check('declining replacement of a nonempty Save As target leaves its bytes intact')

            page.evaluate("""() => {
              window.saveHandle.fail=false;
              window.staleHandle=new MockFile('stale.aeart',[]);
              window.showSaveFilePicker=()=>new Promise(resolve=>{window.finishPicker=()=>resolve(window.staleHandle)});
            }""")
            page.locator('#save-as').click()
            page.wait_for_function('typeof window.finishPicker === "function"')
            page.locator('#add-frame').click()
            page.evaluate('window.finishPicker()')
            page.wait_for_function('document.querySelector(".statusbar").classList.contains("error")')
            assert page.evaluate('window.staleHandle.writes') == 0 and info(page)['state']['dirty']
            check('stale Save As picker is rejected before opening a writer')

            page.evaluate("""() => {
              window.saveHandle.createWritable=async()=>{
                window.saveHandle.writes++;
                let next;
                return {write:async bytes=>{next=new Uint8Array(bytes)},
                  close:()=>new Promise(resolve=>{window.finishWrite=()=>{window.saveHandle.bytes=next;resolve()}}),abort:async()=>{}};
              };
            }""")
            page.locator('#save').click()
            page.wait_for_function('typeof window.finishWrite === "function"')
            page.locator('#add-frame').click()
            page.locator('[data-document]').first.click()
            page.evaluate('window.finishWrite()')
            page.wait_for_function('document.querySelector("#status").textContent.includes("записан")')
            assert next(tab for tab in info(page)['tabs'] if tab['key'] == key)['dirty']
            page.locator(f'[data-document="{key}"]').click()
            check('save completion targets its original tab and keeps later edits dirty')

            page.evaluate('window.showSaveFilePicker=undefined')
            with page.expect_download() as download:
                page.locator('#save-as').click()
            assert download.value.suggested_filename == 'copy.aeart' and info(page)['state']['dirty']
            assert 'браузер не подтверждает' in page.locator('#status').inner_text()
            check('download fallback never marks a document clean')

            page.evaluate("""() => {
              const bytes=aegoArt.exportBytes('png').bytes;
              window.pngHandle=new MockFile('image.png',bytes);
              window.saveHandle=new MockFile('image.aeart',[]);
              window.showOpenFilePicker=async()=>[window.pngHandle];
              window.showSaveFilePicker=async()=>{window.picks++;return window.saveHandle};
            }""")
            page.locator('#open').click()
            page.wait_for_function('document.querySelectorAll("[data-document]").length === 3')
            assert info(page)['state']['dirty']
            page.locator('#save').click()
            page.wait_for_function('window.saveHandle.writes === 1 && !JSON.parse(aegoArt.request(\'{"action":"state"}\')).state.dirty')
            assert page.evaluate('window.pngHandle.writes') == 0
            check('PNG import saves an independent .aeart, not the original PNG')

            page.locator('#add-frame').click()
            page.evaluate('() => { window.showSaveFilePicker=async()=>window.occupied; }')
            page.once('dialog', lambda dialog: dialog.accept())
            page.locator('#brush-size').focus()
            page.keyboard.press('Control+Shift+s')
            page.wait_for_function('window.occupied.writes === 1 && !JSON.parse(aegoArt.request(\'{"action":"state"}\')).state.dirty')
            assert page.evaluate('window.occupied.bytes.length') > 3
            assert page.evaluate('window.pngHandle.writes') == 0
            check('confirmed replacement and Ctrl+Shift+S write only the selected .aeart')

            page.evaluate("""() => {
              const dt=new DataTransfer(), bytes=aegoArt.exportBytes('recovery').bytes;
              dt.items.add(new File([bytes],'drop-one.aeart'));
              dt.items.add(new File([bytes],'drop-two.aeart'));
              window.dispatchEvent(new DragEvent('drop',{dataTransfer:dt,bubbles:true,cancelable:true}));
            }""")
            page.wait_for_function('document.querySelectorAll("[data-document]").length === 5')
            names = [tab['name'] for tab in info(page)['tabs']]
            assert 'drop-one.aeart' in names and 'drop-two.aeart' in names
            check('multiple dropped local documents open as independent tabs')

            data = bytes(page.evaluate("Array.from(aegoArt.exportBytes('recovery').bytes)"))
            page.evaluate('window.showOpenFilePicker=undefined')
            with page.expect_file_chooser() as chooser:
                page.locator('#open').click()
            chooser.value.set_files([{'name':'input-one.aeart','mimeType':'application/octet-stream','buffer':data},
                                     {'name':'input-two.aeart','mimeType':'application/octet-stream','buffer':data}])
            page.wait_for_function('document.querySelectorAll("[data-document]").length === 7')
            names = [tab['name'] for tab in info(page)['tabs']]
            assert 'input-one.aeart' in names and 'input-two.aeart' in names
            check('multiple-file input fallback opens independent local tabs')

            assert not errors, errors
            print(args.engine, browser.version, 'headless; muted audio; loopback; mocked handles', flush=True)
            browser.close()
    finally:
        server.shutdown()
        server.server_close()


if __name__ == '__main__':
    main()
