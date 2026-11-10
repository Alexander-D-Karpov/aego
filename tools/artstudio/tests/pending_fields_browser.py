"""Real Go/WASM document-field regressions, with mock local file handles.

Pass --wasm pointing to a separately built current tools/artstudio/wasm binary.
The loopback browser test does not exercise OS dialogs or physical disk writes.
"""
import argparse
from functools import partial
from http.server import ThreadingHTTPServer
from pathlib import Path
import subprocess
import threading

from playwright.sync_api import sync_playwright
from workspace_files_browser import Handler, ROOT


MOCKS = """() => {
  class Probe {
    constructor(name, mode='normal') { this.name=name;this.kind='file';this.bytes=new Uint8Array();this.mode=mode;this.opens=0;this.writes=0;this.aborts=0; }
    async isSameEntry(other) { return this===other; }
    async getFile() { return new File([this.bytes],this.name); }
    async requestPermission() { return 'granted'; }
    async createWritable() {
      this.opens++;
      let staged;
      const stream={write:async data=>{
        this.writes++;staged=new Uint8Array(data);
        if(this.mode==='write')await new Promise(resolve=>window.releaseWrite=resolve);
      },close:async()=>{
        if(this.mode==='close')await new Promise(resolve=>window.releaseClose=resolve);
        this.bytes=staged;
      },abort:async()=>{this.aborts++}};
      if(this.mode==='create')await new Promise(resolve=>window.releaseCreate=resolve);
      return stream;
    }
  }
  window.Probe=Probe;window.pickerCalls=0;window.openCalls=0;
  window.target=new Probe('first.aeart');
  window.showSaveFilePicker=async()=>{window.pickerCalls++;return window.target};
  window.showOpenFilePicker=async()=>{window.openCalls++;return []};
}"""


def info(page):
    return page.evaluate('JSON.parse(aegoArt.request(\'{"action":"state"}\'))')


def duration(page):
    state = info(page)['state']
    return next(frame['durationMS'] for frame in state['document']['frames'] if frame['id'] == state['frame'])


def check(label):
    print('PASS', label, flush=True)


def reopen(page, expression, expected):
    data = bytes(page.evaluate(f'Array.from({expression}.bytes)'))
    count = len(info(page)['tabs'])
    page.locator('#file-input').set_input_files({'name':f'copy-{count}.aeart', 'mimeType':'application/octet-stream', 'buffer':data})
    page.wait_for_function('(count) => document.querySelectorAll("[data-document]").length === count + 1', arg=count)
    assert duration(page) == expected, (duration(page), expected)


def select(page, key):
    page.locator(f'[data-document="{key}"]').click()
    assert info(page)['active'] == key


def wait_saved(page, expression, count):
    page.wait_for_function(f'{expression}.writes === {count} && !saveBusy')


def main():
    if not __debug__:
        raise SystemExit('Browser checks must not run with Python optimizations (-O/PYTHONOPTIMIZE)')
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--wasm', required=True, type=Path)
    parser.add_argument('--engine', choices=['chromium','firefox'], default='chromium')
    parser.add_argument('--chromium', help='optional Playwright-compatible Chromium executable')
    parser.add_argument('--firefox', help='optional Playwright-patched Firefox executable')
    args = parser.parse_args()
    goroot = Path(subprocess.check_output(['go','env','GOROOT'], cwd=ROOT, text=True).strip())
    runtime = goroot / 'lib/wasm/wasm_exec.js'
    if not runtime.exists():
        runtime = goroot / 'misc/wasm/wasm_exec.js'
    server = ThreadingHTTPServer(('127.0.0.1',0), partial(Handler, wasm=args.wasm.resolve(), runtime=runtime))
    threading.Thread(target=server.serve_forever, daemon=True).start()
    try:
        with sync_playwright() as p:
            browser = (p.chromium.launch(executable_path=args.chromium, headless=True, args=['--mute-audio'])
                       if args.engine == 'chromium' else
                       p.firefox.launch(executable_path=args.firefox, headless=True, firefox_user_prefs={'media.volume_scale':'0.0'}))
            page = browser.new_page(viewport={'width':1440,'height':900})
            errors = []
            page.on('pageerror', lambda error: errors.append(str(error)))
            page.goto(f'http://127.0.0.1:{server.server_port}/')
            page.wait_for_function('window.artStudioReady === true', timeout=30000)
            page.evaluate(MOCKS)
            owner = info(page)['active']
            field = page.locator('#duration')

            # Ctrl+S commits the still-focused field, writes that exact snapshot,
            # and the later native change event must not add another Undo entry.
            field.fill('241')
            assert duration(page) == 100 and info(page)['state']['undo'] == 0
            page.keyboard.press('Control+s')
            wait_saved(page, 'window.target', 1)
            assert duration(page) == 241 and info(page)['state']['undo'] == 1 and not info(page)['state']['dirty']
            page.locator('#brush-size').focus()
            assert info(page)['state']['undo'] == 1
            reopen(page, 'window.target', 241)
            first_copy = info(page)['active']
            field.fill('260')
            page.once('dialog', lambda dialog: dialog.dismiss())
            page.locator(f'[data-close-document="{first_copy}"]').click()
            assert info(page)['active'] == first_copy and duration(page) == 260 and info(page)['state']['dirty']
            page.locator('#undo').click()
            assert duration(page) == 241 and not info(page)['state']['dirty']
            select(page, owner)
            check('Ctrl+S saves and reopens focused duration once; closing a newly dirty tab prompts')

            page.evaluate("window.target=new Probe('second.aeart')")
            field.fill('319')
            page.keyboard.press('Control+Shift+s')
            wait_saved(page, 'window.target', 1)
            assert duration(page) == 319 and info(page)['state']['undo'] == 2
            reopen(page, 'window.target', 319)
            select(page, owner)
            field.fill('400')
            page.locator('#save').click()
            wait_saved(page, 'window.target', 2)
            assert duration(page) == 400 and info(page)['state']['undo'] == 3
            reopen(page, 'window.target', 400)
            select(page, owner)
            page.evaluate("window.target=new Probe('mouse-save-as.aeart')")
            field.fill('450')
            page.locator('#save-as').click()
            wait_saved(page, 'window.target', 1)
            assert duration(page) == 450 and info(page)['state']['undo'] == 4
            reopen(page, 'window.target', 450)
            select(page, owner)
            check('Ctrl+Shift+S and mouse Save/Save As persist each entered duration once')

            # Invalid drafts survive both keyboard and mouse focus transitions;
            # neither picker nor writer may run, and switching tabs is blocked.
            picker_calls = page.evaluate('window.pickerCalls')
            open_calls = page.evaluate('window.openCalls')
            writes = page.evaluate('window.target.writes')
            for value in ['', '0', '600001', '1.5']:
                field.fill(value)
                page.keyboard.press('Control+s')
                assert field.input_value() == value and field.get_attribute('aria-invalid') == 'true'
                assert 'Длительность должна' in page.locator('#status').inner_text()
                assert duration(page) == 450 and info(page)['active'] == owner
                assert page.evaluate('window.target.writes') == writes
            field.fill('0')
            page.keyboard.press('Control+Shift+s')
            page.keyboard.press('Control+o')
            page.locator('#save').click()
            page.locator('#save-as').click()
            page.locator('#open').click()
            page.evaluate("window.paletteClicks=0;document.querySelector('#palette-input').addEventListener('click',()=>window.paletteClicks++)")
            page.locator('#palette-import').click()
            page.locator(f'[data-document="{first_copy}"]').click()
            assert info(page)['active'] == owner and field.input_value() == '0'
            assert page.evaluate('window.pickerCalls') == picker_calls and page.evaluate('window.openCalls') == open_calls
            assert page.evaluate('window.paletteClicks') == 0
            assert page.evaluate('window.target.writes') == writes and info(page)['state']['undo'] == 4
            assert all(not tab['dirty'] for tab in info(page)['tabs'])
            assert page.locator(f'[data-document="{owner}"]').inner_text().startswith('•')
            assert page.evaluate("""() => {const e=new Event('beforeunload',{cancelable:true});window.dispatchEvent(e);return e.defaultPrevented}""")
            field.focus();page.keyboard.press('Escape')
            assert field.input_value() == '450' and info(page)['state']['undo'] == 4
            field.focus();page.keyboard.press('Control+a');page.keyboard.type('1e')
            if field.evaluate('e => e.validity.badInput'):
                page.keyboard.press('Control+s')
                page.locator('#save').click()
                assert field.evaluate('e => e.validity.badInput') and page.evaluate('window.target.writes') == writes
                field.focus();page.keyboard.press('Escape')
                assert field.input_value() == '450'
            check('invalid fields block shortcuts, buttons and tab switches without discarding visible input')

            # A normal tab switch flushes before changing the active session.
            field.fill('470')
            page.locator(f'[data-document="{first_copy}"]').click()
            assert info(page)['active'] == first_copy
            select(page, owner)
            assert duration(page) == 470 and info(page)['state']['undo'] == 5
            field.fill('470')
            page.locator(f'[data-document="{first_copy}"]').click()
            assert info(page)['active'] == first_copy
            select(page, owner)
            assert duration(page) == 470 and info(page)['state']['undo'] == 5
            check('valid pending field is applied to its original tab before switching')

            # Picker is pending while the user edits again: commit the new
            # value but reject the stale export before any writer is opened.
            page.evaluate("""() => {
              window.slow=new Probe('slow-picker.aeart');
              window.showSaveFilePicker=()=>new Promise(resolve=>window.releasePicker=()=>resolve(window.slow));
            }""")
            field.fill('501');page.keyboard.press('Control+Shift+s')
            page.wait_for_function('typeof window.releasePicker === "function"')
            field.fill('502')
            page.evaluate('window.releasePicker()')
            page.wait_for_function('!saveBusy')
            assert duration(page) == 502 and field.input_value() == '502' and info(page)['state']['dirty']
            assert page.evaluate('window.slow.opens') == 0
            assert 'изменился' in page.locator('#status').inner_text()
            check('new valid field input while Save As picker waits rejects the stale snapshot')

            page.evaluate("""() => {
              window.slow=new Probe('picker-tab.aeart');window.releasePicker=null;
              window.showSaveFilePicker=()=>new Promise(resolve=>window.releasePicker=()=>resolve(window.slow));
            }""")
            field.fill('503');page.keyboard.press('Control+Shift+s')
            page.wait_for_function('typeof window.releasePicker === "function"')
            field.fill('504');page.locator(f'[data-document="{first_copy}"]').click()
            assert info(page)['active'] == first_copy
            page.evaluate('window.releasePicker()');page.wait_for_function('!saveBusy')
            assert page.evaluate('window.slow.opens') == 0
            select(page, owner)
            assert duration(page) == 504 and info(page)['state']['dirty']
            check('tab switch during a pending picker preserves the new edit and rejects the old snapshot')

            page.evaluate("""() => {
              window.slow=new Probe('invalid-picker.aeart');
              window.releasePicker=null;
              window.showSaveFilePicker=()=>new Promise(resolve=>window.releasePicker=()=>resolve(window.slow));
            }""")
            field.fill('510');page.keyboard.press('Control+Shift+s')
            page.wait_for_function('typeof window.releasePicker === "function"')
            field.fill('0');page.locator(f'[data-document="{first_copy}"]').click()
            assert info(page)['active'] == owner and field.input_value() == '0'
            page.evaluate('window.releasePicker()')
            page.wait_for_function('!saveBusy')
            assert duration(page) == 510 and field.input_value() == '0' and page.evaluate('window.slow.opens') == 0
            assert field.get_attribute('aria-invalid') == 'true'
            field.fill('511');field.press('Tab')
            check('invalid input while picker waits blocks the write and remains editable')

            # The stream may have been opened before the new edit. Abort it
            # before close if a new field value appears during create/write.
            page.evaluate("""() => {window.slow=new Probe('slow-create.aeart','create');window.releaseCreate=null;
              window.showSaveFilePicker=async()=>window.slow;}""")
            field.fill('601');page.keyboard.press('Control+Shift+s')
            page.wait_for_function('typeof window.releaseCreate === "function"')
            field.fill('602');page.evaluate('window.releaseCreate()')
            page.wait_for_function('!saveBusy')
            assert duration(page) == 602 and page.evaluate('window.slow.aborts') == 1
            assert page.evaluate('window.slow.bytes.length') == 0
            page.evaluate("""() => {window.slow=new Probe('slow-write.aeart','write');window.releaseWrite=null;
              window.showSaveFilePicker=async()=>window.slow;}""")
            field.fill('610');page.keyboard.press('Control+Shift+s')
            page.wait_for_function('typeof window.releaseWrite === "function"')
            field.fill('0');page.evaluate('window.releaseWrite()')
            page.wait_for_function('!saveBusy')
            assert field.input_value() == '0' and page.evaluate('window.slow.aborts') == 1
            assert page.evaluate('window.slow.bytes.length') == 0
            field.fill('611');field.press('Tab')
            check('new drafts during writer creation/write abort the unclosed stream')

            # Once close() is in flight it cannot be reliably stopped. Its old
            # snapshot may land, but later valid edits must remain dirty.
            page.evaluate("""() => {window.slow=new Probe('slow-close.aeart','close');window.releaseClose=null;
              window.showSaveFilePicker=async()=>window.slow;}""")
            field.fill('701');page.keyboard.press('Control+Shift+s')
            page.wait_for_function('typeof window.releaseClose === "function"')
            field.fill('702');page.evaluate('window.releaseClose()')
            page.wait_for_function('!saveBusy')
            assert duration(page) == 702 and info(page)['state']['dirty']
            reopen(page, 'window.slow', 701)
            select(page, owner)
            assert duration(page) == 702 and info(page)['state']['dirty']
            check('edit during in-flight close stays dirty; reopened bytes contain the earlier snapshot')

            page.evaluate("""() => {window.previousSource=window.slow;window.slow=new Probe('invalid-close.aeart','close');window.releaseClose=null;
              window.showSaveFilePicker=async()=>window.slow;}""")
            field.fill('710');page.keyboard.press('Control+Shift+s')
            page.wait_for_function('typeof window.releaseClose === "function"')
            field.fill('0');page.evaluate('window.releaseClose()')
            page.wait_for_function('!saveBusy')
            assert field.input_value() == '0' and field.get_attribute('aria-invalid') == 'true'
            assert info(page)['state']['dirty'] and 'уже записана' in page.locator('#status').inner_text()
            field.fill('711');field.press('Tab')
            reopen(page, 'window.slow', 710)
            select(page, owner)
            assert duration(page) == 711 and info(page)['state']['dirty']
            check('invalid edit during in-flight close remains visible and is not acknowledged as saved')

            # Asynchronous Open must also flush a new edit or reject an invalid
            # one before it publishes a different active document.
            page.evaluate("""() => {
              window.openCandidate=new Probe('candidate.aeart');window.openCandidate.bytes=new Uint8Array(window.target.bytes);
              window.releaseOpen=null;window.showOpenFilePicker=()=>new Promise(resolve=>window.releaseOpen=()=>resolve([window.openCandidate]));
            }""")
            field.fill('801');page.keyboard.press('Control+o')
            page.wait_for_function('typeof window.releaseOpen === "function"')
            field.fill('802');page.evaluate('window.releaseOpen()')
            page.wait_for_function('(key) => JSON.parse(aegoArt.request(\'{"action":"state"}\')).active !== key', arg=owner)
            select(page, owner)
            assert duration(page) == 802
            page.evaluate("""() => {window.releaseOpen=null;
              window.showOpenFilePicker=()=>new Promise(resolve=>window.releaseOpen=()=>resolve([window.openCandidate]));}""")
            field.fill('810');page.keyboard.press('Control+o')
            page.wait_for_function('typeof window.releaseOpen === "function"')
            field.fill('0');page.evaluate('window.releaseOpen()')
            page.wait_for_function('document.querySelector("#duration").getAttribute("aria-invalid") === "true"')
            assert info(page)['active'] == owner and field.input_value() == '0' and duration(page) == 810
            field.fill('811');field.press('Tab')
            check('asynchronous Open applies valid input and refuses a transition with invalid input')

            # Import has a second yield while reading bytes after the picker.
            page.evaluate("""() => {
              window.releaseRead=null;
              window.readHandle={name:'slow-read.aeart',kind:'file',getFile:async()=>({
                name:'slow-read.aeart',size:window.target.bytes.length,
                arrayBuffer:()=>new Promise(resolve=>window.releaseRead=()=>resolve(window.target.bytes.buffer.slice(0)))
              })};
              window.showOpenFilePicker=async()=>[window.readHandle];
            }""")
            field.fill('820');page.keyboard.press('Control+o')
            page.wait_for_function('typeof window.releaseRead === "function"')
            field.fill('0');page.evaluate('window.releaseRead()')
            page.wait_for_function('document.querySelector("#duration").getAttribute("aria-invalid") === "true"')
            assert info(page)['active'] == owner and field.input_value() == '0' and duration(page) == 820
            field.fill('821');field.press('Tab')
            check('invalid input during asynchronous file reading prevents publication')

            page.evaluate("""() => {
              window.releasePalette=null;window.originalArrayBuffer=File.prototype.arrayBuffer;
              File.prototype.arrayBuffer=function(){
                if(this.name!=='slow.gpl')return window.originalArrayBuffer.call(this);
                return new Promise(resolve=>window.releasePalette=()=>resolve(new TextEncoder().encode('GIMP Palette\\n255 0 0 Red\\n').buffer));
              };
            }""")
            before_palette = info(page)['state']['document']['palette']
            field.fill('831')
            page.locator('#palette-input').set_input_files({'name':'slow.gpl','mimeType':'text/plain','buffer':b'placeholder'})
            page.wait_for_function('typeof window.releasePalette === "function"')
            field.fill('0');page.evaluate('window.releasePalette()')
            page.wait_for_function('document.querySelector("#duration").getAttribute("aria-invalid") === "true"')
            assert field.input_value() == '0' and info(page)['state']['document']['palette'] == before_palette
            page.evaluate('() => {File.prototype.arrayBuffer=window.originalArrayBuffer;}')
            field.fill('832');field.press('Tab')
            check('palette file import also refuses to apply while the document field is invalid')

            page.evaluate("""() => {window.releaseOpen=null;
              window.showOpenFilePicker=()=>new Promise(resolve=>window.releaseOpen=()=>resolve([window.openCandidate]));}""")
            page.keyboard.press('Control+o')
            page.wait_for_function('typeof window.releaseOpen === "function"')
            page.locator('#layer-properties').click()
            assert page.locator('#layer-dialog').is_visible()
            page.locator('#layer-dialog [name=name]').fill('Unapplied layer name')
            page.evaluate('window.releaseOpen()')
            page.wait_for_function('document.querySelector("#status").textContent.includes("открытый диалог")')
            assert info(page)['active'] == owner and page.locator('#layer-dialog [name=name]').input_value() == 'Unapplied layer name'
            page.locator('#layer-dialog button[value=cancel]').click()
            check('asynchronous Open cannot replace a document underneath an unapplied dialog')

            # An independent revision changing the same field must not be
            # overwritten by an older uncommitted textual edit.
            field.fill('822')
            page.evaluate("""() => JSON.parse(aegoArt.request(JSON.stringify({action:'apply',operation:{kind:'duration',durationMS:999}})))""")
            writes_before = page.evaluate('window.previousSource.writes')
            page.keyboard.press('Control+s')
            assert field.input_value() == '822' and field.get_attribute('aria-invalid') == 'true'
            assert 'изменились' in page.locator('#status').inner_text() and page.evaluate('window.previousSource.writes') == writes_before
            field.press('Escape')
            assert field.input_value() == '999' and duration(page) == 999
            check('same-field external revision conflict preserves input until explicit Escape')

            field.fill('1001')
            page.evaluate("""() => {
              window.realRequest=aegoArt.request;
              aegoArt.request=input=>{
                const req=JSON.parse(input);
                if(req.action==='apply' && req.operation?.kind==='duration') {
                  const result=JSON.parse(window.realRequest('{"action":"state"}'));result.error='mock rejected field';return JSON.stringify(result);
                }
                return window.realRequest(input);
              };
            }""")
            page.keyboard.press('Control+s')
            assert field.input_value() == '1001' and field.get_attribute('aria-invalid') == 'true'
            assert 'Не удалось применить' in page.locator('#status').inner_text() and duration(page) == 999
            assert page.evaluate('window.previousSource.writes') == writes_before
            page.evaluate('() => {aegoArt.request=window.realRequest;}')
            field.press('Tab')
            assert duration(page) == 1001
            check('rejected document operation keeps input visible and reports why Save did not run')

            # Release adapter is a classic script and shares these guards.
            page.evaluate("""() => {window.AegoStudioRelease={documentVersion:1,buildID:'test',examples:[]};
              window.AegoStudioProtocol={MAX_BYTES:67108864,VERSION:1};}""")
            page.add_script_tag(content=(ROOT/'tools/artstudio/release/web/adapter.js').read_text())
            page.wait_for_function('window.AegoStudioHost !== undefined')
            field.fill('0')
            result = page.evaluate("""() => {
              try { AegoStudioHost.importSnapshot(new Uint8Array(window.target.bytes)); return 'unexpected'; }
              catch(e) { return e.message; }
            }""")
            assert 'Длительность' in result and info(page)['active'] == owner and field.input_value() == '0'
            field.fill('901')
            captured = page.evaluate("""() => {window.transfer=AegoStudioHost.capture();return window.transfer.bytes.length}""")
            assert captured > 0 and duration(page) == 901
            reopen(page, 'window.transfer', 901)
            select(page, owner)
            check('release import refuses invalid field; transfer capture contains applied duration')

            # Layer opacity is another document field with input-before-change.
            # Use a real pointer drag and Save while the range gesture is held.
            page.evaluate("""() => {window.target=new Probe('opacity.aeart');
              window.showSaveFilePicker=async()=>window.target;}""")
            opacity = page.locator('#opacity');opacity.scroll_into_view_if_needed()
            rect = opacity.bounding_box()
            before = info(page)['state']['undo']
            page.mouse.move(rect['x']+rect['width']*0.35, rect['y']+rect['height']/2)
            page.mouse.down()
            page.mouse.move(rect['x']+rect['width']*0.45, rect['y']+rect['height']/2)
            expected_opacity = int(opacity.input_value())
            page.keyboard.press('Control+Shift+s')
            wait_saved(page, 'window.target', 1)
            page.mouse.up()
            assert info(page)['state']['undo'] == before + 1
            assert info(page)['state']['document']['layers'][0]['opacity'] == expected_opacity
            reopen(page, 'window.target', 901)
            assert info(page)['state']['document']['layers'][0]['opacity'] == expected_opacity
            check('range input saves and reopens opacity once despite its later native change event')

            select(page, owner)
            page.evaluate('window.showSaveFilePicker=undefined')
            field.fill('951')
            with page.expect_download() as downloaded:
                page.keyboard.press('Control+Shift+s')
            assert duration(page) == 951 and info(page)['state']['dirty']
            data = Path(downloaded.value.path()).read_bytes()
            count = len(info(page)['tabs'])
            page.locator('#file-input').set_input_files({'name':'downloaded.aeart','mimeType':'application/octet-stream','buffer':data})
            page.wait_for_function('(count) => document.querySelectorAll("[data-document]").length === count + 1', arg=count)
            assert duration(page) == 951
            select(page, owner)
            assert info(page)['state']['dirty']
            check('download fallback contains the focused edit but keeps its source tab dirty')

            assert not errors, errors
            print(args.engine, browser.version, 'headless; muted audio; loopback; mock handles', flush=True)
            browser.close()
    finally:
        server.shutdown()
        server.server_close()


if __name__ == '__main__':
    main()
