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
Object.assign(translations.ru, {'brush-solid':'Цвет','brush-stamp':'Своя кисть','brush-pattern':'Узор','capture-brush':'Из выделения','gpu-light':'Свет','gpu-shadows':'Тени','gpu-rate':'Частиц/с','gpu-restart':'Повторить эффект','template':'Шаблон','blank-document':'Пустой документ','template-help':'Шаблон содержит редактируемые слои, палитру и кадры. Предпросмотр использует соответствующий ParticleSystem2D.'});
Object.assign(translations.en, {'brush-solid':'Solid','brush-stamp':'Custom brush','brush-pattern':'Pattern','capture-brush':'From selection','gpu-light':'Light','gpu-shadows':'Shadows','gpu-rate':'Particles/s','gpu-restart':'Restart effect','template':'Template','blank-document':'Blank document','template-help':'The template contains editable layers, palette and frames. Preview uses its corresponding ParticleSystem2D preset.'});
Object.assign(translations.ru, {'ramp-shade':'Оттенки','ramp-help':'Сдвинуть цвета выделенных пикселей внутри диапазона палитры.','ramp-first':'Первый индекс','ramp-last':'Последний индекс','ramp-step':'Сдвиг','merge-down':'Слить вниз','isolate-layer':'Изоляция','transform':'Преобразовать','scale':'Масштаб','tile-preview':'Мозаика'});
Object.assign(translations.en, {'ramp-shade':'Ramp shade','ramp-help':'Shift selected pixel colors within the chosen palette range.','ramp-first':'First index','ramp-last':'Last index','ramp-step':'Step','merge-down':'Merge down','isolate-layer':'Isolate','transform':'Transform','scale':'Scale','tile-preview':'Tile'});
Object.assign(translations.ru,{'background-checker':'Шахматный','background-light':'Светлый','background-dark':'Тёмный'});
Object.assign(translations.en,{'background-checker':'Checker','background-light':'Light','background-dark':'Dark'});
Object.assign(translations.ru,{'pivot':'Привязка','pivot-help':'Координаты в пикселях текущего кадра. Меняется только точка привязки изображения; коллайдеры не затрагиваются.'});
Object.assign(translations.en,{'pivot':'Pivot','pivot-help':'Pixel coordinates in the current frame. This changes only the image pivot; colliders are unchanged.'});
Object.assign(translations.ru,{'open-busy':'Файл не открыт: завершите или отмените текущий жест и откройте файл снова.','recovery-copy':'Восстановлена отдельная копия. Исходная резервная копия сохранена до явного удаления.','recovery-already':'Эта резервная копия уже восстановлена в этой странице.','recovery-restored':'восстановлено'});
Object.assign(translations.en,{'open-busy':'File not opened: finish or cancel the current gesture, then open the file again.','recovery-copy':'Restored an independent copy. The original recovery copy remains until explicitly discarded.','recovery-already':'This recovery copy has already been restored in this page.','recovery-restored':'restored'});
Object.assign(translations.ru, {'fullscreen':'Полноэкранный режим','duplicate-layer':'Дублировать слой'});
Object.assign(translations.en, {'fullscreen':'Fullscreen','duplicate-layer':'Duplicate layer'});
Object.assign(translations.ru, {'theme':'Тема интерфейса','theme-auto':'Авто','theme-dark':'Тёмная','theme-light':'Светлая'});
Object.assign(translations.en, {'theme':'Interface theme','theme-auto':'Auto','theme-dark':'Dark','theme-light':'Light'});
Object.assign(translations.ru, {
  'workspace-label':'Рабочие пространства','sidebar-label':'Палитра и слои','tools-label':'Инструменты','documents-label':'Документы',
  'brush-source-label':'Источник кисти','symmetry-x':'Горизонтальная симметрия','symmetry-y':'Вертикальная симметрия',
  'recovery-choice-label':'Резервная копия','canvas-label':'Пиксельный холст','background-label':'Фон прозрачности','mode-label':'Режим цвета',
  'foreground-label':'Основной цвет','hex-label':'Цвет в HEX','alpha-label':'Непрозрачность','swap-label':'Поменять основной и фоновый цвета · X',
  'add-layer-label':'Добавить слой','layer-properties-label':'Свойства слоя','delete-layer-label':'Удалить слой','blend-label':'Режим смешивания',
  'alpha-lock-label':'Блокировка прозрачности','layer-up-label':'Поднять слой','layer-down-label':'Опустить слой',
  'preview-mode-label':'Режим предпросмотра','animation-preview-label':'Предпросмотр анимации','gpu-preview-label':'Предпросмотр Aego GPU',
  'first-frame-label':'Первый кадр','play-label':'Воспроизведение / пауза · Enter','next-frame-label':'Следующий кадр',
  'delete-frame-label':'Удалить кадр','frame-left-label':'Переместить кадр влево','frame-right-label':'Переместить кадр вправо',
  'timeline-label':'Слои и кадры','delete-tag-label':'Удалить метку','template-label':'Шаблон эффекта'
});
Object.assign(translations.en, {
  'workspace-label':'Workspaces','sidebar-label':'Palette and layers','tools-label':'Tools','documents-label':'Documents',
  'brush-source-label':'Brush source','symmetry-x':'Horizontal symmetry','symmetry-y':'Vertical symmetry',
  'recovery-choice-label':'Recovery document','canvas-label':'Pixel canvas','background-label':'Transparency background','mode-label':'Color mode',
  'foreground-label':'Foreground color','hex-label':'Hex color','alpha-label':'Alpha','swap-label':'Swap foreground/background · X',
  'add-layer-label':'Add layer','layer-properties-label':'Layer properties','delete-layer-label':'Delete layer','blend-label':'Blend mode',
  'alpha-lock-label':'Alpha lock','layer-up-label':'Move layer up','layer-down-label':'Move layer down',
  'preview-mode-label':'Preview mode','animation-preview-label':'Animation preview','gpu-preview-label':'Aego GPU preview',
  'first-frame-label':'First frame','play-label':'Play / Pause · Enter','next-frame-label':'Next frame',
  'delete-frame-label':'Delete frame','frame-left-label':'Move frame left','frame-right-label':'Move frame right',
  'timeline-label':'Layers by frames','delete-tag-label':'Delete tag','template-label':'Effect template'
});
Object.assign(translations.ru, {
  'workspace-art':'Рисование','workspace-animation':'Анимация','workspace-vfx':'VFX',
  'mode-indexed':'Индексированный','mode-gray':'Оттенки серого',
  'channel-albedo':'Альбедо','channel-normal':'Нормали','channel-height':'Высота','channel-emission':'Свечение',
  'blend-normal':'Обычный','blend-multiply':'Умножение','blend-screen':'Экран','blend-add':'Сложение',
  'preview-pixels':'Пиксели · 4×','preview-sprite':'GPU · Спрайт','preview-lit':'Освещение / нормали',
  'preview-normal':'Канал нормалей','preview-height':'Канал высоты','preview-emission':'Канал свечения',
  'direction-forward':'Вперёд','direction-reverse':'Назад','direction-ping-pong':'Туда и обратно',
  'transform-flip-x':'Отразить X','transform-flip-y':'Отразить Y',
  'transform-rotate-cw':'Повернуть на 90°','transform-rotate-ccw':'Повернуть на −90°',
  'cel-linked':'связанная ячейка','zoom-out-label':'Уменьшить масштаб','zoom-in-label':'Увеличить масштаб',
  'add-tag-label':'Добавить метку','discard-recovery-label':'Удалить выбранную резервную копию'
});
Object.assign(translations.en, {
  'workspace-art':'Art','workspace-animation':'Animation','workspace-vfx':'VFX',
  'mode-indexed':'Indexed','mode-gray':'Grayscale',
  'channel-albedo':'Albedo','channel-normal':'Normal','channel-height':'Height','channel-emission':'Emission',
  'blend-normal':'Normal','blend-multiply':'Multiply','blend-screen':'Screen','blend-add':'Add',
  'preview-pixels':'Pixels · 4×','preview-sprite':'GPU · Sprite','preview-lit':'Lit / Normal',
  'preview-normal':'Normal channel','preview-height':'Height channel','preview-emission':'Emission channel',
  'direction-forward':'Forward','direction-reverse':'Reverse','direction-ping-pong':'Ping-pong',
  'transform-flip-x':'Flip X','transform-flip-y':'Flip Y',
  'transform-rotate-cw':'Rotate 90°','transform-rotate-ccw':'Rotate −90°',
  'cel-linked':'linked cel','zoom-out-label':'Zoom out','zoom-in-label':'Zoom in',
  'add-tag-label':'Add tag','discard-recovery-label':'Discard selected recovery copy'
});
let editingPivot = null;
let tiledPreview = false;
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
const views = new Map(), recoveryKeys = new Map(), recoveryVersions = new Map();
const restoredRecoverySources = new Set();
let recoveryBusy = false;
const browserSession = crypto.randomUUID ? crypto.randomUUID() : `${Date.now()}-${Math.random()}`;
let recoveryWrites = Promise.resolve();
let rangeCells = new Map(), rangeAnchor = null;
const dialogRevisions = new Map();
let editingLayer = null, editingTag = null, pendingRange = [];
let effectTemplates = [];
// Bounded DOM caches only. Go remains the sole source of document/pixel data.
// A stroke does not rebuild an unchanged layers × frames table.
const uiCache = {tabs:'',palette:'',layers:'',timeline:'',tags:'',thumbnails:'',
  layerRows:new Map(),celButtons:new Map(),cellHighlights:new Map()};
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
  const key=JSON.stringify([activeDocument,lang,tabs.map(tab=>[tab.key,tab.name,tab.dirty])]);
  if(uiCache.tabs===key)return;uiCache.tabs=key;
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
  views.delete(tab.key); recoveryKeys.delete(tab.key); recoveryVersions.delete(tab.key);
  if (key && recoveryStore) queueRecovery(() => recoveryStore.remove(key));
  renderUI(); restoreView(); paint();
}

const t = key => translations[lang]?.[key] || translations.en[key] || key;
function safeGet(key) { try { return localStorage.getItem(key) } catch { return null } }
function safeSet(key, value) { try { localStorage.setItem(key, value) } catch {/* Optional UI preferences. */ } }
const systemTheme = typeof window.matchMedia === 'function' ? window.matchMedia('(prefers-color-scheme: light)') : {matches: false};
let themePreference = safeGet('aego.art.theme');
if (!['auto', 'dark', 'light'].includes(themePreference)) themePreference = 'auto';
function applyTheme() {
  document.documentElement.dataset.theme = themePreference === 'auto' ? (systemTheme.matches ? 'light' : 'dark') : themePreference;
  $('theme').value = themePreference;
}
$('theme').addEventListener('change', e => { themePreference = e.target.value; safeSet('aego.art.theme', themePreference); applyTheme(); });
const onSystemTheme = () => { if (themePreference === 'auto') applyTheme(); };
if (systemTheme.addEventListener) systemTheme.addEventListener('change', onSystemTheme);
else if (systemTheme.addListener) systemTheme.addListener(onSystemTheme);
applyTheme();
function say(message, error = false) { $('status').removeAttribute('data-i18n'); $('status').textContent = message; document.querySelector('.statusbar').classList.toggle('error', error) }
function makeButton(label, title, fn, className = '') { const b = document.createElement('button'); b.type = 'button'; b.textContent = label; b.title = title || label; b.className = className; if (fn) b.addEventListener('click', fn); return b }
function translate() {
  document.documentElement.lang = lang;
  $('language').textContent = lang.toUpperCase();
  document.querySelectorAll('[data-i18n]').forEach(el => el.textContent = t(el.dataset.i18n));
  for (const id of ['fullscreen', 'duplicate-layer']) {
    const button = $(id), label = t(id);
    button.title = label;
    button.setAttribute('aria-label', label);
  }
  const labels = {
    '.workspaces':'workspace-label','#toggle-side':'sidebar-label','#tools':'tools-label','#document-tabs':'documents-label',
    '#brush-source':'brush-source-label','#mirror-x':'symmetry-x','#mirror-y':'symmetry-y','#recovery-choice':'recovery-choice-label',
    '#canvas':'canvas-label','#canvas-background':'background-label','#mode':'mode-label','#fg-color':'foreground-label','#hex-color':'hex-label',
    '#fg-alpha':'alpha-label','#swap-colors':'swap-label','#add-layer':'add-layer-label','#layer-properties':'layer-properties-label',
    '#delete-layer':'delete-layer-label','#blend':'blend-label','#alpha-lock':'alpha-lock-label','#layer-up':'layer-up-label','#layer-down':'layer-down-label',
    '#gpu-mode':'preview-mode-label','#preview':'animation-preview-label','#gpu-preview':'gpu-preview-label','#first-frame':'first-frame-label',
    '#play':'play-label','#next-frame':'next-frame-label','#delete-frame':'delete-frame-label','#frame-left':'frame-left-label',
    '#frame-right':'frame-right-label','#timeline-table':'timeline-label','#delete-tag':'delete-tag-label','#new-dialog select[name="template"]':'template-label',
    '#zoom-out':'zoom-out-label','#zoom-in':'zoom-in-label','#add-tag':'add-tag-label','#dismiss-recovery':'discard-recovery-label'
  };
  for (const [selector, key] of Object.entries(labels)) {
    const el = document.querySelector(selector);
    if (!el) continue;
    const label = t(key);
    el.setAttribute('aria-label', label);
    if (el.hasAttribute('title')) el.title = label;
  }
  // These labels are presentation only. Keep every option value, selection,
  // document ID and runtime channel unchanged when the language is switched.
  const text = {
    '[data-workspace="art"]':'workspace-art','[data-workspace="animation"]':'workspace-animation','[data-workspace="vfx"]':'workspace-vfx',
    '#mode option[value="indexed"]':'mode-indexed','#new-dialog [name="mode"] option[value="indexed"]':'mode-indexed',
    '#new-dialog [name="mode"] option[value="grayscale"]':'mode-gray',
    '#channel option[value="albedo"]':'channel-albedo','#channel option[value="normal"]':'channel-normal',
    '#channel option[value="height"]':'channel-height','#channel option[value="emission"]':'channel-emission',
    '#blend option[value="normal"]':'blend-normal','#blend option[value="multiply"]':'blend-multiply',
    '#blend option[value="screen"]':'blend-screen','#blend option[value="add"]':'blend-add',
    '#gpu-mode option[value="pixels"]':'preview-pixels','#gpu-mode option[value="sprite"]':'preview-sprite',
    '#gpu-mode option[value="lit"]':'preview-lit','#gpu-mode option[value="normal"]':'preview-normal',
    '#gpu-mode option[value="height"]':'preview-height','#gpu-mode option[value="emission"]':'preview-emission',
    '#tag-dialog [name="direction"] option[value="forward"]':'direction-forward',
    '#tag-dialog [name="direction"] option[value="reverse"]':'direction-reverse',
    '#tag-dialog [name="direction"] option[value="ping-pong"]':'direction-ping-pong',
    '#transform-dialog [name="kind"] option[value="flip-x"]':'transform-flip-x',
    '#transform-dialog [name="kind"] option[value="flip-y"]':'transform-flip-y',
    '#transform-dialog [name="kind"] option[value="rotate-cw"]':'transform-rotate-cw',
    '#transform-dialog [name="kind"] option[value="rotate-ccw"]':'transform-rotate-ccw'
  };
  for (const [selector, key] of Object.entries(text)) {
    const el = document.querySelector(selector);
    if (el) el.textContent = t(key);
  }
  $('theme').title = t('theme'); $('theme').setAttribute('aria-label', t('theme'));
  buildTools(); renderTemplateOptions(); if (state) renderUI(); safeSet('aego.art.language', lang);
}
function buildTools() { const target = $('tools'); target.replaceChildren(); for (const [name, path] of Object.entries(icons)) { const b = makeButton('', `${t(name)}${toolKeys[name] ? ' · ' + toolKeys[name] : ''}`, () => selectTool(name), 'tool'); b.dataset.tool = name; b.setAttribute('aria-label', t(name)); b.innerHTML = `<svg viewBox="0 0 24 24" aria-hidden="true">${path}</svg><span class="shortcut">${toolKeys[name]}</span>`; b.classList.toggle('active', name === tool); target.append(b) } $('tool-name').textContent = t(tool) }
function selectTool(name) { if (!api) return; cancelPointer(); construction = []; tool = name; buildTools(); if (name === 'polygon') say(t('polyHint')); else if (name === 'curve') say(t('curveHint')); else say(t('ready')) }
function call(req, { quiet = false, update = true } = {}) { if (!api) return null; const res = JSON.parse(api.request(JSON.stringify(req))); if (update) adopt(res); if (res.error) { say(res.error, true); return null } if (!quiet) say(t('ready')); return res }
function brush() { return { source:$('brush-source').value, paint: { color: fg, index: fgIndex, useIndex: state.document.canvas.mode === 'indexed' && state.channel === 'albedo' }, background: { color: bg, index: bgIndex, useIndex: state.document.canvas.mode === 'indexed' && state.channel === 'albedo' }, size: Number($('brush-size').value) || 1, pixelPerfect: $('pixel-perfect').checked, round: $('brush-shape').value === 'round', mirrorX, mirrorY, dither: Number($('brush-dither').value) } }
$('capture-brush').onclick=()=>{cancelPointer();if(call({action:'capture-brush'})){call({action:'selection',tool:'none'});$('brush-source').value='stamp';selectTool('pencil');renderUI();paint();}};
function activeCel() { return state.document.cels.find(c => c.layer === state.layer && c.frame === state.frame && c.channel === state.channel) }
function commit(op, expectedRevision = state.document.revision) { stopPlayback(); const res = call({ action: 'apply', operation: op, expectedRevision }); if (res) { renderUI(); paint(); scheduleRecovery() } return res }
function colorHex(c) { return '#' + [c.R, c.G, c.B].map(v => v.toString(16).padStart(2, '0')).join('') }
function parseColor(v) { if (!/^#[0-9a-f]{6}$/i.test(v)) return null; return { R: parseInt(v.slice(1, 3), 16), G: parseInt(v.slice(3, 5), 16), B: parseInt(v.slice(5, 7), 16), A: 255 } }
function colorsUI() { $('fg-alpha').value = fg.A; $('fg-color').value = colorHex(fg); $('hex-color').value = colorHex(fg); $('swatches').querySelectorAll('button').forEach((b, i) => b.classList.toggle('selected', i === fgIndex)); $('palette-info').textContent = `${state.document.palette.length} ${t('colors')}` }
function renderPalette() {
  const key=JSON.stringify([activeDocument,lang,state.document.canvas.mode,state.channel,state.document.palette]);
  if(uiCache.palette===key){colorsUI();return}uiCache.palette=key;
  fgIndex = Math.min(fgIndex, state.document.palette.length - 1); bgIndex = Math.min(bgIndex, state.document.palette.length - 1);
  if (state.document.canvas.mode === 'indexed' && state.channel === 'albedo') { fg = {...state.document.palette[fgIndex].color}; bg = {...state.document.palette[bgIndex].color}; }
  const target = $('swatches'); target.replaceChildren();
  state.document.palette.forEach((sw, i) => { const b = makeButton('', `${i}: ${sw.name || colorHex(sw.color)}`, () => { fgIndex = i; fg = { ...sw.color }; colorsUI() }, 'swatch'); b.style.backgroundColor = `rgba(${sw.color.R},${sw.color.G},${sw.color.B},${sw.color.A / 255})`; if (sw.color.A === 0) b.classList.add('transparent'); b.addEventListener('contextmenu', e => { e.preventDefault(); bgIndex = i; bg = { ...sw.color }; colorsUI() }); target.append(b) }); colorsUI();
}
function setLayer(partial) { const layer = state.document.layers.find(v => v.id === state.layer); if (layer) commit({ kind: 'set-layer', layerValue: { ...layer, ...partial } }) }
function renderLayers() {
  const key=JSON.stringify([activeDocument,lang,state.document.layers]);
  if(uiCache.layers!==key){
    uiCache.layers=key;uiCache.thumbnails='';uiCache.layerRows.clear();
    const target = $('layers'); target.replaceChildren();[...state.document.layers].reverse().forEach(layer => {
    const row = document.createElement('div'); row.className = 'layer-row'; row.classList.toggle('selected', layer.id === state.layer); row.tabIndex = 0; row.setAttribute('role', 'button'); row.setAttribute('aria-label', layer.name);
    const select = () => { if (call({ action: 'select', layer: layer.id })) { renderUI(); paint() } }; row.addEventListener('click', select); row.addEventListener('keydown', e => { if (e.key === 'Enter') select() });
    const eye = makeButton(layer.visible ? '◉' : '○', lang === 'ru' ? 'Видимость' : 'Visibility', e => { e.stopPropagation(); commit({ kind: 'set-layer', layerValue: { ...layer, visible: !layer.visible } }) }, 'eye');
    const sw = document.createElement('canvas'); sw.className = 'layer-swatch'; sw.width = sw.height = 32; const label = document.createElement('span'); label.className = 'label'; label.textContent = layer.name; label.style.paddingLeft = layer.parent ? '10px' : '0';
    const lock = makeButton(layer.locked ? '▣' : '▫', lang === 'ru' ? 'Блокировка' : 'Lock', e => { e.stopPropagation(); commit({ kind: 'set-layer', layerValue: { ...layer, locked: !layer.locked } }) }, 'lock'); row.append(eye, sw, label, lock); target.append(row);uiCache.layerRows.set(layer.id,{row,sw});
    });
  }
  const thumbnailKey=JSON.stringify([activeDocument,state.document.revision,state.frame,state.channel]);
  for(const [id,{row,sw}] of uiCache.layerRows){
    row.classList.toggle('selected',id===state.layer);
    if(uiCache.thumbnails!==thumbnailKey){const preview=api.thumbnail(id);if(preview)sw.getContext('2d').putImageData(new ImageData(preview,32,32),0,0);}
  }
  uiCache.thumbnails=thumbnailKey;
  const layer = state.document.layers.find(v => v.id === state.layer); if (layer) { $('opacity').value = layer.opacity; $('opacity-value').value = Math.round(layer.opacity / 255 * 100) + '%'; $('blend').value = layer.blend; $('alpha-lock').classList.toggle('active', layer.alphaLock); $('isolate-layer').classList.toggle('active', state.isolation === layer.id) }
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
  const table=$('timeline-table'),frames=state.document.frames;
  const selected=new Set(selectedCells().map(cellKey));
  const topology=JSON.stringify([activeDocument,lang,state.channel,state.document.layers,frames,
    state.document.cels.map(c=>[c.id,c.layer,c.frame,c.channel,c.image])]);
  if(uiCache.timeline!==topology){
    uiCache.timeline=topology;uiCache.celButtons.clear();uiCache.cellHighlights.clear();table.replaceChildren();
    const cells=new Map(),links=new Map();
    for(const c of state.document.cels){cells.set(cellKey(c),c);links.set(c.image,(links.get(c.image)||0)+1);}
    const head=document.createElement('tr'),corner=document.createElement('th');corner.textContent=t('layers');head.append(corner);
    frames.forEach((f,i)=>{const th=document.createElement('th'),b=makeButton(String(i+1),`${i+1} · ${f.durationMS} ms`,e=>selectTimelineCell(state.layer,f.id,e));b.dataset.frame=f.id;th.append(b);head.append(th);});
    const thead=document.createElement('thead');thead.append(head);table.append(thead);const tbody=document.createElement('tbody');
    [...state.document.layers].reverse().forEach(layer=>{
      const row=document.createElement('tr'),label=document.createElement('td');label.textContent=layer.name;row.append(label);
      frames.forEach((f,i)=>{
        const key=cellKey({layer:layer.id,frame:f.id,channel:state.channel}),cell=cells.get(key),linked=cell&&(links.get(cell.image)||0)>1;
        const td=document.createElement('td'),b=makeButton(cell?(linked?'∞':'●'):'',`${layer.name} · ${i+1}${linked?' · '+t('cel-linked'):''}`,e=>selectTimelineCell(layer.id,f.id,e));
        b.classList.toggle('linked',!!linked);b.dataset.cel=cell?.id||'';b.dataset.layer=layer.id;b.dataset.frame=f.id;b.disabled=layer.kind==='group';b.setAttribute('aria-selected','false');
        td.append(b);row.append(td);uiCache.celButtons.set(key,b);
      });tbody.append(row);
    });table.append(tbody);
  }
  table.querySelectorAll('th button[data-frame]').forEach(b=>b.classList.toggle('active',b.dataset.frame===(playFrame||state.frame)));
  const active=cellKey({layer:state.layer,frame:state.frame,channel:state.channel}),highlights=new Map();
  for(const key of selected)highlights.set(key,2);highlights.set(active,(highlights.get(active)||0)|1);
  for(const key of new Set([...uiCache.cellHighlights.keys(),...highlights.keys()])){
    const flags=highlights.get(key)||0;
    if(flags===(uiCache.cellHighlights.get(key)||0))continue;
    const b=uiCache.celButtons.get(key);if(b){b.classList.toggle('active',!!(flags&1));b.classList.toggle('range-selected',!!(flags&2));b.setAttribute('aria-selected',String(!!(flags&2)));}
  }
  uiCache.cellHighlights=highlights;
  const index = frames.findIndex(f => f.id === state.frame); $('duration').value = frames[index]?.durationMS || 100;
  $('frame-info').textContent = `${index + 1} / ${frames.length} · ${frames.reduce((n, f) => n + f.durationMS, 0)} ms`;
  const tagKey=JSON.stringify([activeDocument,lang,state.document.tags]);
  if(uiCache.tags!==tagKey){uiCache.tags=tagKey;const value=$('tags').value,tags=$('tags');tags.replaceChildren(new Option(t('all-frames'),''));(state.document.tags||[]).forEach(tag=>tags.add(new Option(`${tag.name} · ${t('direction-'+tag.direction)}`,tag.id)));if([...tags.options].some(v=>v.value===value))tags.value=value;}
  $('delete-frame').disabled = frames.length === 1; $('unlink').disabled = !activeCel(); $('range-info').textContent = rangeCells.size ? `${rangeCells.size} ${t('range-selected')}` : t('range-hint');
  $('frame-left').disabled = index === 0; $('frame-right').disabled = index === frames.length - 1;
  $('edit-tag').disabled = $('delete-tag').disabled = !$('tags').value;
}
function renderUI() { if (!state) return; renderTabs(); const c = state.document.canvas; $('document-info').textContent = `${c.width} × ${c.height} · ${c.mode.toUpperCase()}`; $('mode').value = c.mode; $('channel').value = state.channel; $('undo').disabled = state.undo === 0 && !state.gesture; $('redo').disabled = state.redo === 0; $('selection-info').textContent = state.selectedPixels ? `${state.selectedPixels} ${t('selected')}` : ''; renderPalette(); renderLayers(); renderTimeline(); selectionOverlay(); document.title = `${state.dirty ? '• ' : ''}${filename} — Aego Asset Studio` }
function fitZoom() { const c = state.document.canvas; const v = $('viewport'); setZoom(Math.max(1, Math.floor(Math.min((v.clientWidth - 70) / c.width, (v.clientHeight - 70) / c.height)))) }
function setZoom(value, anchor = null) {
  const viewport=$('viewport'), before=$('canvas').getBoundingClientRect(), oldZoom=zoom;
  const position=anchor?{x:(anchor.x-before.left)/oldZoom,y:(anchor.y-before.top)/oldZoom}:null;
  zoom=Math.max(1,Math.min(64,value));
  if(state){
    const c=state.document.canvas, stage=$('stage');stage.style.width=c.width*zoom+'px';stage.style.height=c.height*zoom+'px';
    $('canvas').style.width=stage.style.width;$('canvas').style.height=stage.style.height;
    $('pixel-grid').style.backgroundSize=`${zoom}px ${zoom}px`;$('pixel-grid').style.display=grid&&zoom>=5?'block':'none';
    $('zoom').textContent=zoom*100+'%';selectionOverlay();
    if(position){const after=$('canvas').getBoundingClientRect();viewport.scrollLeft+=after.left+position.x*zoom-anchor.x;viewport.scrollTop+=after.top+position.y*zoom-anchor.y;}
  }
  safeSet('aego.art.zoom',String(zoom));
}
function selectionOverlay() { if (!state) return; const pivot=activePivot(playFrame||state.frame), marker=$('pivot-overlay');marker.classList.toggle('hidden',!pivot);if(pivot){marker.style.left=pivot.pivot.X*zoom+'px';marker.style.top=pivot.pivot.Y*zoom+'px';} const box = $('selection-box'); const r = state.selection; const visible = state.selectedPixels > 0; box.classList.toggle('hidden', !visible); if (visible) { box.style.left = r.Min.X * zoom + 'px'; box.style.top = r.Min.Y * zoom + 'px'; box.style.width = (r.Max.X - r.Min.X) * zoom + 'px'; box.style.height = (r.Max.Y - r.Min.Y) * zoom + 'px' } }
function paint(frame = playFrame || state?.frame) {
if (!api || !state) return; const c = state.document.canvas; const bytes = api.pixels(frame, onion && !playing, true); if (!bytes) { say('art: preview failed', true); return } const canvas = $('canvas'); if (canvas.width !== c.width || canvas.height !== c.height) { canvas.width = c.width; canvas.height = c.height; setZoom(zoom) } canvas.getContext('2d').putImageData(new ImageData(bytes, c.width, c.height), 0, 0);
  const pv = $('preview'); pv.width = c.width; pv.height = c.height; const clean = api.pixels(frame, false, false); if (clean) pv.getContext('2d').putImageData(new ImageData(clean, c.width, c.height), 0, 0); pv.style.width = c.width * 4 + 'px'; pv.style.height = c.height * 4 + 'px'; updateTiledPreview(); selectionOverlay(); window.AegoGPUPreview?.update({api, documentKey:activeDocument, revision:state.document.revision, frame, gesture:state.gesture, effectTemplate:state.document.properties?.['aego.effectTemplate'] || ''})
}
function chooseFrame(id) { stopPlayback(); if (call({ action: 'select', frame: id })) { renderUI(); paint() } }
function stopPlayback() { playing = false; playFrame = ''; $('play').textContent = '▶'; $('play').classList.remove('active') }
function togglePlay() { if (!api) return; cancelPointer(); if (playing) { stopPlayback(); renderTimeline(); paint(); return } playing = true; playStart = performance.now(); $('play').textContent = 'Ⅱ'; $('play').classList.add('active'); requestAnimationFrame(tick) }
function tick(now) { if (!playing) return; if (now - lastDraw >= 12) { const res = call({ action: 'frame-at', tag: $('tags').value, timeMS: Math.floor(now - playStart) }, { quiet: true, update: false }); if (res && res.id !== playFrame) { playFrame = res.id; paint(playFrame); $('timeline-table').querySelectorAll('th button[data-frame]').forEach(b => b.classList.toggle('active', b.dataset.frame === playFrame)) } lastDraw = now } requestAnimationFrame(tick) }
function point(e) { const r = $('canvas').getBoundingClientRect(); return { X: Math.floor((e.clientX - r.left) / zoom), Y: Math.floor((e.clientY - r.top) / zoom) } }
function showCursor(p) { lastPoint = p; $('coordinates').textContent = `${p.X}, ${p.Y}`; const cur = $('cursor'); const size = Number($('brush-size').value) || 1; cur.classList.remove('hidden'); cur.style.width = size * zoom + 'px'; cur.style.height = size * zoom + 'px'; cur.style.left = (p.X - Math.floor(size / 2)) * zoom + 'px'; cur.style.top = (p.Y - Math.floor(size / 2)) * zoom + 'px' }
function cancelPointer() { if (!api) return; if (state?.gesture) call({ action: 'cancel' }, { quiet: true }); if (pointer !== null && $('canvas').hasPointerCapture(pointer)) $('canvas').releasePointerCapture(pointer); pointer = null; pan = null; selectionPath = []; construction = []; if (state) paint() }
function previewConstruction(hover) {
  if (!state?.gesture || !construction.length) return;
  const points = construction.slice();
  if (hover) points.push(hover);
  const count = tool === 'curve' ? 4 : 3;
  while (points.length < count) points.push(points[points.length - 1]);
  if (call({action:'shape-preview',points:tool === 'curve' ? points.slice(0,4) : points},{quiet:true})) paint();
}
function finishConstruction() {
  if ((tool === 'polygon' && construction.length < 3) || (tool === 'curve' && construction.length !== 4)) return;
  if (!call({action:'shape-preview',points:construction},{quiet:true})) return;
  if (call({action:'end'})) { construction = []; renderUI(); paint(); scheduleRecovery(); }
}
const canvas = $('canvas');
canvas.addEventListener('pointerdown', e => {
  if (!api) return; e.preventDefault(); canvas.focus({preventScroll:true}); const p = point(e); showCursor(p);
  if (e.pointerType === 'touch') { touches.set(e.pointerId, { x: e.clientX, y: e.clientY }); if (touches.size === 2) { cancelPointer(); const [a, b] = [...touches.values()]; pinch = { distance: Math.hypot(a.x - b.x, a.y - b.y), zoom }; return } }
  if (pointer !== null || pinch) return; pointer = e.pointerId; pointerStart = p; canvas.setPointerCapture(pointer); stopPlayback();
  if (e.button === 1 || e.button === 2 && e.altKey) { pan = { x: e.clientX, y: e.clientY, left: $('viewport').scrollLeft, top: $('viewport').scrollTop }; return }
  if (tool === 'eyedropper') { const res = call({ action: 'sample', points: [p] }); if (res?.color) { fg = res.color; if(res.paint?.useIndex) fgIndex=res.paint.index; colorsUI() } pointer = null; return }
  if (tool === 'wand') { if (call({ action: 'selection', tool: 'wand', points: [p], tolerance: 0 })) { renderUI(); paint() } pointer = null; return }
  if (tool === 'fill') { commit({ kind: 'fill', points: [p], brush: brush() }); pointer = null; return }
  if (tool === 'polygon' || tool === 'curve') {
    if (!construction.length && !call({action:'begin',tool,points:[p],brush:brush(),filled:$('filled').checked},{quiet:true})) { pointer = null; return; }
    construction.push(p); previewConstruction();
    say(tool === 'polygon' ? t('polyHint') : `${t('curveHint')} · ${construction.length}/4`);
    if (tool === 'curve' && construction.length === 4) finishConstruction();
    pointer = null; return;
  }
  if (tool.startsWith('select-') || tool === 'lasso') { selectionPath = [p]; return }
  if (tool === 'move') { return }
  const b = brush(); if (e.button === 2) { [b.paint, b.background] = [b.background, b.paint] }
  if (call({ action: 'begin', tool, points: [p], brush: b, filled: $('filled').checked }, { quiet: true })) paint(); else cancelPointer()
});
canvas.addEventListener('pointermove', e => {
  if (!api) return; if (e.pointerType === 'touch' && touches.has(e.pointerId)) { touches.set(e.pointerId, { x: e.clientX, y: e.clientY }); if (pinch && touches.size >= 2) { const [a, b] = [...touches.values()]; setZoom(Math.round(pinch.zoom * Math.hypot(a.x - b.x, a.y - b.y) / Math.max(1, pinch.distance)),{x:(a.x+b.x)/2,y:(a.y+b.y)/2}); return } }
  const p = point(e); showCursor(p);
  if (construction.length && (tool === 'curve' || tool === 'polygon')) { previewConstruction(p); return; }
  if (pointer !== e.pointerId) return;
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
$('viewport').addEventListener('wheel', e => { if (!api || e.shiftKey) return; e.preventDefault(); setZoom(zoom + (e.deltaY < 0 ? 1 : -1),{x:e.clientX,y:e.clientY}) }, { passive: false });
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
    // File System Access is a top-level enhancement. A cross-origin lesson
    // must retain its ordinary download path even when the API is exposed.
    if (window.top === window && window.isSecureContext && typeof window.showSaveFilePicker === 'function') {
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
  // Reading a local file yields to input. Do not switch the active session
  // underneath a gesture started after the file picker returned.
  const current = JSON.parse(api.request('{"action":"state"}'));
  if (current.error) throw new Error(current.error);
  if (current.state?.gesture || pointer !== null) throw new Error(t('open-busy'));
  stopPlayback(); captureView();
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
function renderTemplateOptions() {
  const select = $('new-dialog').querySelector('[name=template]'), selected = select.value;
  select.replaceChildren(new Option(t('blank-document'), ''));
  for (const item of effectTemplates) select.add(new Option(lang === 'ru' ? item.nameRU : item.nameEN, item.id));
  select.value = effectTemplates.some(item => item.id === selected) ? selected : '';
}
function setNewTemplate(id = '') {
  const form = $('new-dialog').querySelector('form');
  if (id && !effectTemplates.some(item => item.id === id)) throw new Error('Unknown effect template');
  form.elements.template.value = id;
  for (const field of ['width','height','mode']) form.elements[field].disabled = !!id;
  $('template-help').hidden = !id;
}
$('new').onclick = () => {
  if (!api) return;
  cancelPointer(); stopPlayback();
  const result = call({action:'templates'}, {quiet:true, update:false});
  if (!result) return;
  effectTemplates = result.templates || []; renderTemplateOptions(); setNewTemplate('');
  $('new-dialog').showModal();
};
$('new-dialog').querySelector('[name=template]').onchange = event => setNewTemplate(event.target.value);
$('new-dialog').addEventListener('close', () => {
  if ($('new-dialog').returnValue !== 'create') return;
  const form = $('new-dialog').querySelector('form'), template = form.elements.template.value;
  const item = effectTemplates.find(value => value.id === template);
  const name = item ? (lang === 'ru' ? item.nameRU : item.nameEN) + '.aeart' : '';
  const res = call({action:'new', template, name, width:Number(form.elements.width.value), height:Number(form.elements.height.value), mode:form.elements.mode.value, confirm:true});
  if (res) { renderUI(); restoreView(); paint(); scheduleRecovery(); if (template) workspace('vfx'); }
});
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
function activePivot(frame=state.frame){let global=null,specific=null;for(const slice of state.document.slices||[]){if(slice.name!=='pivot')continue;if(!slice.frame)global=slice;else if(slice.frame===frame)specific=slice;}return specific||global;}
$('pivot').onclick=()=>{
  editingPivot=(state.document.slices||[]).find(s=>s.name==='pivot'&&s.frame===state.frame)||null;
  const existing=activePivot(), f=$('pivot-dialog').querySelector('form'), c=state.document.canvas;
  f.elements.x.value=existing?.pivot.X??Math.floor(c.width/2);f.elements.y.value=existing?.pivot.Y??Math.floor(c.height/2);
  openVersionedDialog('pivot-dialog');
};
$('pivot-dialog').addEventListener('close',()=>{
  if($('pivot-dialog').returnValue!=='apply'||!dialogCurrent('pivot-dialog'))return;
  const f=$('pivot-dialog').querySelector('form'), c=state.document.canvas;
  commit({kind:'slice',slice:{id:editingPivot?.id||'',name:'pivot',frame:state.frame,rect:{Min:{X:0,Y:0},Max:{X:c.width,Y:c.height}},pivot:{X:Number(f.elements.x.value),Y:Number(f.elements.y.value)}}});
});
$('merge-down').onclick = () => { cancelPointer(); commit({kind:'merge-down'}); };
$('isolate-layer').onclick = () => { cancelPointer(); if(call({action:'isolate',layer:state.isolation===state.layer?'':state.layer})){renderUI();paint();} };
$('transform').onclick = () => {
  const f=$('transform-dialog').querySelector('form'), r=state.selection, c=state.document.canvas;
  f.elements.width.value=state.selectedPixels?r.Max.X-r.Min.X:c.width;
  f.elements.height.value=state.selectedPixels?r.Max.Y-r.Min.Y:c.height;
  openVersionedDialog('transform-dialog');
};
$('transform-dialog').addEventListener('close',()=>{
  if($('transform-dialog').returnValue!=='apply'||!dialogCurrent('transform-dialog'))return;
  const f=$('transform-dialog').querySelector('form');commit({kind:f.elements.kind.value,width:Number(f.elements.width.value),height:Number(f.elements.height.value)});
});
$('ramp-shade').onclick=()=>{
  const f=$('ramp-dialog').querySelector('form');f.elements.first.max=f.elements.last.max=state.document.palette.length-1;
  f.elements.first.value=Math.max(1,Math.min(fgIndex,bgIndex));f.elements.last.value=Math.max(1,Math.max(fgIndex,bgIndex));
  openVersionedDialog('ramp-dialog');
};
$('ramp-dialog').addEventListener('close',()=>{
  if($('ramp-dialog').returnValue!=='apply'||!dialogCurrent('ramp-dialog'))return;
  const f=$('ramp-dialog').querySelector('form');commit({kind:'ramp-shade',rampFirst:Number(f.elements.first.value),rampLast:Number(f.elements.last.value),offset:Number(f.elements.step.value)});
});
function updateTiledPreview(){
  const space=document.querySelector('.preview-space'), preview=$('preview');
  // Repeat already-composited Go pixels for presentation; authoring and export
  // still use the single canonical document and its Go render operations.
  space.style.backgroundImage=tiledPreview&&$('gpu-mode').value==='pixels'?`url(${preview.toDataURL()})`:'';
  space.style.backgroundSize=tiledPreview?`${preview.width*4}px ${preview.height*4}px`:'';
}
$('canvas-background').onchange=e=>{$('stage').dataset.background=e.target.value;safeSet('aego.art.background',e.target.value);};
$('canvas-background').value=safeGet('aego.art.background')||'checker';$('stage').dataset.background=$('canvas-background').value;
$('tile-preview').onclick=()=>{tiledPreview=!tiledPreview;$('tile-preview').classList.toggle('active',tiledPreview);updateTiledPreview();};
$('gpu-mode').addEventListener('change',updateTiledPreview);
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
function workspace(name) { document.body.classList.toggle('workspace-vfx', name === 'vfx'); if (name === 'vfx') window.AegoGPUPreview?.select('particles'); document.body.classList.toggle('workspace-animation', name === 'animation'); document.querySelectorAll('[data-workspace]').forEach(b => b.classList.toggle('active', b.dataset.workspace === name)); safeSet('aego.art.workspace', name) }
document.querySelectorAll('[data-workspace]').forEach(b => b.onclick = () => workspace(b.dataset.workspace)); workspace(safeGet('aego.art.workspace') || 'art');
document.addEventListener('keydown', e => {
  if (!api || e.target.matches('input,textarea,select') || document.querySelector('dialog[open]')) return; const k = e.key.toLowerCase(), mod = e.ctrlKey || e.metaKey;
  if (e.key === 'Escape') { e.preventDefault(); construction = []; cancelPointer(); call({ action: 'selection', tool: 'none' }); renderUI(); paint(); return }
  if (mod) { if (k === 'z') { e.preventDefault(); e.shiftKey ? redo() : undo() } else if (k === 'y') { e.preventDefault(); redo() } else if (k === 's') { e.preventDefault(); save() } else if (k === 'o') { e.preventDefault(); $('file-input').click() } else if (k === 'c') { e.preventDefault(); if (call({ action: 'copy' })) say(t('copy')) } else if (k === 'v') { e.preventDefault(); if (call({ action: 'paste', points: [lastPoint] })) { renderUI(); paint(); scheduleRecovery() } } else if (k === 'a') { e.preventDefault(); call({ action: 'selection', tool: 'all' }); renderUI(); paint() } else if (k === 'd') { e.preventDefault(); call({ action: 'selection', tool: 'none' }); renderUI(); paint() } return }
  if (e.key === 'Enter') { e.preventDefault(); if (tool === 'polygon' && construction.length) finishConstruction(); else togglePlay(); return }
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
    if (!tab.dirty) { recoveryVersions.delete(tab.key); queueRecovery(() => recoveryStore.remove(key)); continue; }
    // Do not serialize every unchanged dirty tab after a stroke in another
    // document. Coalesce pending writes; a failed storage job clears its
    // version for retry and never marks the document as saved.
    const previous = recoveryVersions.get(tab.key);
    if (previous?.revision === tab.revision && previous.name === tab.name) continue;
    if (status.active === tab.key && status.state?.gesture) continue;
    const res = api.exportBytes('recovery', 8, tab.key);
    if (res.error) { say(t('recoveryError') + ': ' + res.error, true); window.dispatchEvent(new Event('aego-art-recovery-error')); continue; }
    const snapshot = { key, bytes: new Uint8Array(res.bytes), filename: tab.name, revision: res.revision, dirty: true, at: Date.now() };
    const version = { revision: res.revision, name: tab.name };
    recoveryVersions.set(tab.key, version);
    queueRecovery(async () => {
      try { await recoveryStore.put(snapshot); }
      catch (error) { if (recoveryVersions.get(tab.key) === version) recoveryVersions.delete(tab.key); throw error; }
    });
  }
}
async function showRecovery() {
  if (!recoveryStore) return;
  const records = await recoveryStore.list();
  const opened = new Set(recoveryKeys.values());
  const selected = $('recovery-choice').value;
  recoveryData = records.filter(v => !opened.has(v.key) && v.dirty && v.bytes);
  $('recovery-choice').replaceChildren(...recoveryData.map(v => new Option(`${v.filename} · ${new Date(v.at).toLocaleString()}${restoredRecoverySources.has(v.key) ? ' · ' + t('recovery-restored') : ''}`, v.key)));
  if (recoveryData.some(v => v.key === selected)) $('recovery-choice').value = selected;
  $('recovery').classList.toggle('hidden', !recoveryData.length);
  updateRecoveryActions();
}
function updateRecoveryActions() {
  const key = $('recovery-choice').value;
  $('restore').disabled = recoveryBusy || !key || restoredRecoverySources.has(key);
  $('dismiss-recovery').disabled = recoveryBusy || !key;
}
$('recovery-choice').addEventListener('change', updateRecoveryActions);
$('restore').onclick = async () => {
  if (recoveryBusy) return;
  const record = recoveryData.find(v => v.key === $('recovery-choice').value);
  if (!record) return;
  if (restoredRecoverySources.has(record.key)) { say(t('recovery-already')); return; }
  recoveryBusy = true; updateRecoveryActions();
  try {
    cancelPointer(); stopPlayback();
    const res = JSON.parse(api.importBytes(record.bytes, 'aeart', false, record.filename || 'Recovered.aeart'));
    if (res.error) throw new Error(res.error);
    adopt(res);
    // The original may still belong to a live editor in another window. Each
    // restored session owns a fresh key; closing it cannot delete that owner.
    recoveryKeys.set(activeDocument, `${browserSession}/${activeDocument}`);
    restoredRecoverySources.add(record.key);
    call({ action: 'recovered' });
    renderUI(); restoreView(); paint(); scheduleRecovery();
    say(t('recovery-copy'));
    await showRecovery();
  } catch (error) { say(error.message, true); }
  finally { recoveryBusy = false; updateRecoveryActions(); }
};
$('dismiss-recovery').onclick = async () => {
  if (recoveryBusy) return;
  const key = $('recovery-choice').value;
  if (!key || !confirm(lang === 'ru' ? 'Удалить эту резервную копию? Другое окно может использовать её.' : 'Discard this recovery copy? Another window may be using it.')) return;
  recoveryBusy = true; updateRecoveryActions();
  try { await queueRecovery(() => recoveryStore.remove(key)); await showRecovery(); }
  catch (error) { say(error.message, true); }
  finally { recoveryBusy = false; updateRecoveryActions(); }
};
window.addEventListener('pagehide', flushRecovery);
(async () => { translate(); try { const go = new Go(); const response = await fetch(window.AegoStudioRelease?.wasmURL || 'artstudio.wasm'); if (!response.ok) throw new Error(`WASM HTTP ${response.status}`); let instance; try { ({ instance } = await WebAssembly.instantiateStreaming(response.clone(), go.importObject)) } catch { ({ instance } = await WebAssembly.instantiate(await response.arrayBuffer(), go.importObject)) } go.run(instance).catch(error => say(error.message, true)); api = window.aegoArt; if (!api) throw new Error('Go bridge did not initialize'); adopt(JSON.parse(api.request('{"action":"state"}'))); $('loading').classList.add('hidden'); renderUI(); fitZoom(); paint(); say(t('ready')); try { recoveryStore = await AegoRecoveryStore.open(); await showRecovery() } catch (error) { say(t('storageUnavailable'), true) } window.artStudioReady = true } catch (error) { $('loading').textContent = error.message; say(error.message, true) } })();
