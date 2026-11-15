#!/usr/bin/env python3
"""Measure real UI authoring callbacks without a reference-machine PASS claim.

The editor runs unchanged over HTTP. Instrumentation only timestamps trusted
pointer/click events and forwards the original WASM and canvas calls. It does
not implement pixels, inject editor operations, or enable the GPU preview.
"""
import argparse
import hashlib
import json
import math
from pathlib import Path
import sys
import time
import traceback


INSTRUMENT = r"""() => {
  const p=window.artPerf={enabled:false,events:[],backgroundExports:[],active:null};
  const rows=new WeakMap();
  function capture(e){
    if(!p.enabled)return;
    const canvas=e.target.id==='canvas';
    const frame=e.type==='click'&&e.target.closest?.('#timeline-table button[data-frame]');
    if(!(canvas&&(['pointerdown','pointerup'].includes(e.type)||(e.type==='pointermove'&&e.buttons)))&&!frame)return;
    const row={type:frame?'frame-click':e.type,trusted:e.isTrusted,start:performance.now(),calls:[],puts:[]};
    p.events.push(row);p.active=row;rows.set(e,row);
  }
  function finished(e){const row=rows.get(e);if(row){row.handlerMS=performance.now()-row.start;if(p.active===row)p.active=null;}}
  // A browser may run a microtask checkpoint after each event listener. Keep
  // the sample through the actual target handler and end at window bubbling.
  for(const name of ['pointerdown','pointermove','pointerup','click']){
    window.addEventListener(name,capture,true);window.addEventListener(name,finished,false);
  }
  const put=CanvasRenderingContext2D.prototype.putImageData;
  CanvasRenderingContext2D.prototype.putImageData=function(...args){
    const row=p.enabled&&p.active;const start=performance.now();const result=put.apply(this,args);
    if(row&&this.canvas.id==='canvas'){
      row.puts.push(performance.now()-start);row.canvasPutMS=performance.now()-row.start;
      if(!row.rafScheduled){row.rafScheduled=true;requestAnimationFrame(()=>row.nextRAFMS=performance.now()-row.start);}
    }
    return result;
  };
  for(const name of ['request','pixels','thumbnail','exportBytes']){
    const original=aegoArt[name];
    aegoArt[name]=function(...args){
      const row=p.enabled&&p.active;const start=performance.now();const result=original.apply(this,args);
      const ms=performance.now()-start;
      if(row)row.calls.push({method:name,action:name==='request'?JSON.parse(args[0]).action:undefined,ms});
      if(name==='exportBytes')p.backgroundExports.push({start,format:args[0],ms,bytes:result.bytes?.length||0,error:!!result.error,inInputEvent:!!row});
      return result;
    };
  }
}"""


def distribution(values):
    values = sorted(values)
    if not values:
        return {'count': 0}
    return dict(count=len(values), minMS=values[0], medianMS=values[len(values)//2],
                p95MS=values[max(0, math.ceil(len(values)*.95)-1)], maxMS=values[-1],
                meanMS=sum(values)/len(values))


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--artifact', type=Path, required=True)
    parser.add_argument('--app-url', required=True)
    parser.add_argument('--browser', choices=['chromium', 'firefox'], required=True)
    parser.add_argument('--executable', default='')
    parser.add_argument('--fixture', type=Path, action='append', required=True)
    parser.add_argument('--evidence', type=Path, required=True)
    parser.add_argument('--strokes', type=int, default=12)
    args = parser.parse_args()
    if not 4 <= args.strokes <= 32:
        raise SystemExit('strokes must be between 4 and 32')
    args.evidence.mkdir(parents=True, exist_ok=False)
    manifest = (args.artifact/'manifest.json').read_bytes()
    m = json.loads(manifest)
    report = dict(status='ERROR', browser=args.browser, fixtures=[], consoleErrors=[],
                  manifestSHA256=hashlib.sha256(manifest).hexdigest(), buildID=m['buildID'],
                  sourceSHA256=m['sourceSHA256'], referenceTarget='UNVERIFIED',
                  measurementScriptSHA256=hashlib.sha256(Path(__file__).read_bytes()).hexdigest(),
                  scope='Trusted browser input to CPU canvas presentation callbacks; no physical display or input-device latency',
                  limitations=['Shared runner and concurrent workloads; no agreed reference machine.',
                               'Headless browser RAF is not physical screen presentation.',
                               'Instrumentation and DOM reads introduce small measurement overhead.',
                               'Browser performance.now precision applies; small Firefox values may be rounded to milliseconds.',
                               'No Lit/particle GPU runtime or Stage12 FPS measurement.'])
    try:
        from playwright.sync_api import sync_playwright
        with sync_playwright() as pw:
            options = {'headless': True}
            if args.executable:
                options['executable_path'] = args.executable
            if args.browser == 'chromium':
                options['args'] = ['--mute-audio', '--no-sandbox', '--disable-dev-shm-usage']
            else:
                options['firefox_user_prefs'] = {'media.volume_scale': '0.0'}
            browser = getattr(pw,args.browser).launch(**options)
            report['browserVersion'] = browser.version
            for fixture in args.fixture:
                context = browser.new_context(viewport={'width':1440,'height':1050},device_scale_factor=1)
                page = context.new_page();page.set_default_timeout(60000)
                page.on('pageerror',lambda error:report['consoleErrors'].append(str(error)))
                page.on('dialog',lambda dialog:dialog.accept() if dialog.type in ('confirm','beforeunload') else dialog.dismiss())
                wasm = []
                page.on('response',lambda response:wasm.append(response.url) if response.url.endswith('.wasm') else None)
                page.goto(args.app_url.rstrip('/')+'/editor/?lang=en',wait_until='load')
                page.wait_for_function('()=>window.artStudioSurfaceReady===true')
                page.evaluate(INSTRUMENT)
                old = page.evaluate('()=>JSON.parse(aegoArt.request(\'{"action":"state"}\')).state.document.id')
                started = time.perf_counter()
                page.locator('#file-input').set_input_files(fixture)
                page.wait_for_function('(old)=>JSON.parse(aegoArt.request(\'{"action":"state"}\')).state.document.id!==old',arg=old)
                import_ms = (time.perf_counter()-started)*1000
                state = page.evaluate('()=>JSON.parse(aegoArt.request(\'{"action":"state"}\')).state')
                info = state['document'];assert info['canvas']['width']==256 and info['canvas']['height']==256
                page.locator('#fit').click()
                page.locator('[data-tool=pencil]').click()
                page.locator('#brush-size').fill('1')
                page.locator('#hex-color').fill('#16a5e8');page.locator('#hex-color').press('Tab')
                box = page.locator('#canvas').bounding_box()
                assert box and box['width'] >= 256 and box['height'] >= 256
                page.evaluate('()=>{artPerf.events=[];artPerf.enabled=true}')
                for i in range(args.strokes):
                    row=180+i
                    start=(box['x']+8.5*box['width']/256,box['y']+(row+.5)*box['height']/256)
                    end=(box['x']+247.5*box['width']/256,start[1])
                    page.mouse.move(*start);page.mouse.down();page.mouse.move(*end,steps=8);page.mouse.up()
                page.wait_for_timeout(100)
                page.evaluate('()=>artPerf.enabled=false')
                painted = page.evaluate('()=>JSON.parse(aegoArt.request(\'{"action":"state"}\')).state')
                assert painted['undo']==state['undo']+args.strokes, 'one-stroke-one-Undo was lost'
                pixels = page.evaluate('(n)=>{const data=aegoArt.pixels("",false,false);let wrong=0;for(let y=180;y<180+n;y++)for(let x=8;x<=247;x++){const i=(y*256+x)*4;if(data[i]!==22||data[i+1]!==165||data[i+2]!==232||data[i+3]!==255)wrong++;}return {wrong,bytes:data.length}}',args.strokes)
                assert pixels['wrong']==0, 'trusted fast strokes left wrong or missing pixels'
                page.evaluate('()=>artPerf.enabled=true')
                for frame in range(1,min(9,len(info['frames']))):
                    page.locator('#timeline-table thead button').nth(frame).click()
                page.wait_for_timeout(100)
                page.evaluate('()=>artPerf.enabled=false')
                events = page.evaluate('()=>artPerf.events')
                exports = page.evaluate('()=>artPerf.backgroundExports')
                assert events and all(event['trusted'] for event in events)
                for kind in ('pointerdown','pointermove','pointerup'):
                    measured=[event for event in events if event['type']==kind]
                    assert measured and all('canvasPutMS' in event and 'handlerMS' in event and 'nextRAFMS' in event for event in measured), 'incomplete timing instrumentation: '+kind
                assert len(wasm)==1, 'authoring performance loaded another/GPU runtime'
                metrics={}
                for kind in ('pointerdown','pointermove','pointerup','frame-click'):
                    rows=[event for event in events if event['type']==kind]
                    metrics[kind]={field:distribution([row[field] for row in rows if field in row]) for field in ('canvasPutMS','handlerMS','nextRAFMS')}
                    methods={}
                    for method in ('request','pixels','thumbnail'):
                        methods[method]=distribution([call['ms'] for row in rows for call in row['calls'] if call['method']==method])
                    metrics[kind]['coreCalls']=methods
                name=fixture.stem
                (args.evidence/(name+'-events.json')).write_text(json.dumps(events,indent=2)+'\n')
                page.screenshot(path=str(args.evidence/(name+'.png')))
                report['fixtures'].append(dict(name=name,SHA256=hashlib.sha256(fixture.read_bytes()).hexdigest(),
                    width=256,height=256,layers=len(info['layers']),frames=len(info['frames']),cels=len(info['cels']),
                    importWallMS=import_ms,strokes=args.strokes,oneUndoPerStroke=True,missingPixels=0,metrics=metrics))
                report['fixtures'][-1]['backgroundExports'] = exports
                report['fixtures'][-1]['recoveryEncode'] = distribution([value['ms'] for value in exports if value['format']=='recovery'])
                print(json.dumps(report['fixtures'][-1],indent=2),flush=True)
                context.close()
            browser.close()
        assert not report['consoleErrors']
        report['status']='MEASURED'
        return 0
    except BaseException as error:
        report['error']=str(error);report['traceback']=traceback.format_exc()
        print(report['traceback'],file=sys.stderr)
        return 1
    finally:
        (args.evidence/'performance.json').write_text(json.dumps(report,indent=2)+'\n')


if __name__ == '__main__':
    sys.exit(main())
