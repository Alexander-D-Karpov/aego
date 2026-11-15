'use strict';
const test=require('node:test'),assert=require('node:assert/strict');
const P=require('../web/protocol.js');
const session='0123456789abcdef0123456789abcdef';
function message(type='summary',payload={}){return P.envelope(session,type,'req-1',payload)}
test('exact origin, source, schema and session are all mandatory',()=>{
 const source={};const event={source,origin:'https://aego.akarpov.ru',data:message()};
 assert.equal(P.accepted(event,source,event.origin,session),true);
 assert.equal(P.accepted(event,{},event.origin,session),false);
 assert.equal(P.accepted(event,source,'https://evil.example',session),false);
 assert.equal(P.accepted(event,source,event.origin,'f'.repeat(32)),false);
 assert.equal(P.accepted(event,source,'*',session),false);
 assert.equal(P.valid({...message(),version:2}),false);
 assert.equal(P.valid({...message(),type:'eval'}),false);
 assert.equal(P.valid({...message(),type:'shell'}),false);
 assert.equal(P.valid({...message(),payload:null}),false);
 assert.equal(P.valid({...message(),request:'x'.repeat(1000)}),false);
});
test('send never allows wildcard origins',()=>{
 let received;const peer={postMessage:(...v)=>{received=v}};
 P.send(peer,'https://aeditor.akarpov.ru',session,'hello','r',{buildID:'x'});assert.equal(received[1],'https://aeditor.akarpov.ru');
 for(const origin of ['*','null',''])assert.throws(()=>P.send(peer,origin,session,'hello','r',{}));
});
test('transfer only accepts bounded, version-matched binary imports',()=>{
 const p={operation:'import',format:'aeart',buildID:'build',documentVersion:1,bytes:new ArrayBuffer(8),size:8,revision:4,sha256:'a'.repeat(64)};
 assert.equal(P.snapshotShape(p,'build',1),true);
 for(const change of [{operation:'replace'},{operation:'merge'},{format:'shell'},{buildID:'other'},{documentVersion:2},{bytes:'base64'},{size:9},{revision:NaN},{sha256:'bad'}])assert.equal(!!P.snapshotShape({...p,...change},'build',1),false);
 assert.equal(P.snapshotShape({...p,bytes:new ArrayBuffer(0),size:0},'build',1),false);
});
test('SHA-256 is computed over exact snapshot bytes',async()=>{
 assert.equal(await P.sha256(new Uint8Array([97,98,99])),'ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad');
 assert.match(P.token(),/^[a-f0-9]{32}$/);
});
