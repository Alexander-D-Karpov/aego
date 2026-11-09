'use strict';
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const test = require('node:test');
const vm = require('node:vm');

// Exercise the shipped recovery scheduler with faulting asynchronous storage.
// The browser E2E separately covers real IndexedDB, reload and site-data denial.
const source = fs.readFileSync(path.join(__dirname, '../../web/studio.js'), 'utf8');
const code = source.slice(source.indexOf('function queueRecovery('), source.indexOf('async function showRecovery('));
assert(code.includes('function flushRecovery('));

function harness() {
  const status = {active:'a', state:{gesture:false}, tabs:[
    {key:'a',name:'A.aeart',dirty:true,revision:1},
    {key:'b',name:'B.aeart',dirty:true,revision:1}
  ]};
  const exported = [], stored = [], removed = [], errors = [];
  let fail = false;
  const context = vm.createContext({
    status, exported, stored, removed, errors,
    setTimeout, clearTimeout, Uint8Array, Map, Promise, Event,
    browserSession:'test', recoveryKeys:new Map(), recoveryVersions:new Map(),
    recoveryWrites:Promise.resolve(), recoveryTimer:undefined,
    t:key=>key, say:(message,error)=>{ if(error)errors.push(message); },
    window:{dispatchEvent:()=>{}},
    api:{
      request:()=>JSON.stringify(status),
      exportBytes:(format,columns,handle)=>{
        exported.push(handle);
        return {bytes:new Uint8Array([1,2,3]),revision:status.tabs.find(tab=>tab.key===handle).revision};
      }
    },
    recoveryStore:{
      put:async snapshot=>{ if(fail)throw new Error('quota'); stored.push(snapshot); },
      remove:async key=>{removed.push(key);}
    }
  });
  vm.runInContext(code,context);
  return {context,status,exported,stored,removed,errors,
    flush:()=>vm.runInContext('flushRecovery()',context),
    settled:()=>vm.runInContext('recoveryWrites',context),
    fail:value=>{fail=value;}};
}

test('unchanged dirty documents retain independent snapshots without repeated exports',async()=>{
  const h=harness();h.flush();h.flush();await h.settled();
  assert.deepEqual(h.exported,['a','b']);
  assert.equal(h.stored.length,2);
  assert.notEqual(h.stored[0].key,h.stored[1].key);
  h.status.tabs[0].revision=2;h.flush();await h.settled();
  assert.deepEqual(h.exported,['a','b','a']);
  assert.equal(h.stored[2].revision,2);
  assert(h.status.tabs.every(tab=>tab.dirty),'recovery must not clear dirty state');
});

test('failed storage warns and retries exactly the same revision',async()=>{
  const h=harness();h.fail(true);h.flush();await h.settled();
  assert.equal(h.stored.length,0);assert.equal(h.errors.length,2);
  h.fail(false);h.flush();await h.settled();
  assert.deepEqual(h.exported,['a','b','a','b']);assert.equal(h.stored.length,2);
  h.flush();await h.settled();assert.equal(h.exported.length,4);
  assert(h.status.tabs.every(tab=>tab.dirty));
});

test('gesture deferral and a completed save do not suppress later recovery',async()=>{
  const h=harness();h.status.state.gesture=true;h.flush();await h.settled();
  assert.deepEqual(h.exported,['b']);assert.deepEqual(h.errors,[]);
  h.status.state.gesture=false;h.flush();await h.settled();
  assert.deepEqual(h.exported,['b','a']);
  h.status.tabs[0].dirty=false;h.flush();await h.settled();assert.equal(h.removed.length,1);
  h.status.tabs[0].dirty=true;h.flush();await h.settled();
  assert.deepEqual(h.exported,['b','a','a']);
});

// Run the shipped handlers, including their async boundaries. Storage is shared
// between two independent page contexts, as it is for same-origin IndexedDB.
const restoreCode = source.slice(source.indexOf('async function showRecovery('), source.indexOf("window.addEventListener('pagehide'"));
const closeCode = source.slice(source.indexOf('function closeDocument('), source.indexOf('\n\nconst t ='));
const openCode = source.slice(source.indexOf('async function openFile('), source.indexOf("\n$('file-input')"));
assert(restoreCode.includes("$('restore').onclick") && closeCode.includes('document-close') && openCode.includes('await file.arrayBuffer()'));

function copyRecord(record) { return {...record,bytes:new Uint8Array(record.bytes)}; }
function restoringPage(shared, browserSession) {
  const elements = new Map(), messages = [], documents = new Map([['document-1',new Uint8Array([0])]]);
  let serial = 1, imports = 0, readGate = null;
  function element(id) {
    if (!elements.has(id)) elements.set(id, {value:'',disabled:false,
      classList:{toggle(){}},addEventListener(type,fn){this[type]=fn;},
      replaceChildren(...items){this.options=items;this.value=items[0]?.value||'';}});
    return elements.get(id);
  }
  const status = {active:'document-1',state:{gesture:false},tabs:[{key:'document-1',name:'Untitled.aeart',revision:1,dirty:false}]};
  const context = vm.createContext({
    Uint8Array, Map, Set, Promise, Event, browserSession, recoveryData:[], recoveryBusy:false,
    restoredRecoverySources:new Set(),recoveryKeys:new Map(),recoveryVersions:new Map(),views:new Map(),
    recoveryWrites:Promise.resolve(),recoveryTimer:undefined,activeDocument:status.active,
    state:status.state,tabs:status.tabs,lang:'en',$:element,
    Option:class {constructor(text,value){this.text=text;this.value=value;}},
    setTimeout:()=>0,clearTimeout(){},window:{dispatchEvent(){}},
    t:key=>key,say:(text,error=false)=>messages.push({text,error}),confirm:()=>true,
    cancelPointer(){status.state.gesture=false;},stopPlayback(){},renderUI(){},restoreView(){},paint(){},
    adopt(response){context.activeDocument=response.active;context.state=response.state;context.tabs=response.tabs;},
    call(request){
      if(request.action==='recovered')status.tabs.find(tab=>tab.key===status.active).dirty=true;
      else if(request.action==='document-close'){
        status.tabs=status.tabs.filter(tab=>tab.key!==request.handle);documents.delete(request.handle);
        if(status.active===request.handle)status.active=status.tabs[0].key;
      } else throw new Error('Unexpected request '+request.action);
      context.adopt(status);return status;
    },
    api:{
      request:()=>JSON.stringify(status),
      importBytes(bytes,format,replace,name){
        imports++;const key='document-'+(++serial);documents.set(key,new Uint8Array(bytes));
        status.active=key;status.tabs.push({key,name,revision:1,dirty:false});
        return JSON.stringify(status);
      },
      exportBytes(format,columns,handle){return {bytes:new Uint8Array(documents.get(handle)),revision:status.tabs.find(tab=>tab.key===handle).revision};}
    },
    recoveryStore:{
      async list(){if(readGate)await readGate;return [...shared.values()].map(copyRecord);},
      async put(record){shared.set(record.key,copyRecord(record));},
      async remove(key){shared.delete(key);}
    }
  });
  vm.runInContext(code+'\n'+closeCode+'\n'+restoreCode,context);
  return {
    context,status,messages,elements,
    show:()=>vm.runInContext('showRecovery()',context),
    restore:()=>element('restore').onclick(),
    discard:()=>element('dismiss-recovery').onclick(),
    choose(key){element('recovery-choice').value=key;element('recovery-choice').change();},
    async flush(){vm.runInContext('flushRecovery()',context);await vm.runInContext('recoveryWrites',context);},
    async close(handle){context.closeDocument(status.tabs.find(tab=>tab.key===handle));await vm.runInContext('recoveryWrites',context);},
    key:handle=>context.recoveryKeys.get(handle),
    edit(value){documents.set(status.active,new Uint8Array([value]));const tab=status.tabs.find(tab=>tab.key===status.active);tab.revision++;tab.dirty=true;},
    pauseList(){let release;readGate=new Promise(resolve=>release=resolve);return ()=>{readGate=null;release();};},
    get imports(){return imports;}
  };
}

function recoveryFixture() {
  const record={key:'original/session-document',bytes:new Uint8Array([7]),filename:'Original.aeart',revision:1,dirty:true,at:1};
  return {record,shared:new Map([[record.key,copyRecord(record)]])};
}

test('Restore cannot run twice during the async list refresh or again in the same page',async()=>{
  const {record,shared}=recoveryFixture(),page=restoringPage(shared,'page-a');
  await page.show();const release=page.pauseList();
  const restoring=page.restore();
  assert.equal(page.imports,1);assert.equal(page.elements.get('restore').disabled,true);
  assert.equal(page.elements.get('dismiss-recovery').disabled,true,'discard is guarded while restore is refreshing');
  await page.restore();assert.equal(page.imports,1);
  await page.discard();assert(shared.has(record.key),'busy restore must not discard the original');
  release();await restoring;
  page.choose(record.key);await page.restore();assert.equal(page.imports,1);
  assert.equal(page.elements.get('restore').disabled,true);
  assert.equal(page.elements.get('dismiss-recovery').disabled,false,'original remains available for explicit discard');
  assert(page.messages.some(message=>message.text==='recovery-copy'));
  assert(shared.has(record.key),'restoring must not delete another page\'s original');
  await page.discard();assert(!shared.has(record.key),'explicit discard did not remove the selected original');
});

test('two pages restoring one snapshot own separate recovery records through edits and close',async()=>{
  const {record,shared}=recoveryFixture(),a=restoringPage(shared,'page-a'),b=restoringPage(shared,'page-b');
  await Promise.all([a.show(),b.show()]);await Promise.all([a.restore(),b.restore()]);
  const aHandle=a.status.active,bHandle=b.status.active,aKey=a.key(aHandle),bKey=b.key(bHandle);
  assert.notEqual(aKey,bKey);assert.notEqual(aKey,record.key);assert.notEqual(bKey,record.key);
  a.edit(11);b.edit(22);await a.flush();await b.flush();
  assert.deepEqual([...shared.get(aKey).bytes],[11]);assert.deepEqual([...shared.get(bKey).bytes],[22]);
  assert.deepEqual([...shared.get(record.key).bytes],[7]);
  await a.close(aHandle);await b.flush();
  assert(!shared.has(aKey));assert.deepEqual([...shared.get(bKey).bytes],[22]);
  assert.deepEqual([...shared.get(record.key).bytes],[7],'closing a restored copy deleted the live original');
  b.edit(23);await b.flush();assert.deepEqual([...shared.get(bKey).bytes],[23]);
  assert.equal(b.imports,1);
});

test('a deferred file read cannot cancel or redirect a newly started gesture',async()=>{
  let finishRead,gesture=false,imports=0,cancels=0;
  const read=new Promise(resolve=>finishRead=resolve);
  const context=vm.createContext({Uint8Array,pointer:null,t:key=>key,
    api:{request:()=>JSON.stringify({state:{gesture}}),importBytes(){imports++;return '{}';}},
    cancelPointer(){gesture=false;cancels++;},stopPlayback(){},captureView(){},adopt(){},renderUI(){},restoreView(){},paint(){},scheduleRecovery(){},say(){}});
  vm.runInContext(openCode,context);
  const opening=context.openFile({size:3,name:'Other.aeart',arrayBuffer:()=>read});
  gesture=true;context.pointer=9;finishRead(new Uint8Array([1,2,3]).buffer);
  await assert.rejects(opening,/open-busy/);
  assert.equal(imports,0);assert.equal(cancels,1);assert.equal(gesture,true);assert.equal(context.pointer,9);
  gesture=false;context.pointer=null;
  await context.openFile({size:3,name:'Other.aeart',arrayBuffer:async()=>new Uint8Array([1,2,3]).buffer});
  assert.equal(imports,1);
});

test('a deferred file read also preserves an unfinished pointer selection',async()=>{
  let finishRead,imports=0;
  const read=new Promise(resolve=>finishRead=resolve);
  const context=vm.createContext({Uint8Array,pointer:null,t:key=>key,
    api:{request:()=>JSON.stringify({state:{gesture:false}}),importBytes(){imports++;return '{}';}},
    cancelPointer(){},stopPlayback(){},captureView(){}});
  vm.runInContext(openCode,context);
  const opening=context.openFile({size:1,name:'Other.aeart',arrayBuffer:()=>read});
  context.pointer=4;finishRead(new Uint8Array([1]).buffer);
  await assert.rejects(opening,/open-busy/);assert.equal(imports,0);assert.equal(context.pointer,4);
});
