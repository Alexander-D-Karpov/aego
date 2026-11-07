"""Exercise the real shipped UI + Go/WASM without external network access.
By default, use the real local HTTP host started with go run ./tools/artstudio.
--inject is a diagnostic fallback for environments that block URL navigation.
OS downloads and IndexedDB persistence are not covered by this smoke suite.
"""
from pathlib import Path
from playwright.sync_api import sync_playwright
import argparse, base64, json, re, shutil, subprocess
parser = argparse.ArgumentParser(description=__doc__)
parser.add_argument('--root', type=Path, default=Path(__file__).resolve().parents[3])
parser.add_argument('--output', type=Path)
parser.add_argument('--url', default='http://127.0.0.1:8097')
parser.add_argument('--chromium', default=shutil.which('chromium'))
parser.add_argument('--inject', action='store_true', help='diagnostic fallback: not an HTTP/CSP acceptance test')
parser.add_argument('--no-sandbox', action='store_true', help='only for a trusted isolated root test container')
args = parser.parse_args()
ROOT = args.root.resolve()
BUILD = ROOT / '.cache/artstudio'
OUT = (args.output or ROOT / '.cache/artstudio-evidence').resolve()
OUT.mkdir(parents=True, exist_ok=True)
checks=[]
def passed(name, **details):
    checks.append(dict(name=name,status='PASS',**details));print('PASS',name,details,flush=True)
def install(page):
    if not args.inject:
        page.goto(args.url, wait_until='networkidle')
        page.wait_for_function('window.artStudioReady === true', timeout=15000)
        return
    html=re.sub(r'<script\b[^>]*src=[^>]*>\s*</script>','',(BUILD/'index.html').read_text())
    html=re.sub(r'<link\b[^>]*href="studio.css"[^>]*>','',html)
    page.set_content(html)
    page.add_style_tag(content=(BUILD/'studio.css').read_text())
    page.evaluate('b64=>{const data=Uint8Array.from(atob(b64),x=>x.charCodeAt(0));window.fetch=async url=>{if(url!=="artstudio.wasm")throw Error("unexpected network");return new Response(data,{headers:{"Content-Type":"application/wasm"}})};}',base64.b64encode((BUILD/'artstudio.wasm').read_bytes()).decode())
    page.add_script_tag(content=(BUILD/'wasm_exec.js').read_text())
    page.add_script_tag(content=(BUILD/'recovery.js').read_text())
    page.add_script_tag(content=(BUILD/'studio.js').read_text())
    page.wait_for_function('window.artStudioReady === true',timeout=10000)
def request(page,req):
    result=page.evaluate('r=>JSON.parse(aegoArt.request(JSON.stringify(r)))',req)
    assert not result.get('error'),result
    return result

def state(page):return request(page,{'action':'state'})['state']
def hash_(page,frame=''):return page.evaluate('f=>aegoArt.hash(f)',frame)
def xy(page,x,y):
    b=page.locator('#canvas').bounding_box()
    c=state(page)['document']['canvas']
    return b['x']+(x+0.5)*b['width']/c['width'], b['y']+(y+0.5)*b['height']/c['height']
def drag(page,a,b,steps=4):
    page.mouse.move(*xy(page,*a));page.mouse.down();page.mouse.move(*xy(page,*b),steps=steps);page.mouse.up()
def color(page,value):
    page.locator('#hex-color').fill(value);page.locator('#hex-color').press('Tab')
def tool(page,name):page.locator('[data-tool="'+name+'"]').click()
def new(page,w=32,h=32,mode='rgba'):
    page.locator('#new').click();page.locator('#new-dialog [name=width]').fill(str(w));page.locator('#new-dialog [name=height]').fill(str(h));page.locator('#new-dialog [name=mode]').select_option(mode);page.locator('#new-dialog button[value=create]').click();page.wait_for_timeout(50)

def tab(page,key):
    page.locator(f'[data-document="{key}"]').click()
def cell(page,frame,layer=None):
    selector=f'#timeline-table tbody button[data-frame="{frame}"]'
    if layer: selector+=f'[data-layer="{layer}"]'
    return page.locator(selector).first

def authoring_extensions(page):
    # Free tab slots through the actual close buttons before opening test projects.
    for key in [v['key'] for v in request(page,{'action':'state'})['tabs'][:-1]]:
        page.locator(f'[data-close-document="{key}"]').click()
    new(page)
    first=request(page,{'action':'state'})['active']
    tool(page,'pencil');color(page,'#ff696e');page.mouse.click(*xy(page,4,4))
    original=hash_(page); undo=state(page)['undo']
    page.locator('#zoom-in').click() if page.locator('#zoom-in').count() else None
    zoom=page.locator('#zoom').inner_text()
    new(page)
    second=request(page,{'action':'state'})['active'];blank=hash_(page)
    assert blank!=original and state(page)['undo']==0
    tab(page,first)
    assert hash_(page)==original and state(page)['undo']==undo and page.locator('#zoom').inner_text()==zoom
    page.locator('#undo').click();assert hash_(page)==blank
    tab(page,second);assert state(page)['undo']==0
    tab(page,first);page.locator('#redo').click();assert hash_(page)==original
    passed('Document tabs preserve independent pixels, history and view')
    # A linked pair plus an empty target; range copy snapshots overlapping sources.
    page.locator('#link-frame').click();page.locator('#add-frame').click()
    info=state(page)['document'];frames=[f['id'] for f in info['frames']];layer=info['layers'][0]['id']
    cell(page,frames[0],layer).click();cell(page,frames[1],layer).click(modifiers=['Shift'])
    assert page.locator('#timeline-table tbody button[aria-selected=true]').count()==2
    before=state(page)['undo']; old=info['cels']
    page.locator('#range-transfer').click()
    page.locator('#range-dialog [name=operation]').select_option('copy')
    page.locator('#range-dialog [name=overwrite]').check()
    page.locator('#range-dialog button[value=apply]').click();page.wait_for_timeout(60)
    info=state(page)['document'];cels={c['frame']:c for c in info['cels']}
    assert state(page)['undo']==before+1, state(page)
    assert cels[frames[1]]['image']==cels[frames[2]]['image']!=cels[frames[0]]['image']
    page.locator('#undo').click();assert state(page)['document']['cels']==old
    page.locator('#redo').click()
    passed('Timeline Shift selection and overlapping independent range copy undo links as one operation')
    # Copy both linked copied frames to a different live document.
    cell(page,frames[1],layer).click();cell(page,frames[2],layer).click(modifiers=['Control'])
    page.locator('#copy-animation').click();tab(page,second)
    before=state(page)['undo'];page.locator('#paste-animation').click()
    info=state(page)['document'];assert len(info['frames'])==3 and len(info['layers'])==2, info
    pasted=[c for c in info['cels'] if c['layer']==info['layers'][-1]['id']]
    assert len(pasted)==2 and pasted[0]['image']==pasted[1]['image']
    assert state(page)['undo']==before+1
    page.locator('#undo').click();assert len(state(page)['document']['frames'])==1
    page.locator('#redo').click();assert len(state(page)['document']['frames'])==3
    passed('Cross-tab animation copy preserves cel links and uses one reversible command')
    # Explicit mode conversion, palette reorder, and intentional remap differ.
    new(page,32,32,'indexed');tool(page,'pencil')
    page.locator('#swatches button').nth(2).click();page.mouse.click(*xy(page,5,5))
    before=hash_(page);n=len(state(page)['document']['palette']);order=list(range(n));order[1],order[2]=order[2],order[1]
    page.locator('#palette-order').click();page.locator('#palette-order-dialog [name=order]').fill(','.join(map(str,order)))
    page.locator('#palette-order-dialog button[value=apply]').click();page.wait_for_timeout(60)
    assert hash_(page)==before
    page.locator('#palette-order').click();page.locator('#palette-order-dialog [name=order]').fill(','.join(map(str,order)));page.locator('#palette-order-dialog [name=recolor]').check()
    page.locator('#palette-order-dialog button[value=apply]').click();page.wait_for_timeout(60)
    assert hash_(page)!=before
    page.locator('#undo').click();assert hash_(page)==before
    passed('Indexed palette reorder preserves pixels; explicit remap recolors and Undo restores')
    # Import a real GPL through the file input. RGBA pixels remain unchanged.
    page.locator('#mode').select_option('rgba');before=hash_(page)
    page.locator('#palette-input').set_input_files({'name':'test.gpl','mimeType':'text/plain','buffer':b'GIMP Palette\nName: Test\nColumns: 2\n#\n255 0 0 Red\n0 0 255 Blue\n'})
    page.wait_for_function('JSON.parse(aegoArt.request(JSON.stringify({action:"state"}))).state.document.palette.length===3')
    assert hash_(page)==before and state(page)['document']['palette'][1]['name']=='Red'
    passed('Palette file input uses Go GPL decoder without silently recoloring RGBA')
    # A modal captured before an agent edit cannot overwrite the new revision.
    page.locator('#palette-extract').click();before=state(page)['document']['revision']
    request(page,{'action':'apply','operation':{'kind':'duration','durationMS':177}})
    page.locator('#palette-dialog button[value=apply]').click();page.wait_for_timeout(60)
    assert state(page)['document']['revision']==before+1
    assert 'изменился' in page.locator('#status').inner_text()
    passed('Versioned modal rejects an intervening agent edit')
    # Use a controlled file-system API mock, not an OS-write claim.
    # Picker/write promises complete after switching tabs and editing again.
    page.evaluate("""() => {
      Object.defineProperty(window,'isSecureContext',{configurable:true,value:true});
      window.createCalls=0;window.written=null;
      window.showSaveFilePicker=()=>new Promise(resolve=>{window.chooseFile=()=>resolve({name:'Saved-original.aeart',createWritable:async()=>{window.createCalls++;return {write:async bytes=>{window.written=bytes},close:()=>new Promise(r=>window.finishWrite=r),abort:async()=>{}}}})});
    }""")
    # Synchronize stale presentation after the injected intervening edit.
    page.locator('[data-document]').last.click()
    owner=request(page,{'action':'state'})['active'];page.locator('#save').click();page.evaluate('window.chooseFile()')
    page.wait_for_function('typeof window.finishWrite==="function"')
    page.locator(f'[data-close-document="{owner}"]').click();assert any(t['key']==owner for t in request(page,{'action':'state'})['tabs'])
    tool(page,'pencil');color(page,'#24a874');page.mouse.click(*xy(page,12,12));after_edit=hash_(page)
    tab(page,first);page.evaluate('window.finishWrite()');page.wait_for_timeout(60)
    response=request(page,{'action':'state'});owner_tab=next(t for t in response['tabs'] if t['key']==owner)
    assert response['active']==first and owner_tab['dirty'] and owner_tab['name']=='Saved-original.aeart', response
    tab(page,owner);assert hash_(page)==after_edit
    passed('Async save completion belongs to original tab, pins close, and keeps later edits dirty (writer mock)')
    page.locator('#save').click();page.mouse.click(*xy(page,13,13));page.evaluate('window.chooseFile()');page.wait_for_timeout(60)
    assert page.evaluate('window.createCalls')==1 and state(page)['dirty']
    passed('Stale file-picker confirmation is rejected before opening the writer (writer mock)')
    page.screenshot(path=str(OUT/'studio-documents-ranges-palettes.png'),full_page=True)

def run():
 with sync_playwright() as p:
  browser=p.chromium.launch(executable_path=args.chromium,headless=True,chromium_sandbox=not args.no_sandbox,args=['--no-sandbox'] if args.no_sandbox else [])
  context=browser.new_context(viewport={'width':1440,'height':1000},has_touch=True)
  page=context.new_page();errors=[]
  page.on('pageerror',lambda e:errors.append(str(e)))
  page.on('dialog',lambda d:d.accept('Effects' if d.type=='prompt' else None))
  install(page);passed('WASM boot and UI initialization')
  new(page);assert state(page)['document']['canvas']['width']==32;passed('New document dialog')
  tool(page,'rectangle');page.locator('#filled').check();color(page,'#4dc3d1')
  before=hash_(page);drag(page,(8,10),(23,24));s=state(page);assert s['undo']==1 and s['dirty'];after=hash_(page);assert after!=before
  page.locator('#undo').click();assert hash_(page)==before
  page.locator('#redo').click();assert hash_(page)==after
  passed('Shape pointer gesture is exactly one reversible history entry')
  tool(page,'ellipse');color(page,'#ffdb6e');drag(page,(9,4),(22,16))
  tool(page,'pencil');color(page,'#181c2a');page.mouse.click(*xy(page,13,10));page.mouse.click(*xy(page,18,10))
  tool(page,'line');drag(page,(14,13),(17,13))
  color(page,'#8b74dd');drag(page,(9,25),(12,28));drag(page,(20,25),(23,28))
  base=hash_(page);undo=state(page)['undo'];revision=state(page)['document']['revision']
  tool(page,'rectangle');page.mouse.move(*xy(page,0,0));page.mouse.down();page.mouse.move(*xy(page,31,31),steps=4);page.keyboard.press('Escape');page.mouse.up()
  assert hash_(page)==base and state(page)['undo']==undo and state(page)['document']['revision']==revision
  passed('Esc cancels unpublished gesture without history pollution')
  tool(page,'pencil');color(page,'#f0f4f8');undo=state(page)['undo'];drag(page,(4,30),(27,30),steps=1)
  pixels=page.evaluate('Array.from(aegoArt.pixels("",false,false))')
  assert all(pixels[(30*32+x)*4+3]==255 for x in range(4,28))
  assert state(page)['undo']==undo+1;page.locator('#undo').click()
  passed('Sparse mouse events produce an unbroken stroke')
  page.locator('#link-frame').click();s=state(page);first,second=[f['id'] for f in s['document']['frames']];assert s['document']['cels'][0]['image']==s['document']['cels'][1]['image']
  tool(page,'pencil');page.mouse.click(*xy(page,15,6));assert hash_(page,first)==hash_(page,second)
  page.locator('#unlink').click();s=state(page);assert s['document']['cels'][0]['image']!=s['document']['cels'][1]['image']
  color(page,'#ff696e');page.mouse.click(*xy(page,15,6));assert hash_(page,first)!=hash_(page,second)
  page.locator('#undo').click();page.locator('#undo').click();s=state(page);assert s['document']['cels'][0]['image']==s['document']['cels'][1]['image']
  page.locator('#redo').click();page.locator('#redo').click();passed('Linked cels, independent unlink, pixels and link topology Undo/Redo')
  for i in range(6):
   page.locator('#duplicate-frame').click();tool(page,'move');drag(page,(15,15),(15,14 if i%2==0 else 16),steps=2)
  assert len(state(page)['document']['frames'])==8
  page.locator('#duration').fill('80');page.locator('#duration').press('Tab');assert state(page)['document']['frames'][-1]['durationMS']==80
  page.locator('#add-tag').click();page.locator('#tag-dialog [name=name]').fill('Bounce');page.locator('#tag-dialog [name=to]').fill('8');page.locator('#tag-dialog [name=direction]').select_option('ping-pong');page.locator('#tag-dialog button[value=create]').click();page.wait_for_timeout(50)
  assert state(page)['document']['tags'][0]['name']=='Bounce'
  revision=state(page)['document']['revision'];page.locator('#play').click();page.wait_for_timeout(250);assert page.locator('#play').get_attribute('class')=='active';page.locator('#play').click();assert state(page)['document']['revision']==revision
  page.locator('#onion').check();assert state(page)['document']['revision']==revision
  page.locator('[data-workspace=animation]').click();assert 'workspace-animation' in page.locator('body').get_attribute('class');page.locator('[data-workspace=art]').click();assert state(page)['document']['revision']==revision
  passed('Eight frames, variable duration, ping-pong tag, playback and workspace state preservation')
  page.locator('#add-layer').click();page.locator('#layer-dialog [name=name]').fill('Effects');page.locator('#layer-dialog button[value=apply]').click();page.wait_for_timeout(50);s=state(page);assert len(s['document']['layers'])==2
  tool(page,'pencil');color(page,'#ffae5b');page.mouse.click(*xy(page,4,5));assert state(page)['dirty'];page.locator('#layers .layer-row').first.locator('.lock').click();revision=state(page)['document']['revision'];page.mouse.click(*xy(page,3,5));assert state(page)['document']['revision']==revision;page.locator('#layers .layer-row').first.locator('.lock').click()
  passed('Layer creation and locked-layer input rejection')
  page.screenshot(path=str(OUT/'studio-animation-1440.png'),full_page=True)
  original=hash_(page);saved_frame=state(page)['frame'];page.evaluate('window.savedArt=aegoArt.exportBytes("aeart").bytes;window.savedPNG=aegoArt.exportBytes("png").bytes')
  sheet=page.evaluate('({png:Array.from(aegoArt.exportBytes("sheet",4).bytes),meta:new TextDecoder().decode(aegoArt.exportBytes("sheet-json",4).bytes)})')
  assert bytes(sheet['png']).startswith(b'\x89PNG');meta=json.loads(sheet['meta']);assert len(meta['frames'])==8
  result=page.evaluate('JSON.parse(aegoArt.importBytes(window.savedArt,"aeart",true))');assert not result.get('error') and len(result['state']['document']['frames'])==8, result
  request(page,{'action':'select','frame':saved_frame});assert hash_(page)==original
  result=page.evaluate('JSON.parse(aegoArt.importBytes(window.savedPNG,"png",true))');assert not result.get('error') and len(result['state']['document']['frames'])==1 and hash_(page)==original
  passed('Editable source / PNG byte round-trip and spritesheet export')
  # Re-sync presentation using an ordinary UI action after bridge-level import.
  page.locator('#language').click();assert page.locator('#new').inner_text()=='New'
  page.locator('#language').click();assert page.locator('#new').inner_text()=='Создать';passed('RU/EN UI switch')
  new(page,32,32,'indexed');tool(page,'pencil');before=state(page)['undo'];page.touchscreen.tap(*xy(page,3,3));assert state(page)['undo']==before+1
  passed('Simulated touch pointer input in Chromium')
  cdp=context.new_cdp_session(page);x,y=xy(page,7,7);before=state(page)['undo']
  cdp.send('Input.dispatchMouseEvent',{'type':'mousePressed','x':x,'y':y,'button':'left','buttons':1,'pointerType':'pen','clickCount':1})
  cdp.send('Input.dispatchMouseEvent',{'type':'mouseReleased','x':x,'y':y,'button':'left','buttons':0,'pointerType':'pen','clickCount':1})
  assert state(page)['undo']==before+1;passed('Simulated pen pointer input in Chromium')
  page.set_viewport_size({'width':390,'height':844});page.wait_for_timeout(50)
  assert page.evaluate('document.documentElement.scrollWidth<=window.innerWidth')
  page.locator('#toggle-side').click();assert page.locator('#sidebar').is_visible();page.screenshot(path=str(OUT/'studio-mobile-390.png'),full_page=True)
  page.locator('#toggle-side').click();page.set_viewport_size({'width':1440,'height':1000});page.locator('#fit').click()
  passed('Narrow layout, sidebar access and resize')
  authoring_extensions(page)
  # Identical typed commands replay on native CPU and in the real browser WASM.
  fixture={'width':32,'height':32,'mode':'rgba','operations':[{'kind':'rectangle','filled':True,'points':[{'X':2,'Y':3},{'X':23,'Y':17}],'brush':{'size':1,'paint':{'color':{'R':255,'G':105,'B':110,'A':255}}}},{'kind':'ellipse','filled':True,'points':[{'X':7,'Y':7},{'X':28,'Y':28}],'brush':{'size':1,'paint':{'color':{'R':77,'G':195,'B':209,'A':255}}}},{'kind':'duplicate-frame','linked':True},{'kind':'unlink'},{'kind':'flip-x'},{'kind':'mode','mode':'indexed'}]}
  fixture_path=OUT/'parity-replay.json';fixture_path.write_text(json.dumps(fixture,indent=2))
  native=json.loads(subprocess.check_output([str(ROOT/'.cache/artstudio-host'),'-replay',str(fixture_path)],text=True))
  request(page,{'action':'new','width':32,'height':32,'mode':'rgba','confirm':True})
  for op in fixture['operations']:request(page,{'action':'apply','operation':op})
  assert hash_(page)==native['pixelSHA256'];passed('Native CPU / Chromium WASM pixel parity',sha256=native['pixelSHA256'])
  for _ in range(30):
   res=request(page,{'action':'new','width':32,'height':32,'mode':'rgba','confirm':True})
   request(page,{'action':'document-close','handle':res['active'],'confirm':True})
  request(page,{'action':'new','width':32,'height':32,'mode':'rgba','confirm':True})
  assert state(page)['undo']==0 and len(state(page)['document']['frames'])==1
  passed('Thirty document open/close cycles without stale state')
  assert not errors,errors;passed('No JavaScript page errors')
  result={'engine':browser.version,'resourceMode':'injected local resources; HTTP/CSP is NOT covered' if args.inject else 'HTTP navigation to '+args.url,'diagnosticMode':args.inject,'checks':checks,'pageErrors':errors,'notCovered':['native Aego Editor integration','Firefox','WebKit/Safari',*(['HTTP origin/CSP in a browser'] if args.inject else []),'IndexedDB recovery persistence','OS downloads','native GPU-resource lifetime']}
  (OUT/'browser-e2e.json').write_text(json.dumps(result,ensure_ascii=False,indent=2))
  browser.close()
if __name__=='__main__':run()
