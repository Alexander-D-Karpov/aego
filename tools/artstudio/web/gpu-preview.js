/* Presentation transport only. Aego's Go runtime owns the preview world,
 * prepared textures, lighting, materials, CPU particles and GPU resources. */
'use strict';
(() => {
  const byID = id => document.getElementById(id);
  let latest, loaded, loading, queued = false, published = '', configured = '', sequence = 0;
  let stopped = false, lastStatus = {}, transportError = '';
  let rateOverride = null;
  const mode = byID('gpu-mode'), canvas = byID('gpu-preview'), status = byID('gpu-status');
  const ru = () => document.documentElement.lang === 'ru';
  const message = (text, error = false) => { status.textContent = text; status.classList.toggle('error', error); };
  const fail = error => { transportError = error.message || String(error); message(transportError, true); canvas.classList.add('gpu-failed'); };
  function options() {
    return {mode: mode.value, frame: latest?.frame || '', effectTemplate:latest?.effectTemplate || '',
      rateOverride:latest?.effectTemplate ? rateOverride : null, flipX: byID('gpu-flip-x').checked,
      flipY: byID('gpu-flip-y').checked, bloom: byID('gpu-bloom').checked,
      shadows: byID('gpu-shadows').checked, lightX: Number(byID('gpu-light').value), lightY: -.65,
      intensity: 2.2, emission: Number(byID('gpu-emission').value), amount: Number(byID('gpu-amount').value),
      rate: Number(byID('gpu-rate').value), speed: 30, lifetime: 1.4};
  }
  function accept(raw) {
    const result = JSON.parse(raw);
    if (result.error) throw new Error(result.error);
    if (result.status) lastStatus = result.status;
    return result;
  }
  async function load() {
    if (loaded) return loaded;
    if (loading) return loading;
    loading = (async () => {
      message(ru() ? 'Загрузка GPU renderer…' : 'Loading GPU renderer…');
      const response = await fetch(window.AegoStudioRelease?.previewWasmURL || 'preview.wasm');
      if (!response.ok) throw new Error(`GPU WASM HTTP ${response.status}`);
      const go = new Go();
      let instance;
      try { ({instance} = await WebAssembly.instantiateStreaming(response.clone(), go.importObject)); }
      catch { ({instance} = await WebAssembly.instantiate(await response.arrayBuffer(), go.importObject)); }
      go.run(instance).catch(error => { stopped = true; fail(error); });
      const api = window.aegoArtPreview;
      if (!api) throw new Error('GPU WASM bridge did not initialize');
      accept(api.start());
      loaded = api;
      return api;
    })();
    try { return await loading; } catch (error) { loading = null; throw error; }
  }
  async function synchronize() {
    queued = false;
    if (!latest || mode.value === 'pixels' || stopped) return;
    try {
      const preview = await load();
      if (!latest || mode.value === 'pixels' || stopped) return;
      const key = `${latest.documentKey}:${latest.revision}`;
      if (published !== key && !latest.gesture) {
        const snapshot = latest.api.snapshotBytes();
        if (snapshot.error) throw new Error(snapshot.error);
        if (snapshot.handle !== latest.documentKey || snapshot.revision !== latest.revision) { schedule(); return; }
        accept(preview.publish(++sequence, snapshot.revision, snapshot.bytes));
        published = key; configured = '';
      }
      const config = JSON.stringify(options());
      if (config !== configured) { accept(preview.configure(config)); configured = config; }
      // A ready renderer may still contain the previous document. A failed
      // publish/configure remains visible until this exact state is accepted.
      if (published === key && configured === config) transportError = '';
      poll();
    } catch (error) { fail(error); }
  }
  function schedule() { if (!queued) { queued = true; requestAnimationFrame(synchronize); } }
  function poll() {
    if (!loaded || mode.value === 'pixels' || stopped) return;
    try {
      accept(loaded.status());
      if (transportError) { message(transportError, true); canvas.classList.add('gpu-failed'); return; }
      if (latest && published !== `${latest.documentKey}:${latest.revision}`) {
        message(ru() ? 'Обновление предпросмотра…' : 'Updating preview…');
        return;
      }
      if (latest?.effectTemplate && rateOverride === null && Number.isFinite(lastStatus.particleRate)) byID('gpu-rate').value = lastStatus.particleRate;
      const failed = lastStatus.state === 'context-lost' || lastStatus.state === 'error';
      canvas.classList.toggle('gpu-failed', failed);
      if (lastStatus.state === 'context-lost') message(ru() ? 'Контекст WebGL потерян; ожидается восстановление.' : 'WebGL context lost; waiting for restoration.', true);
      else if (lastStatus.error) message(lastStatus.error, true);
      else {status.title = lastStatus.renderer || ''; message(`WebGL2 · ${ru() ? 'Готово' : 'Ready'}${mode.value === 'particles' ? ` · ${lastStatus.particlePreset || 'ParticleSystem2D'} · ${lastStatus.particles || 0} ${ru() ? 'частиц' : 'particles'}` : ''}`);}
    } catch (error) { fail(error); }
  }
  function select() {
    const enabled = mode.value !== 'pixels';
    byID('preview').classList.toggle('hidden', enabled);
    canvas.classList.toggle('hidden', !enabled);
    byID('gpu-controls').classList.toggle('hidden', !enabled);
    status.classList.toggle('hidden', !enabled);
    byID('gpu-restart').hidden = mode.value !== 'particles';
    document.body.classList.toggle('has-gpu-preview', enabled);
    if (enabled) {configured = ''; schedule();}
    else if (loaded) {const config = {...options(), mode:lastStatus.mode || 'sprite', paused:true}; try {accept(loaded.configure(JSON.stringify(config)));} catch (error) {message(error.message,true);} }
  }
  mode.addEventListener('change', select);
  byID('gpu-rate').addEventListener('input', () => { rateOverride = Number(byID('gpu-rate').value); });
  byID('gpu-controls').addEventListener('input', schedule);
  byID('gpu-restart').addEventListener('click', async () => {
    if (mode.value !== 'particles' || stopped) return;
    try { await synchronize(); if (loaded) { accept(loaded.restartParticles()); poll(); } }
    catch (error) { fail(error); }
  });
  canvas.addEventListener('webglcontextlost', () => { canvas.classList.add('gpu-failed'); message(ru() ? 'Контекст WebGL потерян.' : 'WebGL context lost.', true); });
  canvas.addEventListener('webglcontextrestored', () => { configured = ''; schedule(); });
  window.addEventListener('pagehide', event => { if (!event.persisted) { stopped = true; loaded?.close(); } });
  setInterval(poll, 400);
  window.AegoGPUPreview = Object.freeze({
    update(value) {
      if (latest?.documentKey !== value.documentKey || latest?.effectTemplate !== value.effectTemplate) {
        rateOverride = null; byID('gpu-rate').value = 16;
      }
      latest = value; if (mode.value !== 'pixels') schedule();
    },
    select(value) { if (![...mode.options].some(o => o.value === value)) return false; mode.value = value; select(); return true; },
    status() { return {...lastStatus}; }
  });
})();
