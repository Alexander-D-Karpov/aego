/* Standalone/embedded host, not an alternative editor. All snapshots are Go
 * encoded .aeart files; no pixels are sent to an HTTP endpoint. */
'use strict';
(async function () {
  const cfg=window.AegoStudioRelease,P=window.AegoStudioProtocol,H=window.AegoStudioHost;
  const url=new URL(location.href),base=new URL(cfg.baseURL),relative=url.pathname.startsWith(base.pathname)?url.pathname.slice(base.pathname.length):url.pathname.replace(/^\//,'');
  const embedded=relative.startsWith('embed/'),transferID=url.searchParams.get('transfer');
  const locale=(url.searchParams.get('lang')||document.documentElement.lang)==='en'?'en':'ru';
  const allLabels={ru:{full:'Открыть полный редактор',reset:'Сбросить пример',home:'Главная',fallback:'Скачать для переноса',sending:'Передача документа…',received:'Документ получен. Сохраните .aeart для постоянной копии.',sent:'Документ передан. Исходная вкладка и её Undo сохранены.',failed:'Передача не выполнена. Скачайте .aeart и откройте его в полной версии.',blocked:'Браузер заблокировал новую вкладку.',storage:'Локальное восстановление недоступно. Скачивайте .aeart перед закрытием.',limit:'GPU Preview использует отдельную сцену Aego; объекты открытого игрового проекта в браузере недоступны.',open:'Открыть выбранный файл',timeout:'Редактор не готов.'},en:{full:'Open Full Editor',reset:'Reset Example',home:'Home',fallback:'Download for transfer',sending:'Transferring document…',received:'Document received. Save .aeart for a permanent copy.',sent:'Document transferred. Source tab and its Undo are preserved.',failed:'Transfer failed. Download .aeart and open it in the full editor.',blocked:'The browser blocked the new tab.',storage:'Local recovery is unavailable. Download .aeart before closing.',limit:'GPU Preview uses an isolated Aego world; the browser has no current game-project object.',open:'Open a local file',timeout:'Editor is not ready.'}};
  Object.assign(allLabels.ru,{incompatible:'Несовместимая версия редактора.',rejected:'Получатель отклонил снимок.',transferTimeout:'Время передачи истекло.',hashFailed:'Не удалось вычислить хеш.',noSource:'Не найдено подходящее исходное окно.',sourceTimeout:'Время ожидания исходного окна истекло.',snapshotRejected:'Снимок отклонён.',embedRejected:'Источник или сеанс встраивания отклонён. Откройте полный редактор.',busy:'Операция уже выполняется.',noExample:'Пример не выбран.'});
  Object.assign(allLabels.en,{incompatible:'Incompatible target version.',rejected:'Receiver rejected the snapshot.',transferTimeout:'Transfer timed out.',hashFailed:'Hash calculation failed.',noSource:'No valid source window.',sourceTimeout:'Source timeout.',snapshotRejected:'Snapshot rejected.',embedRejected:'Embedding origin/session rejected. Open the full editor directly.',busy:'An operation is already in progress.',noExample:'No example is selected.'});
  let labels=allLabels[locale];
  const bar=document.createElement('div');bar.className='release-toolbar';
  function button(text,fn,id){const b=document.createElement('button');b.type='button';b.textContent=text;if(id)b.id=id;b.onclick=fn;bar.append(b);return b}
  const home=document.createElement('a');home.href=base.href;home.textContent=labels.home;bar.append(home);
  let activeExample=null,pendingTransfer=null,fallbackSnapshot=null;
  const full=button(labels.full,()=>startTransfer(),'studio-open-full');
  const downloadButton=button(locale==='ru'?'Скачать .aeart':'Download .aeart',()=>{try{H.downloadSource()}catch(e){H.notice(e.message,true)}},'studio-download-source');
  full.hidden=!embedded;
  const reset=button(labels.reset,()=>{try{if(activeExample)H.resetExample(activeExample)}catch(e){H.notice(e.message,true)}},'studio-reset-example');reset.hidden=true;
  const fallback=button(labels.fallback,()=>{try{H.downloadSource(fallbackSnapshot)}catch(e){H.notice(e.message,true)}},'studio-transfer-download');fallback.hidden=true;
  const progress=document.createElement('span');progress.className='transfer-status';progress.setAttribute('role','status');bar.append(progress);
  const badge=document.createElement('span');badge.className='release-version';badge.textContent=cfg.buildID+(cfg.snapshot?' · SNAPSHOT':'');bar.append(badge);
  document.querySelector('.menubar').after(bar);document.body.classList.toggle('studio-embedded',embedded);
  document.querySelector('.build-badge').textContent=cfg.buildID+(cfg.snapshot?' · snapshot':'');
  function showStorageWarning(){
    let warning=bar.querySelector('.storage-warning');
    if(!warning){warning=document.createElement('div');warning.className='storage-warning';bar.append(warning)}
    warning.textContent=labels.storage;
  }
  window.addEventListener('aego-art-recovery-error',showStorageWarning);
  new MutationObserver(()=>{
    const code=document.documentElement.lang==='en'?'en':'ru';labels=allLabels[code];
    home.textContent=labels.home;full.textContent=labels.full;reset.textContent=labels.reset;fallback.textContent=labels.fallback;
    downloadButton.textContent=code==='ru'?'Скачать .aeart':'Download .aeart';
    if(bar.querySelector('.storage-warning'))showStorageWarning();
  }).observe(document.documentElement,{attributes:true,attributeFilter:['lang']});
  function failure(message,snapshot){fallbackSnapshot=snapshot;fallback.hidden=!snapshot;progress.textContent=message+' '+labels.failed;H.notice(progress.textContent,true)}
  function startTransfer(){
    if(pendingTransfer)return;
    let snap;try{snap=H.capture()}catch(e){H.notice(e.message,true);return}
    const session=P.token(),targetURL=new URL('editor/',base);targetURL.searchParams.set('transfer',session);targetURL.searchParams.set('lang',document.documentElement.lang);
    const popup=window.open(targetURL.href,'_blank');
    if(!popup){failure(labels.blocked,snap);return}
    progress.textContent=labels.sending;full.disabled=true;
    const pending={popup,session,snapshot:snap,sha:null,ready:false,sent:false};pendingTransfer=pending;
    function cleanup(){clearTimeout(pending.timer);window.removeEventListener('message',pending.listener);pendingTransfer=null;full.disabled=false}
    function transmit(){if(!pending.ready||!pending.sha||pending.sent)return;pending.sent=true;P.send(popup,location.origin,session,'transfer-document','document',{
      operation:'import',format:'aeart',documentVersion:cfg.documentVersion,buildID:cfg.buildID,revision:snap.revision,selection:snap.selection,
      bytes:snap.bytes.buffer,size:snap.bytes.length,sha256:pending.sha
    })}
    pending.listener=event=>{
      if(!P.accepted(event,popup,location.origin,session))return;
      if(event.data.type==='transfer-ready'){
        if(event.data.payload.buildID!==cfg.buildID||event.data.payload.documentVersion!==cfg.documentVersion){cleanup();failure(labels.incompatible,snap);return}
        pending.ready=true;transmit();
      }else if(event.data.type==='transfer-ack' && event.data.payload.sha256===pending.sha){cleanup();fallback.hidden=true;progress.textContent=labels.sent}
      else if(event.data.type==='error'){cleanup();failure(labels.rejected,snap)}
    };
    window.addEventListener('message',pending.listener);
    pending.timer=setTimeout(()=>{cleanup();failure(labels.transferTimeout,snap)},30000);
    P.sha256(snap.bytes).then(hash=>{if(pendingTransfer!==pending)return;pending.sha=hash;transmit()}).catch(()=>{cleanup();failure(labels.hashFailed,snap)});
  }
  try{await H.ready}catch(e){progress.textContent=labels.timeout;return}
  H.language(locale);
  if(!H.storageAvailable()){
    showStorageWarning();
  }
  if(transferID){
    // Only the same-origin opener that created this window can supply a snapshot.
    if(!/^[a-f0-9]{32}$/.test(transferID)||!window.opener){failure(labels.noSource,null);return}
    const opener=window.opener,origin=location.origin;let imported=false,inFlight=false;
    const hello=()=>P.send(opener,origin,transferID,'transfer-ready','ready',{buildID:cfg.buildID,documentVersion:cfg.documentVersion});
    const timer=setInterval(hello,250),timeout=setTimeout(()=>{clearInterval(timer);window.removeEventListener('message',receive);if(!imported)failure(labels.sourceTimeout,null)},30000);
    async function receive(event){
      if(!P.accepted(event,opener,origin,transferID)||event.data.type!=='transfer-document'||inFlight)return;
      const data=event.data.payload;
      try{
        if(imported) return;
        if(!P.snapshotShape(data,cfg.buildID,cfg.documentVersion))throw new Error('Invalid snapshot envelope');
        inFlight=true;
        if(await P.sha256(data.bytes)!==data.sha256)throw new Error('Snapshot SHA-256 mismatch');
        H.importSnapshot(new Uint8Array(data.bytes),{unsaved:true,selection:data.selection});imported=true;
        P.send(opener,origin,transferID,'transfer-ack',event.data.request,{sha256:data.sha256});
        clearInterval(timer);clearTimeout(timeout);window.removeEventListener('message',receive);progress.textContent=labels.received;
        url.searchParams.delete('transfer');history.replaceState(null,'',url.pathname+url.search);try{window.opener=null}catch{}
      }catch(e){inFlight=false;P.send(opener,origin,transferID,'error',event.data.request,{code:'snapshot-rejected'});failure(labels.snapshotRejected,null)}
    }
    window.addEventListener('message',receive);hello();
  } else if(relative.startsWith('examples/')) {
    const id=relative.split('/')[1];try{activeExample=await H.loadExample(id);reset.hidden=false}catch(e){H.notice(e.message,true)}
  } else if(url.searchParams.has('new')) {
    H.newDocument();H.setNewSize(32,32);
    if(url.searchParams.get('new')==='effect') {
      try { H.setNewTemplate(url.searchParams.get('template') || 'explosion'); }
      catch(e) { H.notice(e.message,true); }
    }
    // This query is a one-shot launch action, not persistent editor state.
    // Otherwise a reload opens a modal over IndexedDB recovery for a dirty tab.
    url.searchParams.delete('new');url.searchParams.delete('template');
    history.replaceState(null,'',url.pathname+url.search+url.hash);
  } else if(url.searchParams.has('open')) {
    // Native pickers require user activation: present a real button, don't fake a click from an async load.
    button(labels.open,()=>H.openFile(),'studio-open-local');
  }
  if(embedded){
    const parentOrigin=url.searchParams.get('parentOrigin'),session=url.searchParams.get('session');
    if(window.parent===window||!cfg.docsOrigins.includes(parentOrigin)||!/^[a-f0-9]{32}$/.test(session||'')){
      H.notice(labels.embedRejected,true);return;
    }
    let ready=false,loading=false;const answered=new Map();
    function reply(type,request,payload){P.send(window.parent,parentOrigin,session,type,request,payload)}
    window.addEventListener('message',async event=>{
      if(!P.accepted(event,window.parent,parentOrigin,session))return;
      const {type,request,payload}=event.data;
      if(type==='hello'){
        if(payload.buildID!==cfg.buildID){reply('error',request,{code:'build-mismatch'});return}
        ready=true;reply('ready',request,{...H.capabilities,buildID:cfg.buildID});return;
      }
      if(!ready)return;
      if(answered.has(request)){reply('reply',request,answered.get(request));return}
      try{
        let result={};
        if(type==='summary')result=H.summary();
        else if(type==='load-example'){
          if(loading)throw new Error(labels.busy);loading=true;
          try{activeExample=await H.loadExample(payload.id);reset.hidden=false;result=H.summary()}finally{loading=false}
        }else if(type==='reset-example'){if(!activeExample)throw new Error(labels.noExample);result=H.resetExample(activeExample)||{cancelled:true}}
        else if(type==='download'){H.downloadSource();result={initiated:true,saved:false}}
        else if(type==='open-full'){full.focus();result={requiresUserGesture:true};progress.textContent=labels.full}
        else return;
        if(answered.size>=64)answered.delete(answered.keys().next().value);answered.set(request,result);reply('reply',request,result);
      }catch(e){reply('error',request,{code:'action-rejected'})}
    });
    let lastHeight=0;new ResizeObserver(()=>{
      const height=Math.max(480,Math.min(1000,document.documentElement.clientHeight));if(!ready||height===lastHeight)return;lastHeight=height;reply('resize','viewport',{height});
    }).observe(document.body);
  }
  window.artStudioSurfaceReady=true;
})();
