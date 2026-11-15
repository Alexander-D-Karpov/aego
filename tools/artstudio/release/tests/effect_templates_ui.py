"""Real New Effect UI and GPU checks, called by both existing browser GPU gates.
All mutations use controls/pointers. Go is queried only for state and checksums.
"""
import base64
import hashlib


def exercise_effect_templates(page, manifest, evidence, checked):
    def state():
        return page.evaluate('() => JSON.parse(aegoArt.request(\'{"action":"state"}\'))')

    def snapshot_hash():
        return page.evaluate('async () => AegoStudioProtocol.sha256(AegoStudioHost.capture().bytes)')

    original = state()
    original_hash = snapshot_hash()
    catalog = page.evaluate('() => AegoStudioHost.effectTemplates()')
    expected_ids = ['dust', 'spark', 'smoke', 'explosion', 'fire', 'hit-flash', 'magic-projectile', 'trail']
    assert [item['id'] for item in catalog] == expected_ids
    records = {item['id']: item for item in manifest['templates']}
    assert set(records) == set(expected_ids)
    receipts = []
    for template in catalog:
        identifier = template['id']
        entry = records[identifier]
        assert entry['particlePreset'] == template['particlePreset']
        page.locator('#new').click()
        page.locator('#new-dialog [name=template]').select_option(identifier)
        for field in ('width', 'height', 'mode'):
            assert page.locator(f'#new-dialog [name={field}]').is_disabled()
        page.locator('#new-dialog [value=create]').click()
        page.wait_for_function('(old) => AegoStudioHost.summary().handle !== old', arg=original['active'])
        created = state()
        doc = created['state']['document']
        assert doc['properties']['aego.effectTemplate'] == identifier
        assert len(doc['frames']) == 8 and len(doc['layers']) >= 2 and doc['tags'] and doc['slices']
        assert created['state']['dirty'] and created['state']['undo'] == 0
        assert page.locator('[data-workspace=vfx]').get_attribute('class') == 'active'
        page.wait_for_function('''(id) => {
          if(!window.aegoArtPreview)return false;
          const s=JSON.parse(aegoArtPreview.status()).status;
          return s.state==='ready'&&s.mode==='particles'&&s.effectTemplate===id&&s.frames===8&&s.stats.MaterialPending===0;
        }''', arg=identifier, timeout=60000)
        status = page.evaluate('() => JSON.parse(aegoArtPreview.status()).status')
        assert status['particlePreset'] == template['particlePreset']
        page.wait_for_function('() => Number(document.getElementById("gpu-rate").value)===JSON.parse(aegoArtPreview.status()).status.particleRate')
        assert float(page.locator('#gpu-rate').input_value()) == status['particleRate']
        chosen_rate = status['particleRate'] + 1
        page.locator('#gpu-rate').press('ArrowRight')
        page.wait_for_function('(rate) => JSON.parse(aegoArtPreview.status()).status.particleRate===rate', arg=chosen_rate)
        page.locator('#next-frame').click()
        page.wait_for_function('() => JSON.parse(aegoArtPreview.status()).status.frame===JSON.parse(aegoArt.request(\'{"action":"state"}\')).state.frame')
        assert page.evaluate('() => JSON.parse(aegoArtPreview.status()).status.particleRate') == chosen_rate
        page.locator('#first-frame').click()
        assert state()['state']['undo'] == 0

        before = page.evaluate('() => aegoArt.hash()')
        page.locator('#hex-color').fill('#de17b9')
        page.locator('#hex-color').press('Tab')
        page.locator('[data-tool=pencil]').click()
        box = page.locator('#canvas').bounding_box()
        page.mouse.click(box['x']+1.5*box['width']/doc['canvas']['width'], box['y']+1.5*box['height']/doc['canvas']['height'])
        after = page.evaluate('() => aegoArt.hash()')
        assert before != after and state()['state']['undo'] == 1
        revision = state()['state']['document']['revision']
        page.wait_for_function('(revision) => JSON.parse(aegoArtPreview.status()).status.revision===revision', arg=revision)
        assert page.evaluate('() => JSON.parse(aegoArtPreview.status()).status.particleRate') == chosen_rate
        edit_checkpoint = state()['state']

        # Sample the real WebGL drawing buffer during RAF, including brief
        # one-shot HitFlash. No synthetic render or reference PNG is used.
        page.evaluate('''({id,revision}) => {
          let armed=false;
          document.getElementById('gpu-restart').addEventListener('click',()=>armed=true,{once:true});
          window.effectFrameProbe=new Promise((resolve,reject)=>{
            let finished=false;
            const timer=setTimeout(()=>{finished=true;reject(new Error('No visible runtime particle frame for '+id));},10000);
            async function sample(){
              if(finished)return;
              try {
              const s=JSON.parse(aegoArtPreview.status()).status;
              if(armed&&s.state==='ready'&&s.effectTemplate===id&&s.revision===revision&&s.particles>0&&s.stats.Instances>0){
                const png=document.getElementById('gpu-preview').toDataURL('image/png');
                const bytes=Uint8Array.from(atob(png.split(',')[1]),c=>c.charCodeAt(0));
                const image=await createImageBitmap(new Blob([bytes],{type:'image/png'}));
                const reader=document.createElement('canvas');reader.width=image.width;reader.height=image.height;
                const context=reader.getContext('2d',{willReadFrequently:true});context.drawImage(image,0,0);image.close();
                const rgba=context.getImageData(0,0,reader.width,reader.height).data;
                let visible=0;for(let i=4;i<rgba.length;i+=4)if(Math.abs(rgba[i]-rgba[0])+Math.abs(rgba[i+1]-rgba[1])+Math.abs(rgba[i+2]-rgba[2])>12)visible++;
                if(visible>=5){finished=true;clearTimeout(timer);resolve({png,visible,status:s,width:reader.width,height:reader.height});return;}
              }
              requestAnimationFrame(sample);
              } catch(error) { finished=true;clearTimeout(timer);reject(error); }
            }
            requestAnimationFrame(sample);
          });
        }''', {'id':identifier, 'revision':revision})
        page.locator('#gpu-restart').click()
        observed = page.evaluate('() => window.effectFrameProbe')
        png = base64.b64decode(observed.pop('png').split(',', 1)[1])
        (evidence/f'effect-{identifier}.png').write_bytes(png)
        assert observed['status']['stats']['MaterialCompileFailures'] == 0
        assert observed['status']['particleEmitted'] > 0
        assert state()['state'] == edit_checkpoint, 'Preview changed the document, selection or History'
        assert state()['state']['undo'] == 1 and page.evaluate('() => aegoArt.hash()') == after
        page.locator('#undo').click()
        assert page.evaluate('() => aegoArt.hash()') == before
        page.locator('#redo').click()
        assert page.evaluate('() => aegoArt.hash()') == after
        expected_source = snapshot_hash()
        with page.expect_download() as event:
            page.locator('#studio-download-source').click()
        download = event.value
        path = evidence/f'effect-{identifier}.aeart'
        download.save_as(path)
        assert hashlib.sha256(path.read_bytes()).hexdigest() == expected_source
        assert state()['state']['dirty'], 'Download guessed that a source was durably saved'
        edited = state()['state']['document']
        page.locator('#file-input').set_input_files(path)
        page.wait_for_function('(old) => AegoStudioHost.summary().handle!==old', arg=created['active'])
        reopened = state()
        assert reopened['state']['document'] == edited
        assert page.evaluate('() => aegoArt.hash()') == after
        page.locator('.document-tab.active .close-document').click()
        page.wait_for_function('(handle) => AegoStudioHost.summary().handle===handle', arg=created['active'])
        page.locator('.document-tab.active .close-document').click()
        page.wait_for_function('(handle) => AegoStudioHost.summary().handle===handle', arg=original['active'])
        assert snapshot_hash() == original_hash and state()['state'] == original['state']
        receipts.append({'id':identifier, 'preset':template['particlePreset'], 'sourceSHA256':expected_source,
                         'presetRate':status['particleRate'], 'editedPreviewRate':chosen_rate,
                         'beforePixelHash':before, 'afterPixelHash':after,
                         'framePNG_SHA256':hashlib.sha256(png).hexdigest(), 'observed':observed})
        checked('shared-effect-template-ui-render-edit-export-'+identifier)
    page.locator('[data-workspace=art]').click()
    return receipts
