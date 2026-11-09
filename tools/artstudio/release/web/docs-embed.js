/* Documentation host: lazy iframe, typed commands, one active WASM session. */
'use strict';
(function(){
 const P=window.AegoStudioProtocol;let active=null;
 // Resize messages describe the editor's content viewport. A border-box
 // iframe is taller than that viewport; assigning the content height directly
 // to its CSS height feeds a two-pixel shrink back into ResizeObserver.
 function resizeFrame(frame,height){
  const wanted=Math.max(480,Math.min(1000,height)),style=getComputedStyle(frame);
  const px=name=>Number.parseFloat(style[name])||0;
  const padding=px('paddingTop')+px('paddingBottom');
  if(Math.abs(frame.clientHeight-padding-wanted)<.5)return;
  const decoration=style.boxSizing==='border-box'?padding+px('borderTopWidth')+px('borderBottomWidth'):0;
  frame.style.height=(wanted+decoration)+'px';
 }
 for(const root of document.querySelectorAll('.aego-studio-embed')){
  const lang=root.dataset.lang==='en'?'en':'ru';const text={ru:{starting:'Загрузка редактора…',ready:'Редактор готов. Рисунок остаётся в браузере.',busy:'На странице уже открыт редактор. Сначала сохраните работу в нём.',failed:'Редактор недоступен. Откройте полную версию и используйте локальный файл.',done:'Действие проверено по текущим данным документа.',pending:'Условие ещё не выполнено. Продолжите упражнение.',manual:'Это действие проверяется вручную; автоматический PASS не выставляется.'},en:{starting:'Loading the editor…',ready:'Editor ready. Artwork stays in this browser.',busy:'An editor is already active on this page. Save that work first.',failed:'Editor unavailable. Open the full app and use a local file.',done:'Action verified against the current document.',pending:'Condition not met yet. Continue the exercise.',manual:'This action requires manual verification; no automatic PASS is recorded.'}}[lang];
  const status=root.querySelector('.embed-status'),mount=root.querySelector('.embed-mount');
  let frame,session,origin,helloTimer,helloTimeout,baseline,initializing=false;let next=0;const pending=new Map();
  function say(s){status.textContent=s}
  function fail(){say(text.failed);clearInterval(helloTimer);clearTimeout(helloTimeout);for(const [,p]of pending){clearTimeout(p.timer);p.reject(new Error('Disconnected'))}pending.clear()}
  function request(type,payload={}){
   const id='request-'+(++next);return new Promise((resolve,reject)=>{
    const timer=setTimeout(()=>{pending.delete(id);reject(new Error('Request timeout'))},20000);pending.set(id,{resolve,reject,timer});P.send(frame.contentWindow,origin,session,type,id,payload);
   });
  }
  async function receive(event){
   if(!frame||!P.accepted(event,frame.contentWindow,origin,session))return;
   const m=event.data;
   if(m.type==='ready'){
    if(m.payload.buildID!==root.dataset.build||m.payload.protocol!==P.VERSION){fail();return}
    if(baseline||initializing)return;initializing=true;clearInterval(helloTimer);clearTimeout(helloTimeout);
    try{baseline=await request('load-example',{id:root.dataset.example});root.querySelectorAll('button').forEach(b=>b.disabled=false);root.querySelector('[data-action="start"]').disabled=true;say(text.ready)}catch{fail()}
   }else if(m.type==='reply'||m.type==='error'){
    const p=pending.get(m.request);if(!p)return;pending.delete(m.request);clearTimeout(p.timer);m.type==='reply'?p.resolve(m.payload):p.reject(new Error('Command rejected'));
   }else if(m.type==='resize'&&Number.isFinite(m.payload.height)){resizeFrame(frame,m.payload.height)}
  }
  root.querySelector('[data-action="start"]').onclick=()=>{
   if(active&&active!==root){say(text.busy);return}if(frame)return;
   const base=new URL(root.dataset.editor);if(!['https:','http:'].includes(base.protocol)||base.username||base.password){fail();return}
   active=root;origin=base.origin;session=P.token();frame=document.createElement('iframe');frame.title='Aego Asset Studio';frame.className='asset-studio-frame';
   // The editor's local <form method="dialog"> controls need allow-forms.
   // Network form submissions remain denied by the app's form-action CSP.
   frame.setAttribute('sandbox','allow-scripts allow-same-origin allow-forms allow-downloads allow-modals allow-popups allow-popups-to-escape-sandbox');
   frame.setAttribute('allow','fullscreen');frame.referrerPolicy='no-referrer';
   const url=new URL('embed/',base);url.searchParams.set('parentOrigin',location.origin);url.searchParams.set('session',session);url.searchParams.set('lang',lang);
   frame.src=url.href;window.addEventListener('message',receive);mount.append(frame);say(text.starting);
   const hello=()=>P.send(frame.contentWindow,origin,session,'hello','hello',{buildID:root.dataset.build});
   frame.onload=hello;helloTimer=setInterval(hello,250);helloTimeout=setTimeout(fail,45000);
  };
  root.querySelector('[data-action="download"]').onclick=async()=>{try{await request('download')}catch{fail()}};
  root.querySelector('[data-action="reset"]').onclick=async()=>{try{const r=await request('reset-example');if(!r.cancelled)baseline=r;say(text.ready)}catch{fail()}};
  root.querySelector('[data-action="check"]').onclick=async()=>{
   const check=root.dataset.check;if(check==='manual'){say(text.manual);return}
   try{
    const s=await request('summary');let ok=false;
    if(check==='pixels')ok=s.pixelHash!==baseline.pixelHash;
    else if(check==='layers')ok=s.layers>baseline.layers;
    else if(check==='frames')ok=s.frames>baseline.frames;
    else if(check==='linked')ok=s.linkedImages>baseline.linkedImages;
    else if(check==='onion')ok=s.onion;
    else if(check==='character')ok=s.frames>=8&&s.pixelHash!==baseline.pixelHash;
    else if(check==='channels')ok=s.channels.includes('normal')||s.channels.includes('emission');
    say(ok?text.done:text.pending);root.dataset.checkResult=ok?'observed':'pending';
   }catch{fail()}
  };
 }
})();
