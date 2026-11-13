"""Additional user-input authoring checks shared by the two browser gates.
Go API calls below only read state/pixels; every edit uses the actual UI.
"""
import hashlib
import json
from pathlib import Path


def exercise_export_controls(page, checked):
    """Check the actual localized controls without modifying the document."""
    original_language = page.locator('html').get_attribute('lang')
    original_theme = page.locator('#theme').input_value()
    assert page.locator('.appbar .build-badge').count() == 0
    expected = {
        'ru': (('PNG', 'Экспорт PNG'), ('Спрайт-лист', 'Экспорт спрайт-листа')),
        'en': (('PNG', 'Export PNG'), ('Sprite sheet', 'Export sprite sheet')),
    }
    for theme in ('light', 'dark'):
        page.locator('#theme').select_option(theme)
        for language in ('ru', 'en'):
            if page.locator('html').get_attribute('lang') != language:
                page.locator('#language').click()
            for selector, (text, label) in zip(('#export-png', '#export-sheet'), expected[language]):
                button = page.locator(selector)
                assert button.inner_text() == text
                assert button.get_attribute('aria-label') == button.get_attribute('title') == label
                assert page.get_by_role('button', name=label, exact=True).count() == 1
                icon = button.locator('svg')
                assert icon.count() == 1 and icon.get_attribute('aria-hidden') == 'true'
                measured = icon.evaluate('''icon => {
                    const i=icon.getBoundingClientRect(), b=icon.parentElement.getBoundingClientRect();
                    const style=getComputedStyle(icon), buttonStyle=getComputedStyle(icon.parentElement);
                    return {width:i.width,height:i.height,inside:i.left>=b.left && i.right<=b.right &&
                        i.top>=b.top && i.bottom<=b.bottom,stroke:style.stroke,color:buttonStyle.color};
                }''')
                assert measured['width'] == 14 and measured['height'] == 14 and measured['inside'], measured
                assert measured['stroke'] == measured['color'], measured
    if page.locator('html').get_attribute('lang') != original_language:
        page.locator('#language').click()
    page.locator('#theme').select_option(original_theme)
    # Enter still controls playback when focus is on the canvas, not a button.
    page.locator('#canvas').focus()
    page.keyboard.press('Enter')
    assert page.locator('#play').evaluate('element => element.classList.contains("active")')
    page.keyboard.press('Enter')
    assert not page.locator('#play').evaluate('element => element.classList.contains("active")')
    checked('export-icons-and-accessible-labels-in-both-languages-and-themes')


def exercise_authoring(page, checked, browser_name, evidence):
    def state():
        return page.evaluate('() => JSON.parse(aegoArt.request(\'{"action":"state"}\')).state')

    def image_hash():
        return page.evaluate('() => aegoArt.hash()')

    def at(x, y):
        b = page.locator('#canvas').bounding_box()
        c = state()['document']['canvas']
        return b['x']+(x+.5)*b['width']/c['width'], b['y']+(y+.5)*b['height']/c['height']

    def click(x, y):
        page.mouse.click(*at(x, y))

    def drag(tool, a, b):
        page.locator(f'[data-tool="{tool}"]').click()
        page.mouse.move(*at(*a));page.mouse.down()
        page.mouse.move(*at(*b), steps=1);page.mouse.up()

    previous = state()['document']['id']
    page.locator('#new').click()
    page.locator('#new-dialog [name=width]').fill('32')
    page.locator('#new-dialog [name=height]').fill('32')
    page.locator('#new-dialog [value=create]').click()
    page.wait_for_function('(old) => JSON.parse(aegoArt.request(\'{"action":"state"}\')).state.document.id !== old',arg=previous)
    blank = image_hash()
    drag('pencil', (2, 2), (26, 2))
    row = page.evaluate('() => [...aegoArt.pixels("",false,false)].slice(2*32*4,3*32*4)')
    assert all(row[x*4+3] == 255 for x in range(2, 27)), 'Fast stroke left holes'
    assert state()['undo'] == 1
    page.locator('#undo').click();assert image_hash() == blank
    page.locator('#redo').click()
    checked('fast-pointer-interpolation-and-one-stroke-history')

    stroked = image_hash()
    page.touchscreen.tap(*at(24, 28))
    assert image_hash() != stroked and state()['undo'] == 2
    page.locator('#undo').click();assert image_hash() == stroked
    checked('browser-touch-input-uses-the-same-stroke-and-undo')
    if browser_name == 'chromium':
        # Trusted browser input, rather than a dispatched DOM PointerEvent
        # whose nonexistent pointer capture would not exercise the UI path.
        cdp = page.context.new_cdp_session(page)
        x, y = at(26, 28)
        for event, buttons in (('mousePressed', 1), ('mouseReleased', 0)):
            cdp.send('Input.dispatchMouseEvent', {'type':event,'x':x,'y':y,'button':'left',
                     'buttons':buttons,'clickCount':1,'pointerType':'pen','force':.7 if buttons else 0})
        assert image_hash() != stroked and state()['undo'] == 2
        page.locator('#undo').click();assert image_hash() == stroked
        cdp.detach()
        checked('chromium-pen-input-uses-the-same-stroke-and-undo')

    unchanged = image_hash();revision = state()['document']['revision'];count = state()['undo']
    page.locator('[data-tool=rectangle]').click()
    page.locator('#filled').check()
    page.mouse.move(*at(3, 4));page.mouse.down();page.mouse.move(*at(8, 9))
    assert page.evaluate('() => document.activeElement === document.getElementById("canvas")'), 'Canvas did not take keyboard focus from the Filled checkbox'
    assert state()['gesture'] and state()['document']['revision'] == revision
    preview = page.evaluate('() => [...aegoArt.pixels("",false,false)]')
    assert preview[(5*32+5)*4+3] == 255
    page.keyboard.press('Escape');page.mouse.up()
    assert image_hash() == unchanged and state()['undo'] == count and not state()['gesture']
    checked('shape-preview-and-escape-do-not-publish')

    page.locator('[data-tool=curve]').click()
    count = state()['undo'];revision = state()['document']['revision']
    for p in ((3, 7), (8, 20), (20, 8)):
        click(*p)
    page.mouse.move(*at(27, 26))
    assert state()['gesture'] and state()['document']['revision'] == revision
    click(27, 26)
    assert state()['undo'] == count+1 and not state()['gesture']
    curve_hash = image_hash()
    page.locator('[data-tool=polygon]').click()
    for p in ((3, 14), (9, 14), (7, 22)):
        click(*p)
    assert state()['gesture']
    page.keyboard.press('Enter')
    assert state()['undo'] == count+2 and image_hash() != curve_hash
    page.locator('#undo').click();assert image_hash() == curve_hash
    page.locator('#redo').click()
    checked('curve-and-polygon-live-preview-single-undo')

    drag('rectangle', (2, 4), (4, 6))
    drag('select-rect', (2, 4), (4, 6))
    count = state()['undo']
    page.locator('#capture-brush').click()
    assert state()['brushWidth'] == 3 and state()['brushHeight'] == 3 and state()['undo'] == count
    click(14, 14)
    pixels = page.evaluate('() => [...aegoArt.pixels("",false,false)]')
    assert all(pixels[(y*32+x)*4+3] == 255 for y in range(13, 16) for x in range(13, 16))
    page.locator('#brush-source').select_option('pattern')
    drag('pencil', (21, 14), (25, 14))
    assert state()['undo'] == count+2
    page.locator('#brush-source').select_option('solid')
    checked('selection-captured-custom-and-pattern-brushes')

    before_layer = image_hash()
    page.locator('#add-layer').click()
    page.locator('#layer-dialog [name=name]').fill('Merge and isolate')
    page.locator('#layer-dialog [value=apply]').click()
    page.wait_for_function('() => JSON.parse(aegoArt.request(\'{"action":"state"}\')).state.document.layers.length === 2')
    page.locator('#swatches button').nth(12).click()
    drag('rectangle', (1, 1), (5, 5))
    composite = image_hash();count = state()['undo'];revision = state()['document']['revision']
    page.locator('#isolate-layer').click()
    assert state()['isolation'] and state()['document']['revision'] == revision and state()['undo'] == count
    page.locator('#isolate-layer').click()
    topology = state()['document']['cels']
    page.locator('#merge-down').click()
    assert len(state()['document']['layers']) == 1 and image_hash() == composite
    page.locator('#undo').click()
    assert len(state()['document']['layers']) == 2 and state()['document']['cels'] == topology
    page.locator('#redo').click()
    assert image_hash() == composite and composite != before_layer
    checked('layer-isolation-merge-pixels-and-topology-undo')

    page.locator('#transform').click()
    page.locator('#transform-dialog [name=kind]').select_option('scale')
    page.locator('#transform-dialog [name=width]').fill('20')
    page.locator('#transform-dialog [name=height]').fill('20')
    revision = state()['document']['revision']
    page.locator('#transform-dialog [value=apply]').click()
    page.wait_for_function('(old) => JSON.parse(aegoArt.request(\'{"action":"state"}\')).state.document.revision > old',arg=revision)
    assert image_hash() != composite
    page.locator('#undo').click();assert image_hash() == composite
    page.locator('#pivot').click()
    page.locator('#pivot-dialog [name=x]').fill('8')
    page.locator('#pivot-dialog [name=y]').fill('30')
    page.locator('#pivot-dialog [value=apply]').click()
    page.wait_for_function('() => JSON.parse(aegoArt.request(\'{"action":"state"}\')).state.document.slices.length === 1')
    assert state()['document']['slices'][-1]['pivot'] == {'X':8,'Y':30}
    assert page.locator('#pivot-overlay').is_visible()
    checked('nearest-pixel-scale-and-current-frame-pivot')

    page.locator('#mode').select_option('indexed')
    indexed = image_hash()
    order = list(range(len(state()['document']['palette'])))
    order[6], order[12] = order[12], order[6]
    page.locator('#palette-order').click()
    page.locator('#palette-order-dialog [name=order]').fill(','.join(map(str,order)))
    revision = state()['document']['revision']
    page.locator('#palette-order-dialog [value=apply]').click()
    page.wait_for_function('(old) => JSON.parse(aegoArt.request(\'{"action":"state"}\')).state.document.revision > old',arg=revision)
    assert image_hash() == indexed, 'Palette reorder changed indexed appearance'
    page.locator('#ramp-shade').click()
    page.locator('#ramp-dialog [name=first]').fill('1')
    page.locator('#ramp-dialog [name=last]').fill('15')
    page.locator('#ramp-dialog [name=step]').fill('1')
    revision = state()['document']['revision']
    page.locator('#ramp-dialog [value=apply]').click()
    page.wait_for_function('(old) => JSON.parse(aegoArt.request(\'{"action":"state"}\')).state.document.revision > old',arg=revision)
    assert image_hash() != indexed
    page.locator('#undo').click();assert image_hash() == indexed
    checked('true-indexed-palette-reorder-and-ramp-shading')

    exports = Path(evidence) / 'authoring-exports'
    exports.mkdir(exist_ok=False)

    def decode_png(path):
        # Use the browser's real PNG decoder to inspect downloaded bytes. This
        # canvas is test-only readback; it never implements authoring or export.
        return page.evaluate('''async bytes => {
            const bitmap=await createImageBitmap(new Blob([new Uint8Array(bytes)],{type:'image/png'}));
            const canvas=document.createElement('canvas');canvas.width=bitmap.width;canvas.height=bitmap.height;
            const context=canvas.getContext('2d',{willReadFrequently:true});context.drawImage(bitmap,0,0);
            const result={width:bitmap.width,height:bitmap.height,
                pixels:Array.from(context.getImageData(0,0,bitmap.width,bitmap.height).data)};
            bitmap.close();return result;
        }''',list(path.read_bytes()))

    # Height is an authored data channel. Do not claim a parallax shader where
    # the renderer only supports displaying/exporting those height samples.
    page.locator('#mode').select_option('rgba')
    page.locator('#channel').select_option('height')
    page.locator('#brush-source').select_option('solid')
    page.locator('#brush-size').fill('1')
    page.locator('#brush-size').press('Tab')
    page.locator('#hex-color').fill('#808080')
    page.locator('#hex-color').press('Tab')
    page.locator('#fg-alpha').fill('255')
    page.locator('#fg-alpha').press('Tab')
    page.locator('#canvas').focus()
    page.keyboard.press('Control+d')
    before_height = image_hash()
    albedo = page.evaluate('() => aegoArt.hash("","albedo")')
    count = state()['undo']
    page.locator('[data-tool=pencil]').click()
    click(11, 12)
    height_hash = image_hash()
    assert height_hash != before_height and state()['undo'] == count+1
    assert page.evaluate('() => aegoArt.hash("","albedo")') == albedo
    assert page.evaluate('() => Array.from(aegoArt.pixels("",false,false)).slice((12*32+11)*4,(12*32+12)*4)') == [128,128,128,255]
    with page.expect_download() as height_download:
        page.locator('#export-png').focus()
        page.keyboard.press('Enter')
    height_path = exports / 'height-channel.png'
    height_download.value.save_as(height_path)
    assert height_download.value.failure() is None
    assert not page.locator('#play').evaluate('element => element.classList.contains("active")')
    height_image = decode_png(height_path)
    assert (height_image['width'],height_image['height']) == (32,32)
    assert height_image['pixels'][(12*32+11)*4:(12*32+12)*4] == [128,128,128,255]
    assert hashlib.sha256(bytes(height_image['pixels'])).hexdigest() == height_hash
    with page.expect_download() as space_download:
        page.locator('#export-png').focus()
        page.keyboard.press('Space')
    space_path = exports / 'height-channel-space.png'
    space_download.value.save_as(space_path)
    assert space_download.value.failure() is None and space_path.read_bytes() == height_path.read_bytes()
    page.locator('#undo').click();assert image_hash() == before_height
    page.locator('#redo').click();assert image_hash() == height_hash
    checked('height-channel-pointer-undo-redo-and-real-png-download')

    page.locator('#channel').select_option('albedo')
    for _ in range(6):
        page.locator('#duplicate-frame').click()
    page.locator('#link-frame').click()
    assert len(state()['document']['frames']) == 8
    page.locator('#duration').fill('175')
    page.locator('#duration').press('Tab')
    before_tag = state()['document']
    expected_tag = {'name':'Exported loop','from':before_tag['frames'][0]['id'],
                    'to':before_tag['frames'][-1]['id'],'direction':'ping-pong'}
    page.locator('#add-tag').click()
    page.locator('#tag-dialog [name=name]').fill('Exported loop')
    page.locator('#tag-dialog [name=from]').fill('1')
    page.locator('#tag-dialog [name=to]').fill('8')
    page.locator('#tag-dialog [name=direction]').select_option('ping-pong')
    page.locator('#tag-dialog [value=create]').click()
    # The native dialog queues its close event separately from the click.
    # Wait for that handler's actual Go commit, not just the closed dialog.
    page.wait_for_function('''expected => {
        const doc = JSON.parse(aegoArt.request('{"action":"state"}')).state.document;
        const tag = doc.tags[doc.tags.length-1];
        return !document.querySelector('#tag-dialog').open && doc.id === expected.documentID
            && doc.tags.length === expected.count && Boolean(tag)
            && tag.name === expected.tag.name && tag.from === expected.tag.from
            && tag.to === expected.tag.to && tag.direction === expected.tag.direction;
    }''', arg={'documentID':before_tag['id'],'count':len(before_tag['tags'])+1,
               'tag':expected_tag}, timeout=10000)
    assert state()['document']['tags'][-1]['direction'] == 'ping-pong'
    created_tags = state()['document']['tags']
    assert created_tags[:-1] == before_tag['tags'], 'Creating a tag modified existing tags'
    assert created_tags[-1]['id'] and created_tags[-1]['id'] not in {tag['id'] for tag in before_tag['tags']}
    assert {key:created_tags[-1][key] for key in expected_tag} == expected_tag
    # One independent frame differs, so a duplicated-first-frame export cannot
    # satisfy the pixel assertion for all eight sprite sheet regions.
    page.locator('#timeline-table thead button').nth(3).click()
    page.locator('#hex-color').fill('#16a5e8')
    page.locator('#hex-color').press('Tab')
    click(23, 25)
    source = state()
    expected_hashes = page.evaluate('''() => JSON.parse(aegoArt.request('{"action":"state"}')).state.document.frames
        .map(frame=>[frame.id,aegoArt.hash(frame.id,'albedo')])''')
    assert len(set(value for _,value in expected_hashes)) > 1
    with page.expect_download(predicate=lambda item: item.suggested_filename.endswith('-sheet.png')) as png_download, \
         page.expect_download(predicate=lambda item: item.suggested_filename.endswith('-sheet.json')) as metadata_download:
        page.locator('#export-sheet').focus()
        page.keyboard.press('Enter')
    sheet_path, metadata_path = exports / 'animation-sheet.png', exports / 'animation-sheet.json'
    png_download.value.save_as(sheet_path)
    metadata_download.value.save_as(metadata_path)
    assert png_download.value.failure() is None and metadata_download.value.failure() is None
    assert not page.locator('#play').evaluate('element => element.classList.contains("active")')
    metadata = json.loads(metadata_path.read_text())
    sheet = decode_png(sheet_path)
    assert (sheet['width'],sheet['height']) == (256,32)
    assert (metadata['width'],metadata['height']) == (sheet['width'],sheet['height'])
    assert len(metadata['frames']) == 8
    assert metadata['tags'] == source['document']['tags']
    assert metadata['slices'] == source['document']['slices'] and metadata['slices']
    raw_pixels = bytes(sheet['pixels'])
    actual_hashes = []
    for index,(frame,expected) in enumerate(zip(metadata['frames'],expected_hashes)):
        assert frame['id'] == expected[0] == source['document']['frames'][index]['id']
        assert frame['durationMS'] == source['document']['frames'][index]['durationMS']
        assert frame['rect'] == {'Min':{'X':index*32,'Y':0},'Max':{'X':(index+1)*32,'Y':32}}
        pixels = b''.join(raw_pixels[(y*256+index*32)*4:(y*256+(index+1)*32)*4] for y in range(32))
        digest = hashlib.sha256(pixels).hexdigest()
        assert digest == expected[1], (index,expected[0],'sheet region differs from actual source pixels')
        actual_hashes.append([frame['id'],digest])
    assert state()['document'] == source['document'] and state()['dirty'] == source['dirty']
    assert state()['undo'] == source['undo'], 'Export modified history'
    (exports / 'verification.json').write_text(json.dumps({
        'status':'PASS','browser':browser_name,'sourceDocument':source['document'],
        'heightPixel':[128,128,128,255],'heightPixelHash':height_hash,
        'frameHashes':actual_hashes,'width':sheet['width'],'height':sheet['height'],
        'downloads':[{'path':p.name,'bytes':p.stat().st_size,'sha256':hashlib.sha256(p.read_bytes()).hexdigest()}
                     for p in (height_path,sheet_path,metadata_path)],
        'scope':'Actual UI and browser downloads; .aeanim and runtime AssetIDs belong to the native importer.'
    },ensure_ascii=False,indent=2)+'\n')
    checked('actual-spritesheet-download-eight-distinct-regions-durations-tags-and-pivots')

    page.locator('.document-tab.active .close-document').click()
    assert state()['document']['id'] == previous, 'Closing scratch tab lost the earlier dirty document'
