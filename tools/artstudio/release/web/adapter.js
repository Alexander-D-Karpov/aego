/* Thin, versioned adapter to the existing classic-script presentation. The
 * document remains in aegoArt/Go. Native documents/History are not touched. */
'use strict';
(function () {
  const config = window.AegoStudioRelease;
  const protocol = window.AegoStudioProtocol;
  const maxBytes = protocol.MAX_BYTES;
  let readyResolve, readyReject, readyMS=null, recoveryHealthy=true;
  window.addEventListener('aego-art-recovery-error',()=>{recoveryHealthy=false});
  const ready = new Promise((resolve, reject) => { readyResolve = resolve; readyReject = reject; });
  const start = performance.now();
  const timer = setInterval(() => {
    if (window.artStudioReady && window.aegoArt) {
      clearInterval(timer); readyMS=Math.round(performance.now()-start); readyResolve();
    } else if (performance.now() - start > 45000) {
      clearInterval(timer); readyReject(new Error('Asset Studio initialization timed out'));
    }
  }, 30);
  function status() {
    if (!window.aegoArt || !window.artStudioReady) throw new Error('Editor is not ready');
    const r = JSON.parse(window.aegoArt.request('{"action":"state"}'));
    if (r.error) throw new Error(r.error); return r;
  }
  function redraw(r) { adopt(r); renderUI(); restoreView(); paint(); }
  function summary() {
    const r = status(), d = r.state.document;
    const counts = new Map();
    for (const c of d.cels || []) counts.set(c.image, (counts.get(c.image) || 0) + 1);
    return {handle:r.active, revision:d.revision, dirty:r.state.dirty, gesture:r.state.gesture, width:d.canvas.width, height:d.canvas.height,
      mode:d.canvas.mode, frames:d.frames.length, layers:d.layers.length, cels:d.cels.length,
      linkedImages:[...counts.values()].filter(n=>n>1).length, channels:[...new Set(d.cels.map(c=>c.channel))], onion:!!onion,
      // This hash remains in the local browser and is never included in telemetry.
      pixelHash:window.aegoArt.hash(), buildID:config.buildID, documentVersion:config.documentVersion};
  }
  function capture(handle = '') {
    const r = status(); if (r.state.gesture) throw new Error('Finish or cancel the current gesture first');
    const exp = window.aegoArt.exportBytes('recovery',8,handle);
    if (exp.error) throw new Error(exp.error);
    if (!(exp.bytes instanceof Uint8Array) || exp.bytes.length < 1 || exp.bytes.length > maxBytes) throw new Error('Document exceeds transfer limit');
    // Recovery export does not mark saved and does not retain a pending save receipt.
    return {bytes:new Uint8Array(exp.bytes),revision:exp.revision,handle:exp.handle,
      selection:{frame:r.state.frame,layer:r.state.layer,channel:r.state.channel}};
  }
  function importSnapshot(bytes, {unsaved = true, name = 'Transferred.aeart', selection = null} = {}) {
    if (!(bytes instanceof Uint8Array) || bytes.length < 1 || bytes.length > maxBytes) throw new Error('Invalid snapshot size');
    cancelPointer(); stopPlayback(); captureView();
    const r = JSON.parse(window.aegoArt.importBytes(bytes,'aeart',false,name));
    if (r.error) throw new Error(r.error); redraw(r);
    if (unsaved) call({action:'recovered'});
    if (selection && state.document.frames.some(f=>f.id===selection.frame) && state.document.layers.some(l=>l.id===selection.layer) && ['albedo','normal','height','emission'].includes(selection.channel)) {
      call({action:'select',frame:selection.frame,layer:selection.layer,channel:selection.channel});
    }
    renderUI(); paint(); scheduleRecovery();
    return summary();
  }
  async function loadExample(id) {
    const item = config.examples.find(e=>e.id===id); if (!item) throw new Error('Unknown example');
    const u = new URL(item.file, config.baseURL); if (u.origin!==location.origin || !u.href.startsWith(config.baseURL)) throw new Error('Example URL rejected');
    const res = await fetch(u,{redirect:'error'}); if (!res.ok) throw new Error('Example HTTP '+res.status);
    const length = Number(res.headers.get('Content-Length') || 0); if (length>maxBytes) throw new Error('Example too large');
    const reader = res.body.getReader(); const chunks=[];let size=0;
    try { for (;;) { const {value,done}=await reader.read(); if(done)break;size+=value.length;if(size>item.bytes||size>maxBytes)throw new Error('Example exceeds manifest size');chunks.push(value); } } catch(e){await reader.cancel();throw e}
    const bytes=new Uint8Array(size);let pos=0;for(const b of chunks){bytes.set(b,pos);pos+=b.length}
    if(size!==item.bytes || await protocol.sha256(bytes)!==item.sha256) throw new Error('Example SHA-256 mismatch');
    const info=importSnapshot(bytes,{unsaved:false,name:item.title[lang] + '.aeart'});
    return {info,bytes,id};
  }
  function resetExample(example) {
    // A reset never guesses that the current tab is the example. The exact
    // original handle must still be open, and confirmation is required if dirty.
    const r=status(),tab=r.tabs.find(t=>t.key===example.info.handle);
    if(!tab)throw new Error('The example document was closed. Load it as a new document.');
    if(tab.dirty && !window.confirm(lang==='ru'?'Сбросить только этот пример? Его несохранённые изменения будут потеряны.':'Reset only this example and discard its unsaved changes?')) return null;
    // Import first; only close the original after successful validation/import.
    const info=importSnapshot(example.bytes,{unsaved:false,name:tab.name});
    const close=call({action:'document-close',handle:tab.key,confirm:true});
    if(!close)throw new Error('Reset copy opened, but the original document could not be closed');
    const key=recoveryKeys.get(tab.key);recoveryKeys.delete(tab.key);views.delete(tab.key);
    if(key&&recoveryStore)queueRecovery(()=>recoveryStore.remove(key));
    renderUI();paint();example.info=info;return info;
  }
  function downloadSource(snapshot = null) {
    const exp=snapshot||capture(); download(exp.bytes,'Aego-Drawing.aeart','application/octet-stream');
    // A browser download has no reliable disk-commit acknowledgement. Keep dirty.
  }
  function language(value) { if(!['ru','en'].includes(value))return;lang=value;translate(); }
  const capabilities=Object.freeze({documentVersion:config.documentVersion,protocol:protocol.VERSION,
    operations:['summary','load-example','reset-example','download','open-full'],transfer:['import'],maxDocumentBytes:maxBytes,
    sceneAssignment:false,litPreview:false,particlePreview:false,storage:'best-effort',formats:['aeart','png','ase','aseprite']});
  window.AegoStudioHost=Object.freeze({ready,capabilities,summary,capture,importSnapshot,loadExample,resetExample,downloadSource,language,
    notice:(message,error=false)=>say(message,error),newDocument:()=>document.getElementById('new').click(),openFile:()=>document.getElementById('open').click(),
    setNewSize:(w,h)=>{const f=document.getElementById('new-dialog').querySelector('form');f.elements.width.value=w;f.elements.height.value=h;},
    storageAvailable:()=>!!recoveryStore&&recoveryHealthy,initializationMS:()=>readyMS});
})();
