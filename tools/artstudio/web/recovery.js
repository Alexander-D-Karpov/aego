/* Origin-local persistence only. Documents and pixel serialization live in Go.
 * Never evict another unsaved drawing to make room: quota failures are visible. */
'use strict';
class AegoRecoveryStore {
  static MAX_BYTES = 128 * 1024 * 1024;
  static MAX_RECORDS = 128;
  constructor(db) { this.db = db; }
  static open() {
    return new Promise((resolve, reject) => {
      const req = indexedDB.open('aego-artstudio', 2);
      req.onupgradeneeded = () => {
        if (!req.result.objectStoreNames.contains('documents')) req.result.createObjectStore('documents', { keyPath: 'key' });
        if (req.result.objectStoreNames.contains('recovery')) {
          const old = req.transaction.objectStore('recovery').get('current');
          old.onsuccess = () => {
            if (old.result?.dirty && old.result?.bytes) req.transaction.objectStore('documents').put({ ...old.result, key: 'legacy-current' });
          };
        }
      };
      req.onerror = () => reject(req.error);
      req.onblocked = () => reject(new Error('Close other Asset Studio windows to upgrade local recovery.'));
      req.onsuccess = () => {
        const db = req.result;
        db.onversionchange = () => db.close();
        resolve(new AegoRecoveryStore(db));
      };
    });
  }
  list() {
    return new Promise((resolve, reject) => {
      const tx = this.db.transaction('documents', 'readonly');
      const req = tx.objectStore('documents').getAll();
      tx.oncomplete = () => resolve(req.result.sort((a, b) => b.at - a.at));
      tx.onerror = tx.onabort = () => reject(tx.error || new Error('Recovery read aborted.'));
    });
  }
  put(record) {
    if (!record?.key || !(record.bytes instanceof Uint8Array) || record.bytes.byteLength > 64 * 1024 * 1024) {
      return Promise.reject(new Error('Invalid recovery record.'));
    }
    return new Promise((resolve, reject) => {
      const tx = this.db.transaction('documents', 'readwrite');
      const store = tx.objectStore('documents');
      let failure;
      const all = store.getAll();
      all.onsuccess = () => {
        const others = all.result.filter(v => v.key !== record.key);
        const bytes = others.reduce((n, v) => n + (v.bytes?.byteLength || 0), record.bytes.byteLength);
        if (bytes > AegoRecoveryStore.MAX_BYTES || others.length >= AegoRecoveryStore.MAX_RECORDS) {
          failure = new Error('Recovery storage limit reached; save or explicitly discard older drawings.');
          tx.abort();
          return;
        }
        store.put(record);
      };
      tx.oncomplete = resolve;
      tx.onerror = tx.onabort = () => reject(failure || tx.error || new Error('Recovery write aborted.'));
    });
  }
  remove(key) {
    return new Promise((resolve, reject) => {
      const tx = this.db.transaction('documents', 'readwrite');
      tx.objectStore('documents').delete(key);
      tx.oncomplete = resolve;
      tx.onerror = tx.onabort = () => reject(tx.error || new Error('Recovery deletion aborted.'));
    });
  }
}
window.AegoRecoveryStore = AegoRecoveryStore;
