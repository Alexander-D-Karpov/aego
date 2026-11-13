'use strict';
const test=require('node:test'),assert=require('node:assert/strict');
const fs=require('node:fs'),path=require('node:path'),vm=require('node:vm');
const realProtocol=require('../web/protocol.js');
const source=fs.readFileSync(path.join(__dirname,'../web/docs-embed.js'),'utf8');
const session='0123456789abcdef0123456789abcdef';

function fixture(){
 const sent=[],intervals=new Map(),timeouts=new Map();let nextTimer=0,handler,frame;
 const buttons=Object.fromEntries(['start','check','download','reset'].map(name=>[name,{disabled:name!=='start'}]));
 const status={textContent:''},mount={append(value){frame=value}};
 const root={dataset:{lang:'en',editor:'https://editor.example/',build:'build',example:'test',check:'manual'},
  querySelector(selector){
   if(selector==='.embed-status')return status;
   if(selector==='.embed-mount')return mount;
   const match=selector.match(/^\[data-action="([^"]+)"\]$/);
   if(match)return buttons[match[1]];
   throw new Error('unexpected selector '+selector);
  },querySelectorAll(selector){assert.equal(selector,'button');return Object.values(buttons)}};
 const P={...realProtocol,token:()=>session};
 const window={AegoStudioProtocol:P,addEventListener(type,fn){assert.equal(type,'message');handler=fn}};
 const document={querySelectorAll(selector){assert.equal(selector,'.aego-studio-embed');return [root]},
  createElement(name){assert.equal(name,'iframe');return {setAttribute(){},contentWindow:{postMessage(message,origin){sent.push({message,origin})}}}}};
 const clock={
  setInterval(fn,delay){const id=++nextTimer;intervals.set(id,{fn,delay});return id},
  clearInterval(id){intervals.delete(id)},
  setTimeout(fn,delay){const id=++nextTimer;timeouts.set(id,{fn,delay});return id},
  clearTimeout(id){timeouts.delete(id)},
 };
 vm.runInNewContext(source,{window,document,location:{origin:'https://docs.example'},URL,Map,Math,Number,Error,...clock});
 function dispatch(type,payload,origin='https://editor.example',request='hello'){
  return handler({source:frame.contentWindow,origin,data:P.envelope(session,type,request,payload)});
 }
 return {buttons,status,sent,intervals,timeouts,dispatch,get frame(){return frame}};
}

test('iframe handshake starts only after the first load, with an exact target origin',async()=>{
 const f=fixture();f.buttons.start.onclick();
 assert.equal(f.sent.length,0,'about:blank must never receive a hello');
 assert.equal(f.intervals.size,0,'retries must wait for the iframe load');
 f.frame.onload();
 assert.equal(f.sent.length,1);assert.equal(f.sent[0].message.type,'hello');
 assert.equal(f.sent[0].origin,'https://editor.example');
 assert.equal(f.intervals.size,1);
 [...f.intervals.values()][0].fn();assert.equal(f.sent.length,2);
 await f.dispatch('ready',{buildID:'build',protocol:realProtocol.VERSION},'https://wrong.example');
 assert.equal(f.sent.length,2,'a wrong-origin reply must be ignored');
});

test('a later iframe navigation disconnects the session without another hello',async()=>{
 const f=fixture();f.buttons.start.onclick();f.frame.onload();
 const ready=f.dispatch('ready',{buildID:'build',protocol:realProtocol.VERSION});
 const request=f.sent.at(-1).message;assert.equal(request.type,'load-example');
 await f.dispatch('reply',{pixelHash:'x',layers:1,frames:1},'https://editor.example',request.request);
 await ready;
 assert.equal(f.buttons.check.disabled,false);
 const before=f.sent.length;f.frame.onload();
 assert.equal(f.sent.length,before,'do not send to the navigated document');
 assert.equal(f.intervals.size,0);
 assert.match(f.status.textContent,/Editor unavailable/);
 assert.ok(Object.values(f.buttons).every(button=>button.disabled),'disconnected controls must be disabled');
 await f.buttons.download.onclick();
 assert.equal(f.sent.length,before,'an action after disconnect must not send');
 await f.dispatch('ready',{buildID:'build',protocol:realProtocol.VERSION});
 assert.equal(f.sent.length,before,'a disconnected session must not restart');
});

test('a timeout or second load before readiness cannot start another handshake',()=>{
 const timeout=fixture();timeout.buttons.start.onclick();
 const deadline=[...timeout.timeouts.values()].find(timer=>timer.delay===45000);
 assert.ok(deadline);deadline.fn();timeout.frame.onload();
 assert.equal(timeout.sent.length,0);assert.equal(timeout.intervals.size,0);
 const navigation=fixture();navigation.buttons.start.onclick();navigation.frame.onload();
 const before=navigation.sent.length;navigation.frame.onload();
 assert.equal(navigation.sent.length,before);assert.equal(navigation.intervals.size,0);
 assert.match(navigation.status.textContent,/Editor unavailable/);
});

test('a reply racing with navigation cannot re-enable the disconnected editor',async()=>{
 const f=fixture();f.buttons.start.onclick();f.frame.onload();
 const ready=f.dispatch('ready',{buildID:'build',protocol:realProtocol.VERSION});
 const request=f.sent.at(-1).message;
 const reply=f.dispatch('reply',{pixelHash:'x',layers:1,frames:1},'https://editor.example',request.request);
 f.frame.onload();
 await Promise.all([ready,reply]);
 assert.match(f.status.textContent,/Editor unavailable/);
 assert.ok(Object.values(f.buttons).every(button=>button.disabled));
});
