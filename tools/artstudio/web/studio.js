/* Browser presentation and pointer transport only. Pixel algorithms, selections,
 * animation timing, document edits, undo and all exports run in Go/WASM. */
'use strict';
const $ = id => document.getElementById(id);
const translations = {
  ru: { new: 'Создать', open: 'Открыть', save: 'Сохранить .aeart', undo: 'Отменить', redo: 'Повторить', sheet: 'Спрайт-лист ↗', local: 'Файлы остаются в браузере', size: 'Размер', filled: 'Заливка', channel: 'Канал', palette: 'Палитра', gray: 'Серый', foreground: 'Основной цвет', 'edit-color': 'Изменить цвет', layers: 'Слои', opacity: 'Непрозрачность', preview: 'Предпросмотр', 'flip-x': 'Отразить X', rotate: 'Повернуть', timeline: 'Кадры и ячейки', onion: 'Калька', 'frame-add': '+ Кадр', duplicate: 'Копия', link: 'Связать', unlink: 'Отвязать', duration: 'Длительность', tag: 'Метка', 'all-frames': 'Все кадры', grid: 'Сетка', fit: 'Вписать', recovered: 'В браузере есть несохранённая работа.', restore: 'Восстановить', loading: 'Загрузка общего Go/WASM-ядра…', hints: 'B — карандаш · E — ластик · Ctrl+Z — отменить · Esc — отменить жест', 'new-document': 'Новый рисунок', width: 'Ширина', height: 'Высота', 'color-mode': 'Режим цвета', cancel: 'Отмена', create: 'Создать', 'animation-tag': 'Метка анимации', name: 'Имя', from: 'От кадра', to: 'До кадра', direction: 'Направление', ready: 'Готово', unsaved: 'Есть несохранённые изменения. Продолжить без сохранения?', saved: 'Редактируемый исходник подготовлен к сохранению', exported: 'Экспорт готов', recoverySaved: 'Восстановление сохранено локально', recoveryError: 'Не удалось сохранить локальное восстановление', layerName: 'Имя слоя', layer: 'Слой', frames: 'кадров', colors: 'цветов', selected: 'выделено', copy: 'Скопировано', fileLimit: 'Файл слишком большой (максимум 64 МиБ)', badColor: 'Введите цвет #RRGGBB', pencil: 'Карандаш', eraser: 'Ластик', fill: 'Заливка', eyedropper: 'Пипетка', line: 'Линия', rectangle: 'Прямоугольник', ellipse: 'Эллипс', gradient: 'Градиент', 'select-rect': 'Выделение', 'select-ellipse': 'Овальное выделение', lasso: 'Лассо', wand: 'Волшебная палочка', move: 'Перемещение', polygon: 'Многоугольник', curve: 'Кривая', polyHint: 'Щёлкайте вершины; Enter — закончить; Esc — отменить', curveHint: 'Выберите начало, две контрольные точки и конец', noCel: 'В выбранной ячейке нет рисунка', imported: 'Документ открыт', storageUnavailable: 'Локальное восстановление недоступно', resetLayout: 'Раскладка сброшена' },
  en: { new: 'New', open: 'Open', save: 'Save .aeart', undo: 'Undo', redo: 'Redo', sheet: 'Sprite sheet ↗', local: 'Your files stay in this browser', size: 'Size', filled: 'Filled', channel: 'Channel', palette: 'Palette', gray: 'Gray', foreground: 'Foreground', 'edit-color': 'Edit swatch', layers: 'Layers', opacity: 'Opacity', preview: 'Preview', 'flip-x': 'Flip X', rotate: 'Rotate', timeline: 'Frames & cels', onion: 'Onion skin', 'frame-add': '+ Frame', duplicate: 'Duplicate', link: 'Link', unlink: 'Unlink', duration: 'Duration', tag: 'Tag', 'all-frames': 'All frames', grid: 'Grid', fit: 'Fit', recovered: 'Unsaved work is available in this browser.', restore: 'Restore', loading: 'Loading shared Go/WASM core…', hints: 'B — pencil · E — eraser · Ctrl+Z — undo · Esc — cancel gesture', 'new-document': 'New drawing', width: 'Width', height: 'Height', 'color-mode': 'Color mode', cancel: 'Cancel', create: 'Create', 'animation-tag': 'Animation tag', name: 'Name', from: 'From frame', to: 'To frame', direction: 'Direction', ready: 'Ready', unsaved: 'There are unsaved changes. Continue without saving?', saved: 'Editable source is ready to save', exported: 'Export ready', recoverySaved: 'Recovery saved locally', recoveryError: 'Could not save local recovery', layerName: 'Layer name', layer: 'Layer', frames: 'frames', colors: 'colors', selected: 'selected', copy: 'Copied', fileLimit: 'File too large (64 MiB maximum)', badColor: 'Enter a #RRGGBB color', pencil: 'Pencil', eraser: 'Eraser', fill: 'Flood fill', eyedropper: 'Eyedropper', line: 'Line', rectangle: 'Rectangle', ellipse: 'Ellipse', gradient: 'Gradient', 'select-rect': 'Selection', 'select-ellipse': 'Ellipse selection', lasso: 'Lasso', wand: 'Magic wand', move: 'Move', polygon: 'Polygon', curve: 'Curve', polyHint: 'Click vertices; Enter to finish; Esc to cancel', curveHint: 'Choose start, two control points and end', noCel: 'There is no drawing in this cel', imported: 'Document opened', storageUnavailable: 'Local recovery is unavailable', resetLayout: 'Layout reset' }
};
Object.assign(translations.ru, {"palette-import": "Импорт", "palette-export": "Экспорт", "palette-extract": "Из рисунка", "palette-order": "Порядок", "brush-shape": "Форма", "square": "Квадрат", "round": "Круг", "dither": "Дизеринг", "range-hint": "Shift — диапазон · Ctrl — несколько ячеек", "range-transfer": "Перенос / копия", "copy-animation": "Копировать кадры", "paste-animation": "Вставить анимацию", "edit": "Изменить", "apply": "Применить", "palette-help": "Без квантования рисунок не меняется. Квантование явно ограничивает цвета всей анимации.", "color-limit": "Количество цветов", "quantize": "Квантовать цвета рисунка", "palette-order-help": "Перестановка: новый индекс → старый, вид рисунка сохраняется. Remap: старый → новый, рисунок перекрашивается. Индекс 0 остаётся первым.", "indices": "Индексы", "recolor": "Remap: намеренно перекрасить индексы", "range-help": "Смещение от исходных ячеек. Отрицательное число означает предыдущие кадры или нижние слои.", "frame-offset": "Смещение кадров", "layer-offset": "Смещение слоёв", "action": "Действие", "overwrite-cels": "Разрешить замену целевых ячеек", "layer-properties": "Свойства слоя", "layer-kind": "Тип", "raster": "Растровый", "group": "Группа", "reference": "Референс", "parent": "Группа", "no-group": "Без группы", "range-selected": "выбрано ячеек", "palette-loaded": "Палитра загружена без изменения рисунка", "stale-dialog": "Документ изменился; откройте диалог снова.", "empty-animation": "Сначала выберите кадры.", "invalid-order": "Укажите целые индексы через запятую.", "new-layer": "Новый слой"});
Object.assign(translations.en, {"palette-import": "Import", "palette-export": "Export", "palette-extract": "From image", "palette-order": "Order", "brush-shape": "Shape", "square": "Square", "round": "Round", "dither": "Dither", "range-hint": "Shift \u2014 range \u00b7 Ctrl \u2014 multiple cels", "range-transfer": "Move / copy", "copy-animation": "Copy frames", "paste-animation": "Paste animation", "edit": "Edit", "apply": "Apply", "palette-help": "Without quantization the artwork is unchanged. Quantization explicitly limits colors in the whole animation.", "color-limit": "Color count", "quantize": "Quantize artwork colors", "palette-order-help": "Reorder: new index \u2192 old, preserving appearance. Remap: old \u2192 new, intentionally recoloring. Keep index 0 first.", "indices": "Indices", "recolor": "Remap: intentionally recolor indices", "range-help": "Offsets from source cels. Negative numbers move to earlier frames or lower layers.", "frame-offset": "Frame offset", "layer-offset": "Layer offset", "action": "Action", "overwrite-cels": "Allow replacing destination cels", "layer-properties": "Layer properties", "layer-kind": "Type", "raster": "Raster", "group": "Group", "reference": "Reference", "parent": "Group", "no-group": "No group", "range-selected": "selected cels", "palette-loaded": "Palette imported without recoloring", "stale-dialog": "Document changed; reopen the dialog.", "empty-animation": "Select frames first.", "invalid-order": "Enter integer indices separated by commas.", "new-layer": "New layer"});
const icons = {
  pencil: '<path d="m4 17 1-5L16 1l5 5L10 17l-6 1Zm10-14 5 5M5 12l5 5"/>',
  eraser: '<path d="m3 13 9-10 8 7-9 10H8l-5-5Zm5-5 8 7M12 20h9"/>',
  fill: '<path d="m4 11 8-8 8 8-8 8-8-8Zm3-6L4 2M4 11h16M20 16q-4 4 0 5 4-1 0-5"/>',
  eyedropper: '<path d="m4 17 9-9 3 3-9 9H4v-3Zm8-11 6 6M15 7l4-4 2 2-4 4"/>',
  line: '<path d="M4 20 20 4"/><rect x="2" y="18" width="4" height="4"/><rect x="18" y="2" width="4" height="4"/>',
  rectangle: '<rect x="3" y="5" width="18" height="14" rx="1"/>',
  ellipse: '<ellipse cx="12" cy="12" rx="10" ry="7"/>',
  gradient: '<rect x="3" y="4" width="18" height="16"/><path d="M6 4v16M9 4v16M13 4v16M18 4v16"/>',
  'select-rect': '<rect x="3" y="4" width="18" height="16" stroke-dasharray="3 3"/>',
  'select-ellipse': '<ellipse cx="12" cy="12" rx="10" ry="7" stroke-dasharray="3 3"/>',
  lasso: '<path d="M7 16C-1 13 1 3 12 3s15 12 2 14c-5 1-8-4-5-6s8 2 2 8l-4 3"/>',
  wand: '<path d="m4 21 13-13 3 3L7 24M12 3v4M21 2v4M18 5h6M3 9h4M5 7v4"/>',
  move: '<path d="M12 2v20M2 12h20M8 6l4-4 4 4M8 18l4 4 4-4M6 8l-4 4 4 4M18 8l4 4-4 4"/>',
  polygon: '<path d="m3 8 7-6 11 7-5 12L2 18 3 8Z"/>',
  curve: '<path d="M2 20C3-3 21 27 22 3"/><path d="M2 20 6 3M22 3l-4 18" stroke-dasharray="2 2"/>'
};
const toolKeys = { pencil: 'B', eraser: 'E', fill: 'G', eyedropper: 'I', line: 'L', rectangle: 'R', ellipse: 'O', gradient: 'D', 'select-rect': 'M', 'select-ellipse': '', lasso: 'Q', wand: 'W', move: 'V', polygon: 'P', curve: 'C' };
let lang = safeGet('aego.art.language') || 'ru';
let api, state, tool = 'pencil', zoom = 8, fg = { R: 255, G: 105, B: 110, A: 255 }, bg = { R: 24, G: 28, B: 42, A: 255 }, fgIndex = 6, bgIndex = 1;
let grid = true, mirrorX = false, mirrorY = false, onion = false, playing = false, playStart = 0, playFrame = '', lastDraw = 0;
let pointer = null, pointerStart = null, lastPoint = { X: 0, Y: 0 }, selectionPath = [], construction = [], pan = null, pinch = null;
const touches = new Map();
let recoveryTimer, recoveryData = [], recoveryStore = null, filename = 'Untitled.aeart';
let tabs = [], activeDocument = '', pendingView = null;
const views = new Map(), recoveryKeys = new Map();
const browserSession = crypto.randomUUID ? crypto.randomUUID() : `${Date.now()}-${Math.random()}`;
let recoveryWrites = Promise.resolve();
let rangeCells = new Map(), rangeAnchor = null;
const dialogRevisions = new Map();
let editingLayer = null, editingTag = null, pendingRange = [];
function captureView() {
  if (!activeDocument) return;
  views.set(activeDocument, { zoom, left: $('viewport').scrollLeft, top: $('viewport').scrollTop,
    tag: $('tags').value, onion, grid, range: [...rangeCells.values()], anchor: rangeAnchor });
}
function adopt(res) {
  if (!res?.state) return;
  if (activeDocument !== res.active) { captureView(); pendingView = views.get(res.active) || null; rangeCells = new Map(); rangeAnchor = null; }
  state = res.state; tabs = res.tabs || []; activeDocument = res.active || '';
  filename = tabs.find(v => v.key === activeDocument)?.name || 'Untitled.aeart';
}
function restoreView() {
  const view = pendingView || views.get(activeDocument);
  if (view) {
    onion = view.onion; grid = view.grid; rangeCells = new Map((view.range || []).map(v => [cellKey(v), v])); rangeAnchor = view.anchor || null; $('onion').checked = onion;
    $('grid').classList.toggle('active', grid); $('tags').value = view.tag; setZoom(view.zoom);
    $('viewport').scrollLeft = view.left; $('viewport').scrollTop = view.top;
  } else { fitZoom(); }
  pendingView = null; renderTimeline();
}
function renderTabs() {
  const target = $('document-tabs'); target.replaceChildren();
  for (const tab of tabs) {
    const group = document.createElement('span'); group.className = 'document-tab';
    group.classList.toggle('active', tab.key === activeDocument);
    const b = makeButton(`${tab.dirty ? '• ' : ''}${tab.name}`, tab.name, () => {
      cancelPointer(); stopPlayback(); captureView();
      if (call({ action: 'document-select', handle: tab.key })) { renderUI(); restoreView(); paint(); }
    });
    b.dataset.document = tab.key; b.setAttribute('role', 'tab'); b.setAttribute('aria-selected', String(tab.key === activeDocument));
    const close = makeButton('×', lang === 'ru' ? 'Закрыть документ' : 'Close document', () => closeDocument(tab));
    close.className = 'close-document'; close.dataset.closeDocument = tab.key;
    group.append(b, close); target.append(group);
  }
}
function closeDocument(tab) {
  if (tab.dirty && !confirm(t('unsaved'))) return;
  cancelPointer(); stopPlayback();
  const key = recoveryKeys.get(tab.key);
  if (!call({ action: 'document-close', handle: tab.key, confirm: true })) return;
  views.delete(tab.key); recoveryKeys.delete(tab.key);
  if (key && recoveryStore) queueRecovery(() => recoveryStore.remove(key));
  renderUI(); restoreView(); paint();
}

const t = key => translations[lang]?.[key] || translations.en[key] || key;
function safeGet(key) { try { return localStorage.getItem(key) } catch { return null } }
function safeSet(key, value) { try { localStorage.setItem(key, value) } catch {/* Optional UI preferences. */ } }
function say(message, error = false) { $('status').textContent = message; document.querySelector('.statusbar').classList.toggle('error', error) }
function makeButton(label, title, fn, className = '') { const b = document.createElement('button'); b.type = 'button'; b.textContent = label; b.title = title || label; b.className = className; if (fn) b.addEventListener('click', fn); return b }
function translate() { document.documentElement.lang = lang; $('language').textContent = lang.toUpperCase(); document.querySelectorAll('[data-i18n]').forEach(el => el.textContent = t(el.dataset.i18n)); buildTools(); if (state) renderUI(); safeSet('aego.art.language', lang) }
function buildTools() { const target = $('tools'); target.replaceChildren(); for (const [name, path] of Object.entries(icons)) { const b = makeButton('', `${t(name)}${toolKeys[name] ? ' · ' + toolKeys[name] : ''}`, () => selectTool(name), 'tool'); b.dataset.tool = name; b.setAttribute('aria-label', t(name)); b.innerHTML = `<svg viewBox="0 0 24 24" aria-hidden="true">${path}</svg><span class="shortcut">${toolKeys[name]}</span>`; b.classList.toggle('active', name === tool); target.append(b) } $('tool-name').textContent = t(tool) }
function selectTool(name) { if (!api) return; cancelPointer(); construction = []; tool = name; buildTools(); if (name === 'polygon') say(t('polyHint')); else if (name === 'curve') say(t('curveHint')); else say(t('ready')) }
function call(req, { quiet = false, update = true } = {}) { if (!api) return null; const res = JSON.parse(api.request(JSON.stringify(req))); if (update) adopt(res); if (res.error) { say(res.error, true); return null } if (!quiet) say(t('ready')); return res }
function brush() { return { paint: { color: fg, index: fgIndex, useIndex: state.document.canvas.mode === 'indexed' }, background: { color: bg, index: bgIndex, useIndex: state.document.canvas.mode === 'indexed' }, size: Number($('brush-size').value) || 1, pixelPerfect: $('pixel-perfect').checked, round: $('brush-shape').value === 'round', mirrorX, mirrorY, dither: Number($('brush-dither').value) } }
function activeCel() { return state.document.cels.find(c => c.layer === state.layer && c.frame === state.frame && c.channel === state.channel) }
function commit(op, expectedRevision = state.document.revision) { stopPlayback(); const res = call({ action: 'apply', operation: op, expectedRevision }); if (res) { renderUI(); paint(); scheduleRecovery() } return res }
function colorHex(c) { return '#' + [c.R, c.G, c.B].map(v => v.toString(16).padStart(2, '0')).join('') }
function parseColor(v) { if (!/^#[0-9a-f]{6}$/i.test(v)) return null; return { R: parseInt(v.slice(1, 3), 16), G: parseInt(v.slice(3, 5), 16), B: parseInt(v.slice(5, 7), 16), A: 255 } }
function colorsUI() { $('fg-alpha').value = fg.A; $('fg-color').value = colorHex(fg); $('hex-color').value = colorHex(fg); $('swatches').querySelectorAll('button').forEach((b, i) => b.classList.toggle('selected', i === fgIndex)); $('palette-info').textContent = `${state.document.palette.length} ${t('colors')}` }
function renderPalette() { fgIndex = Math.min(fgIndex, state.document.palette.length - 1); bgIndex = Math.min(bgIndex, state.document.palette.length - 1); if (state.document.canvas.mode === 'indexed') { fg = {...state.document.palette[fgIndex].color}; bg = {...state.document.palette[bgIndex].color}; } const target = $('swatches'); target.replaceChildren(); state.document.palette.forEach((sw, i) => { const b = makeButton('', `${i}: ${sw.name || colorHex(sw.color)}`, () => { fgIndex = i; fg = { ...sw.color }; colorsUI() }, 'swatch'); b.style.backgroundColor = `rgba(${sw.color.R},${sw.color.G},${sw.color.B},${sw.color.A / 255})`; if (sw.color.A === 0) b.classList.add('transparent'); b.addEventListener('contextmenu', e => { e.preventDefault(); bgIndex = i; bg = { ...sw.color }; colorsUI() }); target.append(b) }); colorsUI() }
function setLayer(partial) { const layer = state.document.layers.find(v => v.id === state.layer); if (layer) commit({ kind: 'set-layer', layerValue: { ...layer, ...partial } }) }
function renderLayers() {
const target = $('layers'); target.replaceChildren();[...state.document.layers].reverse().forEach(layer => {
    const row = document.createElement('div'); row.className = 'layer-row'; row.classList.toggle('selected', layer.id === state.layer); row.tabIndex = 0; row.setAttribute('role', 'button'); row.setAttribute('aria-label', layer.name);
    const select = () => { if (call({ action: 'select', layer: layer.id })) { renderUI(); paint() } }; row.addEventListener('click', select); row.addEventListener('keydown', e => { if (e.key === 'Enter') select() });
    const eye = makeButton(layer.visible ? '◉' : '○', lang === 'ru' ? 'Видимость' : 'Visibility', e => { e.stopPropagation(); commit({ kind: 'set-layer', layerValue: { ...layer, visible: !layer.visible } }) }, 'eye');
    const sw = document.createElement('canvas'); sw.className = 'layer-swatch'; sw.width = sw.height = 32; const preview = api.thumbnail(layer.id); if (preview) sw.getContext('2d').putImageData(new ImageData(preview, 32, 32), 0, 0); const label = document.createElement('span'); label.className = 'label'; label.textContent = layer.name; label.style.paddingLeft = layer.parent ? '10px' : '0';
    const lock = makeButton(layer.locked ? '▣' : '▫', lang === 'ru' ? 'Блокировка' : 'Lock', e => { e.stopPropagation(); commit({ kind: 'set-layer', layerValue: { ...layer, locked: !layer.locked } }) }, 'lock'); row.append(eye, sw, label, lock); target.append(row)
  }); const layer = state.document.layers.find(v => v.id === state.layer); if (layer) { $('opacity').value = layer.opacity; $('opacity-value').value = Math.round(layer.opacity / 255 * 100) + '%'; $('blend').value = layer.blend; $('alpha-lock').classList.toggle('active', layer.alphaLock) }
}
function cellKey(cell) { return JSON.stringify([cell.layer, cell.frame, cell.channel]); }
function selectedCells() {
  const layers = new Set(state.document.layers.filter(v => v.kind !== 'group').map(v => v.id));
  const frames = new Set(state.document.frames.map(v => v.id));
  for (const [key, cell] of rangeCells) if (!layers.has(cell.layer) || !frames.has(cell.frame) || cell.channel !== state.channel) rangeCells.delete(key);
  return rangeCells.size ? [...rangeCells.values()] : (layers.has(state.layer) ? [{layer: state.layer, frame: state.frame, channel: state.channel}] : []);
}
function selectTimelineCell(layer, frame, event) {
  stopPlayback(); cancelPointer();
  const cell = {layer, frame, channel: state.channel};
  const extend = event.shiftKey && rangeAnchor;
  if (!event.ctrlKey && !event.metaKey) rangeCells.clear();
  if (extend) {
    const layers = state.document.layers, frames = state.document.frames;
    const a = layers.findIndex(v => v.id === rangeAnchor.layer), b = layers.findIndex(v => v.id === layer);
    const x = frames.findIndex(v => v.id === rangeAnchor.frame), y = frames.findIndex(v => v.id === frame);
    if (a >= 0 && x >= 0) for (let li = Math.min(a,b); li <= Math.max(a,b); li++) if (layers[li].kind !== 'group') {
      for (let fi = Math.min(x,y); fi <= Math.max(x,y); fi++) {
        const v = {layer: layers[li].id, frame: frames[fi].id, channel: state.channel}; rangeCells.set(cellKey(v),v);
      }
    }
  } else if (rangeCells.has(cellKey(cell)) && (event.ctrlKey || event.metaKey)) { rangeCells.delete(cellKey(cell)); }
  else if (state.document.layers.find(v => v.id === layer)?.kind !== 'group') { rangeCells.set(cellKey(cell),cell); rangeAnchor = cell; }
  if (call({action:'select', layer, frame})) { renderUI(); paint(); }
}
function renderTimeline() {
  const table = $('timeline-table'); table.replaceChildren(); const selected = new Set(selectedCells().map(cellKey));
  const head = document.createElement('tr'), corner = document.createElement('th'); corner.textContent = t('layers'); head.append(corner);
  const frames = state.document.frames;
  frames.forEach((f, i) => { const th = document.createElement('th'); const b = makeButton(String(i + 1), `${i + 1} · ${f.durationMS} ms`, e => selectTimelineCell(state.layer, f.id, e)); b.dataset.frame = f.id; b.classList.toggle('active', f.id === (playFrame || state.frame)); th.append(b); head.append(th); });
  const thead = document.createElement('thead'); thead.append(head); table.append(thead); const tbody = document.createElement('tbody');
  [...state.document.layers].reverse().forEach(layer => {
    const row = document.createElement('tr'), label = document.createElement('td'); label.textContent = layer.name; row.append(label);
    frames.forEach((f,i) => {
      const slot = {layer:layer.id,frame:f.id,channel:state.channel};
      const cell = state.document.cels.find(c => c.frame === f.id && c.layer === layer.id && c.channel === state.channel);
      const linked = cell && state.document.cels.filter(c => c.image === cell.image).length > 1;
      const td = document.createElement('td'), b = makeButton(cell ? (linked ? '∞' : '●') : '', `${layer.name} · ${i + 1}${linked ? ' · linked' : ''}`, e => selectTimelineCell(layer.id, f.id, e));
      b.classList.toggle('active', state.frame === f.id && state.layer === layer.id); b.classList.toggle('range-selected', selected.has(cellKey(slot))); b.classList.toggle('linked', !!linked);
      b.dataset.cel = cell?.id || ''; b.dataset.layer = layer.id; b.dataset.frame = f.id; b.setAttribute('aria-selected',String(selected.has(cellKey(slot)))); b.disabled = layer.kind === 'group'; td.append(b); row.append(td);
    }); tbody.append(row);
  }); table.append(tbody);
  const index = frames.findIndex(f => f.id === state.frame); $('duration').value = frames[index]?.durationMS || 100;
  $('frame-info').textContent = `${index + 1} / ${frames.length} · ${frames.reduce((n, f) => n + f.durationMS, 0)} ms`;
  const value = $('tags').value, tags = $('tags'); tags.replaceChildren(new Option(t('all-frames'), '')); (state.document.tags || []).forEach(tag => tags.add(new Option(`${tag.name} · ${tag.direction}`, tag.id))); if ([...tags.options].some(v => v.value === value)) tags.value = value;
  $('delete-frame').disabled = frames.length === 1; $('unlink').disabled = !activeCel(); $('range-info').textContent = rangeCells.size ? `${rangeCells.size} ${t('range-selected')}` : t('range-hint');
  $('frame-left').disabled = index === 0; $('frame-right').disabled = index === frames.length - 1;
  $('edit-tag').disabled = $('delete-tag').disabled = !tags.value;
}
function renderUI() { if (!state) return; renderTabs(); const c = state.document.canvas; $('document-info').textContent = `${c.width} × ${c.height} · ${c.mode.toUpperCase()}`; $('mode').value = c.mode; $('channel').value = state.channel; $('undo').disabled = state.undo === 0 && !state.gesture; $('redo').disabled = state.redo === 0; $('selection-info').textContent = state.selectedPixels ? `${state.selectedPixels} ${t('selected')}` : ''; renderPalette(); renderLayers(); renderTimeline(); selectionOverlay(); document.title = `${state.dirty ? '• ' : ''}${filename} — Aego Asset Studio` }
function fitZoom() { const c = state.document.canvas; const v = $('viewport'); setZoom(Math.max(1, Math.floor(Math.min((v.clientWidth - 70) / c.width, (v.clientHeight - 70) / c.height)))) }
function setZoom(value) { zoom = Math.max(1, Math.min(64, value)); if (state) { const c = state.document.canvas; const stage = $('stage'); stage.style.width = c.width * zoom + 'px'; stage.style.height = c.height * zoom + 'px'; $('canvas').style.width = stage.style.width; $('canvas').style.height = stage.style.height; $('pixel-grid').style.backgroundSize = `${zoom}px ${zoom}px`; $('pixel-grid').style.display = grid && zoom >= 5 ? 'block' : 'none'; $('zoom').textContent = zoom * 100 + '%'; selectionOverlay() } safeSet('aego.art.zoom', String(zoom)) }
function selectionOverlay() { if (!state) return; const box = $('selection-box'); const r = state.selection; const visible = state.selectedPixels > 0; box.classList.toggle('hidden', !visible); if (visible) { box.style.left = r.Min.X * zoom + 'px'; box.style.top = r.Min.Y * zoom + 'px'; box.style.width = (r.Max.X - r.Min.X) * zoom + 'px'; box.style.height = (r.Max.Y - r.Min.Y) * zoom + 'px' } }
function paint(frame = playFrame || state?.frame) {
if (!api || !state) return; const c = state.document.canvas; const bytes = api.pixels(frame, onion && !playing, true); if (!bytes) { say('art: preview failed', true); return } const canvas = $('canvas'); if (canvas.width !== c.width || canvas.height !== c.height) { canvas.width = c.width; canvas.height = c.height; setZoom(zoom) } canvas.getContext('2d').putImageData(new ImageData(bytes, c.width, c.height), 0, 0);
  const pv = $('preview'); pv.width = c.width; pv.height = c.height; const clean = api.pixels(frame, false, false); if (clean) pv.getContext('2d').putImageData(new ImageData(clean, c.width, c.height), 0, 0); pv.style.width = c.width * 4 + 'px'; pv.style.height = c.height * 4 + 'px'; selectionOverlay()
}
function chooseFrame(id) { stopPlayback(); if (call({ action: 'select', frame: id })) { renderUI(); paint() } }
function stopPlayback() { playing = false; playFrame = ''; $('play').textContent = '▶'; $('play').classList.remove('active') }
function togglePlay() { if (!api) return; cancelPointer(); if (playing) { stopPlayback(); renderTimeline(); paint(); return } playing = true; playStart = performance.now(); $('play').textContent = 'Ⅱ'; $('play').classList.add('active'); requestAnimationFrame(tick) }
function tick(now) { if (!playing) return; if (now - lastDraw >= 12) { const res = call({ action: 'frame-at', tag: $('tags').value, timeMS: Math.floor(now - playStart) }, { quiet: true, update: false }); if (res && res.id !== playFrame) { playFrame = res.id; paint(playFrame); $('timeline-table').querySelectorAll('th button[data-frame]').forEach(b => b.classList.toggle('active', b.dataset.frame === playFrame)) } lastDraw = now } requestAnimationFrame(tick) }
function point(e) { const r = $('canvas').getBoundingClientRect(); return { X: Math.floor((e.clientX - r.left) / zoom), Y: Math.floor((e.clientY - r.top) / zoom) } }
function showCursor(p) { lastPoint = p; $('coordinates').textContent = `${p.X}, ${p.Y}`; const cur = $('cursor'); const size = Number($('brush-size').value) || 1; cur.classList.remove('hidden'); cur.style.width = size * zoom + 'px'; cur.style.height = size * zoom + 'px'; cur.style.left = (p.X - Math.floor(size / 2)) * zoom + 'px'; cur.style.top = (p.Y - Math.floor(size / 2)) * zoom + 'px' }
function cancelPointer() { if (!api) return; if (state?.gesture) call({ action: 'cancel' }, { quiet: true }); if (pointer !== null && $('canvas').hasPointerCapture(pointer)) $('canvas').releasePointerCapture(pointer); pointer = null; pan = null; selectionPath = []; if (state) paint() }
const canvas = $('canvas');
canvas.addEventListener('pointerdown', e => {
  if (!api) return; e.preventDefault(); canvas.focus(); const p = point(e); showCursor(p);
  if (e.pointerType === 'touch') { touches.set(e.pointerId, { x: e.clientX, y: e.clientY }); if (touches.size === 2) { cancelPointer(); const [a, b] = [...touches.values()]; pinch = { distance: Math.hypot(a.x - b.x, a.y - b.y), zoom }; return } }
  if (pointer !== null || pinch) return; pointer = e.pointerId; pointerStart = p; canvas.setPointerCapture(pointer); stopPlayback();
  if (e.button === 1 || e.button === 2 && e.altKey) { pan = { x: e.clientX, y: e.clientY, left: $('viewport').scrollLeft, top: $('viewport').scrollTop }; return }
  if (tool === 'eyedropper') { const res = call({ action: 'sample', points: [p] }); if (res?.color) { fg = res.color; colorsUI() } pointer = null; return }
  if (tool === 'wand') { if (call({ action: 'selection', tool: 'wand', points: [p], tolerance: 0 })) { renderUI(); paint() } pointer = null; return }
  if (tool === 'fill') { commit({ kind: 'fill', points: [p], brush: brush() }); pointer = null; return }
  if (tool === 'polygon' || tool === 'curve') { construction.push(p); say(tool === 'polygon' ? t('polyHint') : `${t('curveHint')} · ${construction.length}/4`); if (tool === 'curve' && construction.length === 4) { commit({ kind: 'curve', points: construction, brush: brush() }); construction = [] } pointer = null; return }
  if (tool.startsWith('select-') || tool === 'lasso') { selectionPath = [p]; return }
  if (tool === 'move') { return }
  const b = brush(); if (e.button === 2) { [b.paint, b.background] = [b.background, b.paint] }
  if (call({ action: 'begin', tool, points: [p], brush: b, filled: $('filled').checked }, { quiet: true })) paint(); else cancelPointer()
});
canvas.addEventListener('pointermove', e => {
  if (!api) return; if (e.pointerType === 'touch' && touches.has(e.pointerId)) { touches.set(e.pointerId, { x: e.clientX, y: e.clientY }); if (pinch && touches.size >= 2) { const [a, b] = [...touches.values()]; setZoom(Math.round(pinch.zoom * Math.hypot(a.x - b.x, a.y - b.y) / Math.max(1, pinch.distance))); return } }
  const p = point(e); showCursor(p); if (pointer !== e.pointerId) return;
  if (pan) { $('viewport').scrollLeft = pan.left - (e.clientX - pan.x); $('viewport').scrollTop = pan.top - (e.clientY - pan.y); return }
  if (tool === 'lasso') { selectionPath.push(p); return }
  if (tool.startsWith('select-')) { selectionPath = [pointerStart, p]; return }
  if (tool === 'move') return;
  const events = e.getCoalescedEvents?.(); const points = (events?.length ? events : [e]).map(point); if (call({ action: 'move', points }, { quiet: true })) { paint() }
});
function finishPointer(e, cancel = false) {
if (e.pointerType === 'touch') { touches.delete(e.pointerId); if (touches.size < 2) pinch = null } if (pointer !== e.pointerId) return;
  if (cancel) { cancelPointer(); renderUI(); return } const p = point(e);
  if (pan) { pan = null } else if (tool.startsWith('select-') || tool === 'lasso') { const points = tool === 'lasso' ? selectionPath.concat([p]) : [pointerStart, p]; call({ action: 'selection', tool: tool === 'select-rect' ? 'rectangle' : tool === 'select-ellipse' ? 'ellipse' : 'lasso', points }); }
  else if (tool === 'move') { commit({ kind: 'move', points: [{ X: p.X - pointerStart.X, Y: p.Y - pointerStart.Y }] }) }
  else { call({ action: 'move', points: [p] }, { quiet: true }); call({ action: 'end' }); scheduleRecovery() }
  const id = pointer; pointer = null; if (canvas.hasPointerCapture(id)) canvas.releasePointerCapture(id); renderUI(); paint()
}
canvas.addEventListener('pointerup', e => finishPointer(e)); canvas.addEventListener('pointercancel', e => finishPointer(e, true)); canvas.addEventListener('lostpointercapture', e => { if (pointer === e.pointerId) finishPointer(e, true) });
canvas.addEventListener('pointerleave', () => { if (pointer === null) $('cursor').classList.add('hidden') }); canvas.addEventListener('contextmenu', e => e.preventDefault());
$('viewport').addEventListener('wheel', e => { if (!api || e.shiftKey) return; e.preventDefault(); setZoom(zoom + (e.deltaY < 0 ? 1 : -1)) }, { passive: false });
function undo() { cancelPointer(); stopPlayback(); if (call({ action: 'undo' })) { renderUI(); paint(); scheduleRecovery() } }
function redo() { cancelPointer(); stopPlayback(); if (call({ action: 'redo' })) { renderUI(); paint(); scheduleRecovery() } }
function download(bytes, name, type) { const url = URL.createObjectURL(new Blob([bytes], { type })); const link = document.createElement('a'); link.href = url; link.download = name; link.click(); setTimeout(() => URL.revokeObjectURL(url), 1000) }
function stem() { return filename.replace(/\.[^.]+$/, '') }
async function save() {
  cancelPointer();
  const savedName = filename;
  const res = api.exportBytes('aeart');
  if (res.error) { say(res.error, true); return; }
  let receipt = res.receipt;
  try {
    if (window.isSecureContext && typeof window.showSaveFilePicker === 'function') {
      const handle = await window.showSaveFilePicker({
        suggestedName: savedName,
        types: [{ description: 'Aego Art Document', accept: { 'application/octet-stream': ['.aeart'] } }]
      });
      // No late publication after Close or a stale file-picker result. While
      // writing, this tab is pinned; new edits remain dirty after completion.
      if (!call({ action: 'begin-save', receipt }, { quiet: true })) return;
      const writable = await handle.createWritable();
      try { await writable.write(res.bytes); await writable.close(); }
      catch (error) { await writable.abort().catch(() => {}); throw error; }
      if (!call({ action: 'saved', receipt }, { quiet: true })) return;
      receipt = '';
      call({ action: 'document-name', handle: res.handle, name: handle.name || savedName }, { quiet: true });
    } else {
      download(res.bytes, savedName, 'application/octet-stream');
      // Browsers do not acknowledge whether an anchor download reached disk.
      // Keep dirty and recovery until an actual write acknowledgement exists.
    }
    renderUI(); say(t('saved')); scheduleRecovery();
  } catch (error) { if (error.name !== 'AbortError') say(error.message, true); }
  finally { if (receipt) call({ action: 'cancel-save', receipt }, { quiet: true }); }
}

function exportPNG() { cancelPointer(); const res = api.exportBytes('png'); if (res.error) { say(res.error, true); return } download(res.bytes, stem() + '.png', 'image/png'); say(t('exported')) }
function exportSheet() { cancelPointer(); const columns = Math.min(8, state.document.frames.length); const png = api.exportBytes('sheet', columns); const meta = api.exportBytes('sheet-json', columns); if (png.error || meta.error) { say(png.error || meta.error, true); return } download(png.bytes, stem() + '-sheet.png', 'image/png'); download(meta.bytes, stem() + '-sheet.json', 'application/json'); say(t('exported')) }
async function openFile(file) {
  if (file.size > 64 * 1024 * 1024) throw new Error(t('fileLimit'));
  cancelPointer(); stopPlayback(); captureView();
  const format = /\.(ase|aseprite)$/i.test(file.name) ? 'aseprite' : /\.png$/i.test(file.name) ? 'png' : 'aeart';
  const bytes = new Uint8Array(await file.arrayBuffer());
  const name = file.name.replace(/\.(png|ase|aseprite)$/i, '.aeart');
  let res = JSON.parse(api.importBytes(bytes, format, false, name));
  if (res.needsConfirmation) {
    const message = lang === 'ru' ? 'Некоторые метаданные не будут сохранены. Импортировать после просмотра предупреждений?' : 'Some metadata will not be retained. Import after reviewing these warnings?';
    if (!confirm(message + '\n\n' + (res.warnings || []).join('\n'))) return;
    res = JSON.parse(api.importBytes(bytes, format, false, name, true));
  }
  if (res.error) throw new Error(res.error);
  adopt(res); renderUI(); restoreView(); paint(); scheduleRecovery();
  say(res.warnings?.length ? res.warnings.join(' · ') : t('imported'));
}
$('file-input').addEventListener('change', async e => { try { await openFile(e.target.files[0]) } catch (error) { say(error.message, true) } e.target.value = '' });
$('new').onclick = () => { if (api) { cancelPointer(); $('new-dialog').showModal() } };
$('new-dialog').addEventListener('close', () => { if ($('new-dialog').returnValue !== 'create') return; const form = $('new-dialog').querySelector('form'); const res = call({ action: 'new', width: Number(form.elements.width.value), height: Number(form.elements.height.value), mode: form.elements.mode.value, confirm: true }); if (res) { renderUI(); restoreView(); paint(); scheduleRecovery() } });
$('open').onclick = () => api && $('file-input').click(); $('save').onclick = () => api && save(); $('undo').onclick = undo; $('redo').onclick = redo; $('export-png').onclick = exportPNG; $('export-sheet').onclick = exportSheet;
$('zoom-in').onclick = () => setZoom(zoom + 1); $('zoom-out').onclick = () => setZoom(zoom - 1); $('fit').onclick = fitZoom; $('grid').onclick = () => { grid = !grid; $('grid').classList.toggle('active', grid); setZoom(zoom) };
$('mirror-x').onclick = () => { mirrorX = !mirrorX; $('mirror-x').classList.toggle('active', mirrorX) }; $('mirror-y').onclick = () => { mirrorY = !mirrorY; $('mirror-y').classList.toggle('active', mirrorY) };
$('channel').onchange = e => { if (call({ action: 'select', channel: e.target.value })) { renderUI(); paint() } }; $('mode').onchange = e => commit({ kind: 'mode', mode: e.target.value });
$('fg-color').oninput = e => { fg = {...parseColor(e.target.value), A: fg.A}; colorsUI() }; $('hex-color').onchange = e => { const c = parseColor(e.target.value); if (c) { fg = {...c, A: fg.A}; colorsUI() } else { say(t('badColor'), true); colorsUI() } };
$('swap-colors').onclick = () => { [fg, bg] = [bg, fg];[fgIndex, bgIndex] = [bgIndex, fgIndex]; colorsUI() }; $('palette-edit').onclick = () => commit({ kind: 'palette-color', index: fgIndex, swatch: { color: fg, name: state.document.palette[fgIndex]?.name || '' } });
$('add-layer').onclick = () => openLayerDialog(null);
$('layer-properties').onclick = () => openLayerDialog(state.document.layers.find(v => v.id === state.layer));
$('duplicate-layer').onclick = () => commit({ kind: 'duplicate-layer' }); $('delete-layer').onclick = () => commit({ kind: 'delete-layer' }); $('layer-up').onclick = () => commit({ kind: 'move-layer', offset: 1 }); $('layer-down').onclick = () => commit({ kind: 'move-layer', offset: -1 });
$('opacity').oninput = e => $('opacity-value').value = Math.round(Number(e.target.value) / 255 * 100) + '%'; $('opacity').onchange = e => setLayer({ opacity: Number(e.target.value) }); $('blend').onchange = e => setLayer({ blend: e.target.value }); $('alpha-lock').onclick = () => { const l = state.document.layers.find(v => v.id === state.layer); setLayer({ alphaLock: !l.alphaLock }) };
$('flip-x').onclick = () => commit({ kind: 'flip-x' }); $('rotate').onclick = () => commit({ kind: 'rotate-cw' }); $('onion').onchange = e => { onion = e.target.checked; paint() }; $('play').onclick = togglePlay;
$('first-frame').onclick = () => chooseFrame(state.document.frames[0].id); $('next-frame').onclick = () => { const f = state.document.frames; chooseFrame(f[(f.findIndex(v => v.id === state.frame) + 1) % f.length].id) };
$('add-frame').onclick = () => commit({ kind: 'add-frame', durationMS: Number($('duration').value) }); $('duplicate-frame').onclick = () => commit({ kind: 'duplicate-frame', linked: false }); $('link-frame').onclick = () => commit({ kind: 'duplicate-frame', linked: true }); $('unlink').onclick = () => commit({ kind: 'unlink' }); $('delete-frame').onclick = () => commit({ kind: 'delete-frame' }); $('duration').onchange = e => commit({ kind: 'duration', durationMS: Number(e.target.value) });
function openTagDialog(tag) {
  cancelPointer(); editingTag = tag || null; const f = $('tag-dialog').querySelector('form'), frames = state.document.frames;
  f.elements.name.value = tag?.name || 'Idle'; f.elements.from.value = tag ? frames.findIndex(v => v.id === tag.from)+1 : 1;
  f.elements.to.value = tag ? frames.findIndex(v => v.id === tag.to)+1 : frames.length;
  f.elements.direction.value = tag?.direction || 'forward'; f.elements.from.max = f.elements.to.max = frames.length; openVersionedDialog('tag-dialog');
}
$('add-tag').onclick = () => openTagDialog(null);
$('edit-tag').onclick = () => openTagDialog(state.document.tags.find(v => v.id === $('tags').value));
$('delete-tag').onclick = () => { if ($('tags').value) commit({kind:'delete-tag',tag:{id:$('tags').value}}); };
$('tags').onchange = () => { renderTimeline(); };
$('tag-dialog').addEventListener('close', () => {
  if ($('tag-dialog').returnValue !== 'create' || !dialogCurrent('tag-dialog')) return;
  const f = $('tag-dialog').querySelector('form'), frames = state.document.frames;
  const from = frames[Number(f.elements.from.value)-1], to = frames[Number(f.elements.to.value)-1]; if (!from || !to) return;
  const res = commit({kind:'tag',tag:{id:editingTag?.id || '',name:f.elements.name.value,from:from.id,to:to.id,direction:f.elements.direction.value}});
  if (res) { $('tags').value = res.id; renderTimeline(); }
});
function openVersionedDialog(id) {
  cancelPointer(); stopPlayback(); dialogRevisions.set(id,{handle:activeDocument,revision:state.document.revision});
  $(id).returnValue = ''; $(id).showModal();
}
function dialogCurrent(id) {
  const expected = dialogRevisions.get(id); dialogRevisions.delete(id);
  const current = call({action:'state'},{quiet:true});
  if (!current || !expected || expected.handle !== activeDocument || expected.revision !== state.document.revision) { say(t('stale-dialog'),true); return false; }
  return true;
}
function openLayerDialog(layer) {
  const f = $('layer-dialog').querySelector('form'); editingLayer = layer || null;
  f.elements.name.value = layer?.name || `${t('layer')} ${state.document.layers.length+1}`;
  f.elements.kind.value = layer?.kind || 'raster'; f.elements.kind.disabled = !!layer;
  f.elements.parent.replaceChildren(new Option(t('no-group'),''));
  state.document.layers.filter(v => v.kind === 'group' && v.id !== layer?.id).forEach(v => f.elements.parent.add(new Option(v.name,v.id)));
  f.elements.parent.value = layer?.parent || ''; openVersionedDialog('layer-dialog');
}
$('layer-dialog').addEventListener('close',() => {
  if ($('layer-dialog').returnValue !== 'apply' || !dialogCurrent('layer-dialog')) return;
  const f = $('layer-dialog').querySelector('form');
  commit(editingLayer ? {kind:'set-layer',layerValue:{...editingLayer,name:f.elements.name.value,parent:f.elements.parent.value}} : {kind:'add-layer',name:f.elements.name.value,layerKind:f.elements.kind.value,parent:f.elements.parent.value});
});
$('fg-alpha').onchange = e => { fg.A = Math.max(0,Math.min(255,Number(e.target.value))); colorsUI(); };
$('palette-import').onclick = () => $('palette-input').click();
$('palette-input').onchange = async e => {
  const file = e.target.files[0], handle = activeDocument, revision = state.document.revision; e.target.value = ''; if (!file) return;
  try {
    if (file.size > 128*1024) throw Error('Palette exceeds 128 KiB');
    const bytes = new Uint8Array(await file.arrayBuffer()), format = /\.gpl$/i.test(file.name) ? 'gpl' : 'json';
    const res = JSON.parse(api.importPalette(bytes,format,false,handle,revision));
    if (res.error) { say(res.error,true); return; } adopt(res); renderUI(); paint(); scheduleRecovery(); say(t('palette-loaded'));
  } catch (error) { say(error.message,true); }
};
$('palette-export').onclick = () => { const out = api.exportBytes('palette'); if (out.error) say(out.error,true); else download(out.bytes,stem()+'-palette.json','application/json'); };
$('palette-extract').onclick = () => openVersionedDialog('palette-dialog');
$('palette-dialog').addEventListener('close',() => {
  if ($('palette-dialog').returnValue !== 'apply' || !dialogCurrent('palette-dialog')) return;
  const f = $('palette-dialog').querySelector('form'); commit({kind:'palette-extract',colorLimit:Number(f.elements.limit.value),quantize:f.elements.quantize.checked});
});
$('palette-order').onclick = () => {
  const f = $('palette-order-dialog').querySelector('form'); f.elements.order.value = state.document.palette.map((_,i) => i).join(', '); f.elements.recolor.checked = false;
  openVersionedDialog('palette-order-dialog');
};
$('palette-order-dialog').addEventListener('close',() => {
  if ($('palette-order-dialog').returnValue !== 'apply' || !dialogCurrent('palette-order-dialog')) return;
  const f = $('palette-order-dialog').querySelector('form'), tokens = f.elements.order.value.split(',').map(v=>v.trim());
  if (!tokens.every(v=>/^\d+$/.test(v))) { say(t('invalid-order'),true); return; }
  commit({kind:f.elements.recolor.checked ? 'palette-remap' : 'palette-reorder',paletteOrder:tokens.map(Number)});
});
$('range-transfer').onclick = () => { pendingRange = selectedCells(); if (pendingRange.length) openVersionedDialog('range-dialog'); };
$('range-dialog').addEventListener('close',() => {
  if ($('range-dialog').returnValue !== 'apply' || !dialogCurrent('range-dialog')) return;
  const f = $('range-dialog').querySelector('form'), layerOffset=Number(f.elements.layers.value), frameOffset=Number(f.elements.frames.value);
  const layers = state.document.layers, frames = state.document.frames;
  const dest = pendingRange.map(v=>({layer:layers[layers.findIndex(l=>l.id===v.layer)+layerOffset]?.id,frame:frames[frames.findIndex(fr=>fr.id===v.frame)+frameOffset]?.id,channel:v.channel}));
  const res = commit({kind:'cel-range',cells:pendingRange,layerOffset,frameOffset,copy:f.elements.operation.value!=='move',linked:f.elements.operation.value==='link',overwrite:f.elements.overwrite.checked});
  if (res) { rangeCells=new Map(dest.map(v=>[cellKey(v),v])); if (dest.length) call({action:'select',layer:dest[0].layer,frame:dest[0].frame}); renderUI(); paint(); }
});
$('copy-animation').onclick = () => {
  const selected = new Set(selectedCells().map(v=>v.frame));
  const frames = state.document.frames.filter(v=>selected.has(v.id)).map(v=>v.id);
  if (call({action:'copy-animation',frames})) say(t('copy'));
};
$('paste-animation').onclick = () => { cancelPointer(); stopPlayback(); if(call({action:'paste-animation',expectedRevision:state.document.revision})) { rangeCells.clear(); renderUI(); paint(); scheduleRecovery(); } };
function shiftFrame(offset) {
  const frames = state.document.frames.map(v=>v.id), i=frames.indexOf(state.frame), j=i+offset;
  if (j<0 || j>=frames.length) return; [frames[i],frames[j]]=[frames[j],frames[i]]; commit({kind:'frame-order',frameOrder:frames});
}
$('frame-left').onclick = () => shiftFrame(-1); $('frame-right').onclick = () => shiftFrame(1);
$('language').onclick = () => { lang = lang === 'ru' ? 'en' : 'ru'; translate() }; $('fullscreen').onclick = async () => { try { if (document.fullscreenElement) await document.exitFullscreen(); else await document.documentElement.requestFullscreen() } catch (error) { say(error.message, true) } };
$('toggle-side').onclick = () => $('sidebar').classList.toggle('open');
function workspace(name) { document.body.classList.toggle('workspace-animation', name === 'animation'); document.querySelectorAll('[data-workspace]').forEach(b => b.classList.toggle('active', b.dataset.workspace === name)); safeSet('aego.art.workspace', name) }
document.querySelectorAll('[data-workspace]').forEach(b => b.onclick = () => workspace(b.dataset.workspace)); workspace(safeGet('aego.art.workspace') || 'art');
document.addEventListener('keydown', e => {
  if (!api || e.target.matches('input,textarea,select') || document.querySelector('dialog[open]')) return; const k = e.key.toLowerCase(), mod = e.ctrlKey || e.metaKey;
  if (e.key === 'Escape') { e.preventDefault(); construction = []; cancelPointer(); call({ action: 'selection', tool: 'none' }); renderUI(); paint(); return }
  if (mod) { if (k === 'z') { e.preventDefault(); e.shiftKey ? redo() : undo() } else if (k === 'y') { e.preventDefault(); redo() } else if (k === 's') { e.preventDefault(); save() } else if (k === 'o') { e.preventDefault(); $('file-input').click() } else if (k === 'c') { e.preventDefault(); if (call({ action: 'copy' })) say(t('copy')) } else if (k === 'v') { e.preventDefault(); if (call({ action: 'paste', points: [lastPoint] })) { renderUI(); paint(); scheduleRecovery() } } else if (k === 'a') { e.preventDefault(); call({ action: 'selection', tool: 'all' }); renderUI(); paint() } else if (k === 'd') { e.preventDefault(); call({ action: 'selection', tool: 'none' }); renderUI(); paint() } return }
  if (e.key === 'Enter') { e.preventDefault(); if (tool === 'polygon' && construction.length >= 2) { commit({ kind: 'polygon', points: construction, brush: brush(), filled: $('filled').checked }); construction = [] } else togglePlay(); return }
  if (e.key === 'Delete' || e.key === 'Backspace') { e.preventDefault(); commit({ kind: 'clear' }); return }
  if (k === 'x') { $('swap-colors').click(); return } const entry = Object.entries(toolKeys).find(([, key]) => key.toLowerCase() === k); if (entry) { e.preventDefault(); selectTool(entry[0]) }
});
window.addEventListener('beforeunload', e => { if (tabs.some(v => v.dirty) || state?.gesture) { e.preventDefault(); e.returnValue = '' } });
window.addEventListener('dragover', e => e.preventDefault()); window.addEventListener('drop', async e => { e.preventDefault(); if (api && e.dataTransfer.files.length) try { await openFile(e.dataTransfer.files[0]) } catch (error) { say(error.message, true) } });
function queueRecovery(job) {
  recoveryWrites = recoveryWrites.then(job).catch(error => { say(t('recoveryError') + ': ' + error.message, true); window.dispatchEvent(new Event('aego-art-recovery-error')); });
  return recoveryWrites;
}
function scheduleRecovery() {
  if (!recoveryStore || !api) return;
  clearTimeout(recoveryTimer);
  recoveryTimer = setTimeout(flushRecovery, 400);
}
function flushRecovery() {
  if (!recoveryStore || !api) return;
  const status = JSON.parse(api.request('{"action":"state"}'));
  for (const tab of status.tabs || []) {
    let key = recoveryKeys.get(tab.key);
    if (!key) { key = `${browserSession}/${tab.key}`; recoveryKeys.set(tab.key, key); }
    if (!tab.dirty) { queueRecovery(() => recoveryStore.remove(key)); continue; }
    const res = api.exportBytes('recovery', 8, tab.key);
    if (res.error) continue; // A live gesture will be retried on pointerup.
    const snapshot = { key, bytes: new Uint8Array(res.bytes), filename: tab.name, revision: res.revision, dirty: true, at: Date.now() };
    queueRecovery(() => recoveryStore.put(snapshot));
  }
}
async function showRecovery() {
  if (!recoveryStore) return;
  const opened = new Set(recoveryKeys.values());
  recoveryData = (await recoveryStore.list()).filter(v => !opened.has(v.key) && v.dirty && v.bytes);
  $('recovery-choice').replaceChildren(...recoveryData.map(v => new Option(`${v.filename} · ${new Date(v.at).toLocaleString()}`, v.key)));
  $('recovery').classList.toggle('hidden', !recoveryData.length);
}
$('restore').onclick = async () => {
  const record = recoveryData.find(v => v.key === $('recovery-choice').value);
  if (!record) return;
  cancelPointer(); stopPlayback();
  const res = JSON.parse(api.importBytes(record.bytes, 'aeart', false, record.filename || 'Recovered.aeart'));
  if (res.error) { say(res.error, true); return; }
  adopt(res); recoveryKeys.set(activeDocument, record.key); call({ action: 'recovered' });
  renderUI(); restoreView(); paint(); await showRecovery(); scheduleRecovery();
};
$('dismiss-recovery').onclick = async () => {
  const key = $('recovery-choice').value;
  if (!key || !confirm(lang === 'ru' ? 'Удалить эту резервную копию без восстановления?' : 'Discard this recovery copy without restoring it?')) return;
  await queueRecovery(() => recoveryStore.remove(key)); await showRecovery();
};
window.addEventListener('pagehide', flushRecovery);
(async () => { translate(); try { const go = new Go(); const response = await fetch(window.AegoStudioRelease?.wasmURL || 'artstudio.wasm'); if (!response.ok) throw new Error(`WASM HTTP ${response.status}`); let instance; try { ({ instance } = await WebAssembly.instantiateStreaming(response.clone(), go.importObject)) } catch { ({ instance } = await WebAssembly.instantiate(await response.arrayBuffer(), go.importObject)) } go.run(instance).catch(error => say(error.message, true)); api = window.aegoArt; if (!api) throw new Error('Go bridge did not initialize'); adopt(JSON.parse(api.request('{"action":"state"}'))); $('loading').classList.add('hidden'); renderUI(); fitZoom(); paint(); say(t('ready')); try { recoveryStore = await AegoRecoveryStore.open(); await showRecovery() } catch (error) { say(t('storageUnavailable'), true) } window.artStudioReady = true } catch (error) { $('loading').textContent = error.message; say(error.message, true) } })();
