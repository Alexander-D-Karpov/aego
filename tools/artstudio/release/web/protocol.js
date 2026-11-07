/* Host communication only. No document or pixel algorithms live here. */
'use strict';
(function (root) {
  const CHANNEL = 'aego.asset-studio', VERSION = 1, MAX_BYTES = 64 * 1024 * 1024;
  const TYPES = new Set(['hello', 'ready', 'summary', 'load-example', 'reset-example', 'download', 'open-full', 'resize', 'reply', 'error', 'transfer-ready', 'transfer-document', 'transfer-ack']);
  const TOKEN = /^[a-f0-9]{32}$/, REQUEST = /^[a-zA-Z0-9_-]{1,80}$/;
  function token() { return Array.from(root.crypto.getRandomValues(new Uint8Array(16)), n => n.toString(16).padStart(2, '0')).join(''); }
  function valid(m, session) {
    return !!m && typeof m === 'object' && !Array.isArray(m) && m.channel === CHANNEL && m.version === VERSION &&
      typeof m.session === 'string' && TOKEN.test(m.session) && (!session || m.session === session) && TYPES.has(m.type) &&
      typeof m.request === 'string' && REQUEST.test(m.request) && !!m.payload && typeof m.payload === 'object' && !Array.isArray(m.payload);
  }
  function envelope(session, type, request, payload = {}) {
    const m = {channel: CHANNEL, version: VERSION, session, type, request, payload};
    if (!valid(m)) throw new Error('Invalid protocol envelope'); return m;
  }
  function accepted(event, source, origin, session) {
    return !!source && origin !== '*' && origin !== 'null' && event.source === source && event.origin === origin && valid(event.data, session);
  }
  function send(target, origin, session, type, request, payload) {
    if (!target || !origin || origin === '*' || origin === 'null') throw new Error('Exact target origin required');
    target.postMessage(envelope(session, type, request, payload), origin);
  }
  async function sha256(bytes) {
    const b = await root.crypto.subtle.digest('SHA-256', bytes);
    return Array.from(new Uint8Array(b), n => n.toString(16).padStart(2, '0')).join('');
  }
  function snapshotShape(p, buildID, documentVersion) {
    return p && p.operation === 'import' && p.format === 'aeart' && p.buildID === buildID && p.documentVersion === documentVersion &&
      p.bytes instanceof ArrayBuffer && p.bytes.byteLength > 0 && p.bytes.byteLength <= MAX_BYTES && p.bytes.byteLength === p.size &&
      /^[a-f0-9]{64}$/.test(p.sha256) && Number.isSafeInteger(p.revision) && p.revision >= 0;
  }
  const api = Object.freeze({CHANNEL, VERSION, MAX_BYTES, TYPES, token, valid, envelope, accepted, send, sha256, snapshotShape});
  root.AegoStudioProtocol = api;
  if (typeof module !== 'undefined' && module.exports) module.exports = api;
})(globalThis);
