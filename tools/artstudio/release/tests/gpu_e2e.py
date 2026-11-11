#!/usr/bin/env python3
"""Real WebGL2 presentation acceptance on the same immutable Studio artifact.
No mock canvas, shader fixture substitution or updated golden images.
"""
from __future__ import annotations
import argparse
import base64
import hashlib
import json
import os
from pathlib import Path
import sys
import traceback


def main():
    if not __debug__:
        raise RuntimeError("GPU acceptance must not run with Python optimization")
    p = argparse.ArgumentParser()
    p.add_argument('--artifact', required=True, type=Path)
    p.add_argument('--app-url', required=True)
    p.add_argument('--browser', choices=['chromium', 'firefox'], required=True)
    p.add_argument('--executable', default='')
    p.add_argument('--headed', action='store_true')
    p.add_argument('--evidence', required=True, type=Path)
    args = p.parse_args()
    args.evidence.mkdir(parents=True, exist_ok=False)
    raw = (args.artifact/'manifest.json').read_bytes()
    m = json.loads(raw)
    report = dict(status='FAIL', browser=args.browser, manifestSHA256=hashlib.sha256(raw).hexdigest(),
                  sourceSHA256=m['sourceSHA256'], buildID=m['buildID'], checks=[], consoleErrors=[],
                  transport='real-http-webgl2', resourceCycles=[])
    def checked(name):
        report['checks'].append(name)
        print('PASS', name, flush=True)
    try:
        from playwright.sync_api import sync_playwright
        with sync_playwright() as pw:
            options = dict(headless=not args.headed)
            if args.executable:
                options['executable_path'] = args.executable
            if args.browser == 'chromium':
                options['args'] = ['--mute-audio', '--no-sandbox', '--disable-dev-shm-usage', '--use-gl=angle', '--use-angle=swiftshader', '--enable-unsafe-swiftshader']
            else:
                options['firefox_user_prefs'] = {'webgl.force-enabled': True, 'webgl.disabled': False, 'media.volume_scale': '0.0'}
                if os.environ.get('MOZ_DISABLE_CONTENT_SANDBOX') == '1':
                    options['firefox_user_prefs']['security.sandbox.content.level'] = 0
            browser = getattr(pw,args.browser).launch(**options)
            report['browserVersion'] = browser.version
            ctx = browser.new_context(viewport={'width':1440,'height':1050},device_scale_factor=2)
            page = ctx.new_page()
            page.on('pageerror',lambda e:report['consoleErrors'].append(str(e)))
            page.on('dialog',lambda d:d.accept() if d.type in ('confirm','beforeunload') else d.dismiss())
            responses = []
            page.on('response',lambda r:responses.append(r) if r.url.endswith('.wasm') else None)
            page.goto(args.app_url.rstrip('/')+'/editor/?new=sprite&lang=en',wait_until='load')
            page.wait_for_function('() => window.artStudioSurfaceReady === true',timeout=60000)
            assert m.get('previewWasm'), 'GPU runtime absent from manifest'
            assert len(responses)==1, 'GPU runtime must not load before a GPU mode is requested'
            page.locator('#new-dialog [name=width]').fill('32')
            page.locator('#new-dialog [name=height]').fill('32')
            old_handle = page.evaluate('() => AegoStudioHost.summary().handle')
            page.locator('#new-dialog [value=create]').click()
            page.wait_for_function('(old) => AegoStudioHost.summary().handle !== old',arg=old_handle)
            def state():
                return page.evaluate('() => JSON.parse(aegoArt.request(\'{"action":"state"}\'))')
            def status():
                return page.evaluate('() => JSON.parse(aegoArtPreview.status()).status')
            def ready(mode):
                page.wait_for_function('(mode)=>{if(!window.aegoArtPreview)return false;const s=JSON.parse(aegoArtPreview.status()).status;return s.state==="ready"&&s.mode===mode&&s.stats.MaterialPending===0}',arg=mode,timeout=60000)
            def paint(x,y,x2=None,y2=None,tool='pencil'):
                page.locator(f'[data-tool="{tool}"]').click()
                b=page.locator('#canvas').bounding_box()
                size=state()['state']['document']['canvas']
                sx,sy=b['width']/size['width'],b['height']/size['height']
                page.mouse.move(b['x']+(x+.5)*sx,b['y']+(y+.5)*sy)
                page.mouse.down()
                if x2 is not None:page.mouse.move(b['x']+(x2+.5)*sx,b['y']+(y2+.5)*sy,steps=12)
                page.mouse.up()
            def capture(name):
                image=page.locator('#gpu-preview').screenshot(path=str(args.evidence/(name+'.png')))
                return hashlib.sha256(image).hexdigest()
            def capture_pixels(name):
                digest=capture(name)
                # Decode the actual browser screenshot for numerical pixel
                # assertions. This canvas is a test-side PNG reader, never a
                # substitute preview renderer or an authoring implementation.
                page.evaluate('''async ({name,png}) => {
                  const bytes=Uint8Array.from(atob(png),c=>c.charCodeAt(0));
                  const image=await createImageBitmap(new Blob([bytes],{type:'image/png'}));
                  const reader=document.createElement('canvas');
                  reader.width=image.width;reader.height=image.height;
                  const context=reader.getContext('2d',{willReadFrequently:true});
                  context.drawImage(image,0,0);image.close();
                  window.artGPUScreens ??= {};
                  window.artGPUScreens[name]=context.getImageData(0,0,reader.width,reader.height);
                }''',dict(name=name,png=base64.b64encode((args.evidence/(name+'.png')).read_bytes()).decode('ascii')))
                return digest
            def settle_gpu():
                # Input publication and the real renderer have separate RAF
                # callbacks. Wait for completed frames after the UI change.
                page.evaluate('''() => new Promise(resolve=>{
                  let left=4;function frame(){if(--left===0)resolve();else requestAnimationFrame(frame);}
                  requestAnimationFrame(frame);
                })''')
                ready('lit')
            def emission_strength(key):
                page.locator('#gpu-emission').press(key)
                settle_gpu()
            def select(mode):
                page.locator('#gpu-mode').select_option(mode)
                ready(mode)
            def sample_shadows():
                # Renderer counters describe one frame. Observe before the
                # input: checking only after a locator click can miss the sole
                # dirty-mask frame and then see only valid cache hits.
                page.evaluate('''() => {
                  window.artShadowSamples=[];
                  const token={};window.artShadowSampling=token;
                  function sample(){
                    if(window.artShadowSampling!==token)return;
                    const s=JSON.parse(aegoArtPreview.status()).status;
                    artShadowSamples.push({revision:s.revision,sequence:s.sequence,
                      draws:s.stats.ShadowDraws,dirty:s.stats.DirtyShadowMasks,hits:s.stats.ShadowCacheHits});
                    if(artShadowSamples.length<240)requestAnimationFrame(sample);
                  }
                  requestAnimationFrame(sample);
                }''')
            def shadow_result(revision):
                page.wait_for_function('(revision) => artShadowSamples.some(s=>s.revision===revision&&s.draws>0&&s.dirty>0)',arg=revision,timeout=30000)
                page.wait_for_function('() => JSON.parse(aegoArtPreview.status()).status.stats.ShadowCacheHits>0',timeout=30000)
                return page.evaluate('() => {window.artShadowSampling=null;return artShadowSamples;}')
            def configure_surface_pixels():
                # Keep the existing scenario and its controls intact. Author a
                # separate real document while GPU preview is paused, publish
                # it once, then exercise presentation-only selections. Direct
                # channel views use Unlit; this is not a height-lighting test.
                def snapshot():
                    return page.evaluate('''async () => {
                      const s=aegoArt.snapshotBytes();if(s.error)throw Error(s.error);
                      const digest=await crypto.subtle.digest('SHA-256',s.bytes);
                      return {handle:s.handle,revision:s.revision,size:s.bytes.length,
                        sha256:[...new Uint8Array(digest)].map(x=>x.toString(16).padStart(2,'0')).join('')};
                    }''')
                original_state=state()['state'];original_snapshot=snapshot()
                original_controls=page.evaluate('''() => ({
                  tool:document.querySelector('[data-tool].active').dataset.tool,
                  color:document.getElementById('fg-color').value,
                  alpha:document.getElementById('fg-alpha').value,
                  size:document.getElementById('brush-size').value
                })''')
                original_sequence=status()['sequence']
                original_resources=page.evaluate('() => JSON.parse(aegoArtPreview.resources())')
                page.locator('#gpu-mode').select_option('pixels')
                page.locator('#new').click()
                page.locator('#new-dialog [name=width]').fill('32')
                page.locator('#new-dialog [name=height]').fill('32')
                page.locator('#new-dialog [value=create]').click()
                page.wait_for_function('(old)=>AegoStudioHost.summary().handle!==old',arg=original_snapshot['handle'])
                page.locator('#fg-alpha').fill('255')
                page.locator('#fg-alpha').press('Tab')
                channels=('albedo','normal','height','emission')
                # Endpoints have the same expected displayed values under
                # both sRGB color and linear NonColor texture preparation.
                # Normal vectors are nonzero; the raw diagnostic view does
                # not apply the normalization performed by the Lit shader.
                colors=(('#ff0000','#00ffff','#ffffff','#ffff00'),
                        ('#0000ff','#00ff00','#000000','#ff00ff'))
                witness=dict(status='RUNNING',scope='Unlit frame and raw channel selection',
                    generationEvidence='TestConfigureChangesCachedFrameAndChannelRegion checks the unexposed loader generation',
                    pixelTolerance=2,fixture=[],observations=[])
                report['configureSurfacePixels']=witness
                frames=[]
                for fi in range(2):
                    if fi:
                        previous=state()['state']['frame']
                        page.locator('#add-frame').click()
                        page.wait_for_function('(old)=>JSON.parse(aegoArt.request(\'{"action":"state"}\')).state.frame!==old',arg=previous)
                    frames.append(state()['state']['frame'])
                    for channel,hex_color in zip(channels,colors[fi]):
                        page.locator('#channel').select_option(channel)
                        page.locator('#fg-color').fill(hex_color)
                        paint(16,16,tool='fill')
                        expected=[int(hex_color[i:i+2],16) for i in (1,3,5)]+[255]
                        canonical=page.evaluate('''expected => {
                          const p=aegoArt.pixels('',false,false);let matching=0;
                          for(let i=0;i<p.length;i+=4)if(expected.every((v,k)=>p[i+k]===v))matching++;
                          return {pixels:p.length/4,matching,sha256:aegoArt.hash()};
                        }''',expected)
                        witness['fixture'].append(dict(frame=frames[-1],channel=channel,expected=expected,canonical=canonical))
                        assert canonical['pixels']==1024 and canonical['matching']==1024, ('Canonical channel fixture differs from its intended opaque RGBA',fi,channel,canonical,expected)
                page.locator('#channel').select_option('albedo')
                page.locator('#first-frame').click()
                fixture_state=state()['state'];fixture_snapshot=snapshot()
                select('sprite')
                page.wait_for_function('''expected => {
                  const s=JSON.parse(aegoArtPreview.status()).status;
                  return s.document===expected.id&&s.revision===expected.revision&&s.frame===expected.frame;
                }''',arg=dict(id=fixture_state['document']['id'],revision=fixture_snapshot['revision'],frame=frames[0]))
                published=status()
                witness['snapshot']=fixture_snapshot
                witness['publicationSequence']=published['sequence']
                witness['renderer']=published['renderer']
                assert published['sequence']==original_sequence+1, 'Fixture selection published more than its final document snapshot'
                for mode,ci in (('sprite',0),('normal',1),('height',2),('emission',3)):
                    select(mode)
                    first_pixels=None
                    for visit,fi in enumerate((0,1,0)):
                        page.locator('#first-frame' if fi==0 else '#next-frame').click()
                        page.wait_for_function('''expected => {
                          const s=JSON.parse(aegoArtPreview.status()).status;
                          return s.state==='ready'&&s.frame===expected.frame&&s.mode===expected.mode&&s.stats.MaterialPending===0;
                        }''',arg=dict(frame=frames[fi],mode=mode),timeout=30000)
                        # Allow completed render frames, not a retry until the
                        # pixels happen to match. A stale Ref cache must fail.
                        page.evaluate('''() => new Promise(resolve=>{
                          let left=4;function frame(){if(--left===0)resolve();else requestAnimationFrame(frame);}
                          requestAnimationFrame(frame);
                        })''')
                        current=status();name=f'configure-{mode}-{visit}-{fi}'
                        png_sha=capture_pixels(name)
                        expected=witness['fixture'][fi*4+ci]['expected']
                        pixels=page.evaluate('''async ({name,expected,tolerance}) => {
                          const image=artGPUScreens[name],p=image.data;
                          const x0=Math.floor(image.width*.4),x1=Math.ceil(image.width*.6);
                          const y0=Math.floor(image.height*.4),y1=Math.ceil(image.height*.6);
                          let matching=0,count=0,maxError=0;const sample=[];
                          for(let y=y0;y<y1;y++)for(let x=x0;x<x1;x++){
                            const i=(y*image.width+x)*4;
                            const error=Math.max(...expected.map((v,k)=>Math.abs(p[i+k]-v)));
                            count++;if(error<=tolerance)matching++;maxError=Math.max(maxError,error);
                            if(sample.length===0)sample.push(...p.slice(i,i+4));
                          }
                          const digest=await crypto.subtle.digest('SHA-256',p);
                          return {width:image.width,height:image.height,roi:[x0,y0,x1,y1],count,matching,maxError,sample,
                            rgbaSHA256:[...new Uint8Array(digest)].map(x=>x.toString(16).padStart(2,'0')).join('')};
                        }''',dict(name=name,expected=expected,tolerance=witness['pixelTolerance']))
                        witness['observations'].append(dict(mode=mode,frame=frames[fi],visit=visit,expected=expected,
                            png=name+'.png',pngSHA256=png_sha,pixels=pixels,status=current))
                        assert pixels['count']>=100 and pixels['matching']==pixels['count'], ('Configure retained another frame/channel in actual WebGL pixels',mode,fi,pixels,expected)
                        assert current['stats']['Instances']>0, 'No real sprite draw accompanied the channel capture'
                        assert current['sequence']==published['sequence'] and current['revision']==published['revision'], 'Presentation selection republished the document'
                        assert current['stats']['MaterialReloadGenerations']==published['stats']['MaterialReloadGenerations'], 'Presentation selection reloaded material resources'
                        now=state()['state'];now['frame']=fixture_state['frame']
                        assert now==fixture_state and snapshot()==fixture_snapshot, 'Frame/channel preview changed source bytes, revision, selection or History'
                        if visit==0:first_pixels=pixels['rgbaSHA256']
                        elif visit==2:assert pixels['rgbaSHA256']==first_pixels, 'Returning to frame A did not restore its exact GPU pixels'
                page.locator('#gpu-mode').select_option('pixels')
                page.locator('.document-tab.active .close-document').click()
                page.wait_for_function('(handle)=>AegoStudioHost.summary().handle===handle',arg=original_snapshot['handle'])
                # Tool options are contextual: select the original tool before
                # restoring its size, which is hidden for the flood-fill tool.
                page.locator(f'[data-tool="{original_controls["tool"]}"]').click()
                page.locator('#fg-color').fill(original_controls['color'])
                page.locator('#fg-alpha').fill(original_controls['alpha']);page.locator('#fg-alpha').press('Tab')
                page.locator('#brush-size').fill(original_controls['size'])
                select('sprite')
                page.wait_for_function('(id)=>JSON.parse(aegoArtPreview.status()).status.document===id',arg=original_state['document']['id'])
                page.evaluate('''() => new Promise(resolve=>{
                  let left=4;function frame(){if(--left===0)resolve();else requestAnimationFrame(frame);}
                  requestAnimationFrame(frame);
                })''')
                assert state()['state']==original_state and snapshot()==original_snapshot, 'Temporary GPU fixture changed the original document'
                restored_resources=page.evaluate('() => JSON.parse(aegoArtPreview.resources())')
                witness['resourcesBefore']=original_resources
                witness['resourcesAfter']=restored_resources
                for field in ('Textures','Buffers','Shaders','Pipelines','Framebuffers'):
                    assert restored_resources[field]<=original_resources[field], ('Temporary frame/channel fixture retained GPU resources',field,original_resources,restored_resources)
                page.evaluate('() => {for(const name of Object.keys(artGPUScreens))if(name.startsWith("configure-"))delete artGPUScreens[name];}')
                witness['status']='PASS'
                checked('configure-frame-and-channel-pixels-without-republication')
            # Filled is a shape option, not a pencil option. Use the real tool
            # selector before interacting with the visible checkbox.
            page.locator('[data-tool="rectangle"]').click()
            page.locator('#filled').check()
            paint(6,7,24,25,tool='rectangle')
            select('sprite')
            assert len(responses)==2
            for key in ('wasm','previewWasm'):
                res=next(r for r in responses if r.url.endswith('/'+m[key]['path']))
                # Chromium evicts large streaming compilation bodies from the
                # DevTools inspector. Hash the immutable response in the browser
                # cache instead; this is the same content-hashed HTTP resource,
                # with no route interception or substituted bytes.
                downloaded=page.evaluate('''async url => {
                  const r=await fetch(url,{cache:'force-cache'});
                  if(!r.ok||!r.headers.get('Content-Type')?.startsWith('application/wasm'))throw Error('Invalid WASM response');
                  const hash=await crypto.subtle.digest('SHA-256',await r.arrayBuffer());
                  return [...new Uint8Array(hash)].map(x=>x.toString(16).padStart(2,'0')).join('');
                }''',res.url)
                assert res.status==200 and downloaded==m[key]['sha256']
            checked('lazy-core-and-real-gpu-wasm-match-manifest')
            configure_surface_pixels()
            source=state()['state']
            plain=capture('sprite')
            select('lit')
            page.wait_for_function('() => JSON.parse(aegoArtPreview.status()).status.stats.SelectedLights>0')
            lit=capture('lit')
            assert lit!=plain, 'Lit did not change rendered output'
            page.locator('#gpu-light').press('End')
            page.wait_for_timeout(350)
            moved=capture('moved-light')
            assert moved!=lit, 'Real light control has no observable result'
            sample_shadows()
            page.locator('#gpu-shadows').check()
            report['shadowEnableFrames']=shadow_result(state()['state']['document']['revision'])
            shadowed=capture('alpha-shadow')
            assert shadowed!=moved
            sample_shadows()
            paint(6,7,8,9,tool='eraser')
            report['shadowAlphaFrames']=shadow_result(state()['state']['document']['revision'])
            assert capture('changed-alpha-shadow')!=shadowed, 'Changed alpha did not update visible pixels and shadow'
            report['litStatus']=status()
            checked('runtime-lit-moving-light-and-alpha-shadows')
            # Missing emission must not fall back to uniform material glow.
            emission_strength('Home')
            no_emission=capture('no-emission-zero')
            emission_strength('End')
            assert capture('no-emission-maximum')==no_emission, 'Intensity emitted light without an emission channel'
            checked('albedo-only-artwork-has-no-material-emission')
            # Author actual surface data using the same UI and shared core.
            page.locator('#channel').select_option('normal')
            page.locator('#fg-color').fill('#8080ff')
            page.locator('#brush-size').fill('32')
            paint(16,16)
            page.locator('#channel').select_option('emission')
            page.locator('#fg-color').fill('#ffb030')
            page.locator('#brush-size').fill('6')
            paint(16,16)
            page.locator('#channel').select_option('albedo')
            page.wait_for_function('()=>{const s=JSON.parse(aegoArtPreview.status()).status;return s.hasNormal&&s.hasEmission&&s.revision===AegoStudioHost.summary().revision}')
            readonly=state()['state']
            page.locator('#gpu-bloom').uncheck()
            emission_strength('Home');capture_pixels('emission-zero-no-bloom')
            emission_strength('End');capture_pixels('emission-high-no-bloom')
            page.locator('#gpu-bloom').check()
            settle_gpu()
            page.wait_for_function('() => JSON.parse(aegoArtPreview.status()).status.stats.PostFXPasses>=4')
            capture_pixels('emission-high-bloom')
            emission_strength('Home');capture_pixels('emission-zero-bloom')
            report['emissionPixels']=page.evaluate('''() => {
              const a=artGPUScreens['emission-zero-no-bloom'],b=artGPUScreens['emission-high-no-bloom'];
              const c=artGPUScreens['emission-zero-bloom'],d=artGPUScreens['emission-high-bloom'];
              if([b,c,d].some(v=>v.width!==a.width||v.height!==a.height))throw Error('GPU capture dimensions changed');
              let changed=0,borderChanged=0,x0=a.width,y0=a.height,x1=-1,y1=-1;
              for(let y=0;y<a.height;y++)for(let x=0;x<a.width;x++){
                const i=(y*a.width+x)*4;
                const delta=Math.max(...[0,1,2].map(k=>b.data[i+k]-a.data[i+k]));
                if(delta>32){changed++;x0=Math.min(x0,x);y0=Math.min(y0,y);x1=Math.max(x1,x);y1=Math.max(y1,y);}
                if((x<a.width*.12||x>a.width*.88||y<a.height*.12||y>a.height*.88)&&delta>2)borderChanged++;
              }
              let halo=0;
              for(let y=0;y<a.height;y++)for(let x=0;x<a.width;x++){
                if(x>=x0-2&&x<=x1+2&&y>=y0-2&&y<=y1+2)continue;
                const i=(y*a.width+x)*4;
                // Subtract the Bloom contribution of ordinary Lit pixels.
                // Only the additional halo caused by authored emission counts.
                const extra=Math.max(...[0,1,2].map(k=>(d.data[i+k]-b.data[i+k])-(c.data[i+k]-a.data[i+k])));
                if(extra>8)halo++;
              }
              return {width:a.width,height:a.height,changed,borderChanged,halo,emissionBounds:[x0,y0,x1,y1]};
            }''')
            assert report['emissionPixels']['changed']>=20, ('Authored emission did not change actual Lit pixels',report['emissionPixels'])
            assert report['emissionPixels']['borderChanged']==0, ('Artwork intensity changed the shadow receiver material',report['emissionPixels'])
            assert report['emissionPixels']['halo']>=10, ('Bloom produced no additional halo from authored emission',report['emissionPixels'])
            emission_strength('End')
            assert status()['stats']['PostFXFailures']==0 and status()['stats']['LightingFailures']==0
            assert state()['state']==readonly, 'Emission and Bloom controls mutated the authoring document'
            checked('dirty-normal-emission-and-real-bloom')
            # An albedo-only frame in the same document must not inherit the
            # preceding frame's emission. Exercise the actual Play button too;
            # shared Go FrameAt drives the presentation's frame configuration.
            page.locator('#gpu-bloom').uncheck()
            page.locator('#add-frame').click()
            # A screenshot can take longer than a short animation frame on a
            # loaded software renderer. Keep playback running, but use the
            # supported maximum duration so the expected frame cannot normally
            # wrap back while Playwright captures it.
            page.locator('#duration').fill('600000');page.locator('#duration').press('Tab')
            paint(6,7,24,25,tool='rectangle')
            plain_frame=state()['state']['frame']
            page.wait_for_function('(frame)=>{const s=JSON.parse(aegoArtPreview.status()).status;return s.frame===frame&&!s.hasEmission&&s.revision===AegoStudioHost.summary().revision}',arg=plain_frame)
            emission_strength('Home');plain_frame_zero=capture('mixed-frame-no-emission-zero')
            emission_strength('End');plain_frame_high=capture('mixed-frame-no-emission-maximum')
            assert plain_frame_high==plain_frame_zero, 'Previous frame emission leaked into an albedo-only frame'
            page.locator('#first-frame').click();settle_gpu()
            assert status()['hasEmission']
            emissive_frame_high=capture('mixed-frame-emission-maximum')
            assert emissive_frame_high!=plain_frame_high, 'Playback fixture does not distinguish its emissive and plain frames'
            before_play=state()['state']
            programs=status()['stats']['MaterialPrograms']
            generations=status()['stats']['MaterialReloadGenerations']
            page.locator('#play').click()
            page.wait_for_function('''(frame)=>{
              const s=JSON.parse(aegoArtPreview.status()).status;
              const active=document.querySelector('#timeline-table th button[data-frame].active');
              return s.frame===frame&&!s.hasEmission&&active?.dataset.frame===frame;
            }''',arg=plain_frame)
            # Observe timeline changes, not just status at either endpoint: a
            # slow capture might otherwise wrap away and back to this frame.
            page.evaluate('''() => {
              const changes=[];
              const observer=new MutationObserver(records=>{
                for(const r of records)if(r.target.matches('th button[data-frame]'))
                  changes.push({frame:r.target.dataset.frame,oldClass:r.oldValue});
              });
              observer.observe(document.getElementById('timeline-table'),
                {subtree:true,attributes:true,attributeFilter:['class'],attributeOldValue:true});
              window.gpuPlaybackCapture={changes,observer,started:performance.now()};
            }''')
            settle_gpu()
            # Regression: exceed the old 1500ms frame duration while playback
            # remains live. This models a delayed screenshot consumer without
            # retrying or relaxing the pixel comparison.
            page.wait_for_timeout(1700)
            playback_before=status()
            playing_before=page.locator('#play').evaluate('(button)=>button.classList.contains("active")')
            playback_pixels=capture('playback-no-emission-frame')
            playback_after=status()
            playing_after=page.locator('#play').evaluate('(button)=>button.classList.contains("active")')
            playback_observation=page.evaluate('''() => {
              const {observer,changes,started}=window.gpuPlaybackCapture;
              for(const r of observer.takeRecords())if(r.target.matches('th button[data-frame]'))
                changes.push({frame:r.target.dataset.frame,oldClass:r.oldValue});
              observer.disconnect();delete window.gpuPlaybackCapture;
              return {changes,elapsedMS:performance.now()-started};
            }''')
            report['playbackCapture']=dict(before=playback_before,after=playback_after,
                playingBefore=playing_before,playingAfter=playing_after,**playback_observation)
            assert playback_observation['elapsedMS']>=1500, 'Playback capture did not exercise the delayed-consumer regression'
            assert playing_before and playing_after, 'Playback stopped during the GPU capture'
            assert not playback_observation['changes'], ('Playback changed frames during the GPU capture',playback_observation)
            for observed in (playback_before,playback_after):
                assert observed['frame']==plain_frame and not observed['hasEmission'], ('GPU capture observed a different playback frame',observed)
            assert playback_pixels==plain_frame_high, 'Automatic playback retained the emissive frame material'
            page.locator('#play').click();settle_gpu()
            assert state()['state']==before_play, 'Playback changed the authoring frame or document'
            assert status()['stats']['MaterialPrograms']==programs and status()['stats']['MaterialReloadGenerations']==generations, 'Playback recompiled or recreated the material'
            page.locator('#next-frame').click()
            assert state()['state']['frame']==plain_frame
            page.locator('#delete-frame').click()
            page.wait_for_function('()=>{const s=JSON.parse(aegoArtPreview.status()).status;return s.hasEmission&&s.frames===1&&s.revision===AegoStudioHost.summary().revision}')
            checked('mixed-frame-emission-follows-real-playback-without-shader-reload')
            readonly=state()['state']
            select('flash');page.locator('#gpu-amount').press('Home');page.wait_for_timeout(250)
            base=capture('flash-zero')
            page.locator('#gpu-amount').press('End');page.wait_for_timeout(250)
            assert capture('flash-one')!=base
            select('dissolve');page.locator('#gpu-amount').press('Home');page.wait_for_timeout(250)
            base=capture('dissolve-zero')
            page.locator('#gpu-amount').press('End');page.wait_for_timeout(250)
            assert capture('dissolve-one')!=base
            assert state()['state']==readonly, 'Preview mutated document, selection or History'
            checked('existing-flash-dissolve-shaders-and-preview-isolation')
            for _ in range(6):page.locator('#duplicate-frame').click()
            page.locator('#link-frame').click()
            page.locator('#duration').fill('160');page.locator('#duration').press('Tab')
            select('particles')
            page.wait_for_function('()=>{const s=JSON.parse(aegoArtPreview.status()).status;return s.frames===8&&s.particles>=3}',timeout=30000)
            report['particleStatus']=status();capture('particle-flipbook')
            assert status()['stats']['Instances']>0
            page.locator('#gpu-rate').press('Home')
            page.wait_for_function('() => JSON.parse(aegoArtPreview.status()).status.particles===0',timeout=15000)
            page.locator('#gpu-rate').press('ArrowRight')
            page.wait_for_function('() => JSON.parse(aegoArtPreview.status()).status.particles>0',timeout=10000)
            checked('real-cpu-particle-flipbook-rate-and-expiration')
            select('lit')
            doc_hash=page.evaluate('() => aegoArt.hash()')
            assert page.evaluate('() => aegoArtPreview.contextLoss(true)'), 'WEBGL_lose_context unavailable'
            page.wait_for_function('() => JSON.parse(aegoArtPreview.status()).status.state==="context-lost"')
            page.wait_for_timeout(500)
            assert 'lost' in page.locator('#gpu-status').inner_text().lower()
            assert page.evaluate('() => aegoArtPreview.contextLoss(false)')
            ready('lit')
            assert page.evaluate('() => aegoArt.hash()')==doc_hash
            checked('actual-webgl-context-loss-restore-cpu-pixels-preserved')
            page.set_viewport_size({'width':1280,'height':720})
            ready('lit');capture('resize-dpr2')
            assert page.evaluate('() => document.getElementById("gpu-preview").width>=document.getElementById("gpu-preview").clientWidth*1.9')
            page.set_viewport_size({'width':1440,'height':1050})
            from effect_templates_ui import exercise_effect_templates
            report['effectTemplates'] = exercise_effect_templates(page, m, args.evidence, checked)
            select('sprite');page.locator('#gpu-bloom').uncheck()
            page.wait_for_timeout(500)
            baseline=page.evaluate('() => JSON.parse(aegoArtPreview.resources())')
            for cycle in range(30):
                page.locator('#new').click()
                page.locator('#new-dialog [name=width]').fill('16')
                page.locator('#new-dialog [name=height]').fill('16')
                page.locator('#new-dialog [value=create]').click()
                page.wait_for_function('()=>JSON.parse(aegoArtPreview.status()).status.document===JSON.parse(aegoArt.request(\'{"action":"state"}\')).state.document.id')
                page.locator('.document-tab.active .close-document').click()
                page.wait_for_function('()=>JSON.parse(aegoArtPreview.status()).status.document===JSON.parse(aegoArt.request(\'{"action":"state"}\')).state.document.id')
                page.wait_for_timeout(70)
                resources=page.evaluate('() => JSON.parse(aegoArtPreview.resources())')
                report['resourceCycles'].append(resources)
                for field in ('Textures','Buffers','Shaders','Pipelines','Framebuffers'):
                    assert resources[field]<=baseline[field]+2,(cycle,field,resources,baseline)
            report['baselineResources']=baseline
            checked('thirty-document-open-close-cycles-no-growing-gpu-objects')
            assert page.evaluate('() => aegoArt.hash()')==doc_hash
            assert not report['consoleErrors'],report['consoleErrors']
            assert status()['stats']['MaterialCompileFailures']==0
            page.screenshot(path=str(args.evidence/'final.png'))
            ctx.close();browser.close()
        report['status']='PASS'
        return 0
    except Exception as e:
        report['error']=f'{type(e).__name__}: {e}'
        report['traceback']=traceback.format_exc()
        print(report['error'],file=sys.stderr)
        return 1
    finally:
        (args.evidence/'gpu.json').write_text(json.dumps(report,indent=2,ensure_ascii=False)+'\n')


if __name__=='__main__':
    raise SystemExit(main())
