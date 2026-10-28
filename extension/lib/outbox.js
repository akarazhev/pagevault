import { captureDigest } from './canonical.js';

const DATABASE = 'pagevault-outbox-v1';
const MAX_ITEMS = 500;
const MAX_BYTES = 128 * 1024 * 1024;
const MAX_CAPTURE_BYTES = 24 * 1024 * 1024;
let databasePromise;

function database() {
  if (!databasePromise) databasePromise = new Promise((resolve, reject) => {
    const request = indexedDB.open(DATABASE, 1);
    request.onupgradeneeded = () => {
      const db = request.result;
      const store = db.createObjectStore('outbox', { keyPath: 'id' });
      store.createIndex('createdAt', 'createdAt');
      db.createObjectStore('meta').put({ pendingCount: 0, queueBytes: 0 }, 'totals');
    };
    request.onerror = () => { databasePromise = null; reject(request.error); };
    request.onsuccess = () => {
      const db = request.result;
      db.onversionchange = () => { db.close(); databasePromise = null; };
      resolve(db);
    };
  });
  return databasePromise;
}

export async function enqueue(capture) {
  const bytes = new TextEncoder().encode(JSON.stringify(capture)).length;
  if (bytes > MAX_CAPTURE_BYTES) throw new Error('Capture exceeds 24 MiB. Reduce the HTML or text limit in settings.');
  const id = await captureDigest(capture);
  const db = await database();
  return new Promise((resolve, reject) => {
    const tx = db.transaction(['outbox', 'meta'], 'readwrite');
    const store = tx.objectStore('outbox'); const meta = tx.objectStore('meta');
    let result, failure;
    tx.oncomplete = () => resolve(result);
    tx.onabort = tx.onerror = () => reject(failure || tx.error || new Error('Could not write to the queue.'));
    const existingRequest = store.get(id);
    existingRequest.onsuccess = () => {
      const existing = existingRequest.result;
      const totalsRequest = meta.get('totals');
      totalsRequest.onsuccess = () => {
        const totals = totalsRequest.result || { pendingCount: 0, queueBytes: 0 };
        const pendingCount = totals.pendingCount + (existing ? 0 : 1);
        const queueBytes = totals.queueBytes - (existing?.bytes || 0) + bytes;
        if (pendingCount > MAX_ITEMS || queueBytes > MAX_BYTES) {
          failure = new Error('The queue is full (500 captures or 128 MiB). Connect the local helper and retry. Previously accepted captures are retained.');
          tx.abort(); return;
        }
        function write(lastCreatedAt) {
          // A durable clock prevents same-ms/hash ties and survives clock rollback.
          // Replacements retain their original queue position.
          const createdAt = existing?.createdAt ?? Math.max(Date.now(), lastCreatedAt + 1);
          store.put({ id, revision: (existing?.revision || 0) + 1, createdAt, capture, bytes, attempts: 0, error: null });
          meta.put(Math.max(lastCreatedAt, createdAt), 'lastCreatedAt');
          meta.put({ pendingCount, queueBytes }, 'totals');
          result = { queued: true, pendingCount, queueBytes, id };
        }
        const clockRequest = meta.get('lastCreatedAt');
        clockRequest.onsuccess = () => {
          if (clockRequest.result !== undefined) { write(clockRequest.result); return; }
          // Existing v1 queues have no counter: initialize after their newest row.
          const latest = store.index('createdAt').openCursor(null, 'prev');
          latest.onsuccess = () => write(latest.result?.value.createdAt ?? 0);
        };
      };
    };
  });
}

export async function peek(limit = 20, { includeRejected = true } = {}) {
  const db = await database();
  return new Promise((resolve, reject) => {
    const tx = db.transaction('outbox', 'readonly');
    const result = [];
    const request = tx.objectStore('outbox').index('createdAt').openCursor();
    request.onsuccess = () => { const cursor = request.result; if (cursor && result.length < limit) { if (includeRejected || !cursor.value.rejected) result.push(cursor.value); cursor.continue(); } };
    tx.oncomplete = () => resolve(result);
    tx.onerror = () => reject(tx.error);
  });
}

export async function remove(id, revision, { rejectedOnly = false } = {}) {
  const db = await database();
  return new Promise((resolve, reject) => {
    const tx = db.transaction(['outbox', 'meta'], 'readwrite');
    const store = tx.objectStore('outbox'); const meta = tx.objectStore('meta');
    const request = store.get(id);
    request.onsuccess = () => {
      const item = request.result;
      if (!item || item.revision !== revision || (rejectedOnly && !item.rejected)) return;
      const totalsRequest = meta.get('totals');
      totalsRequest.onsuccess = () => {
        const totals = totalsRequest.result;
        store.delete(id);
        meta.put({ pendingCount: Math.max(0, totals.pendingCount - 1), queueBytes: Math.max(0, totals.queueBytes - item.bytes) }, 'totals');
      };
    };
    tx.oncomplete = () => resolve();
    tx.onerror = () => reject(tx.error);
  });
}

// Explicit page deletion removes every accepted revision for the exact original URL.
// The cursor deletes and accounting commit together; other URLs remain untouched.
export async function removeUrl(url) {
  const db = await database();
  return new Promise((resolve, reject) => {
    const tx = db.transaction(['outbox', 'meta'], 'readwrite');
    const store = tx.objectStore('outbox'); const meta = tx.objectStore('meta');
    let removed = 0, removedBytes = 0;
    const request = store.openCursor();
    request.onsuccess = () => {
      const cursor = request.result;
      if (cursor) {
        if (cursor.value.capture.url === url) {
          removed++; removedBytes += cursor.value.bytes; cursor.delete();
        }
        cursor.continue();
      } else {
        const totalsRequest = meta.get('totals');
        totalsRequest.onsuccess = () => {
          const totals = totalsRequest.result;
          meta.put({ pendingCount: totals.pendingCount - removed, queueBytes: totals.queueBytes - removedBytes }, 'totals');
        };
      }
    };
    tx.oncomplete = () => resolve(removed);
    tx.onabort = tx.onerror = () => reject(tx.error || new Error('Could not remove captures from the queue.'));
  });
}

export async function markError(id, revision, error, { permanent = false } = {}) {
  const db = await database();
  return new Promise((resolve, reject) => {
    const tx = db.transaction('outbox', 'readwrite'); const store = tx.objectStore('outbox');
    const request = store.get(id);
    request.onsuccess = () => { const item = request.result; if (item && item.revision === revision) store.put({ ...item, attempts: (item.attempts || 0) + 1, error: String(error).slice(0, 1000), rejected: permanent }); };
    tx.oncomplete = () => resolve(); tx.onerror = () => reject(tx.error);
  });
}

export async function getStats() {
  const db = await database();
  return new Promise((resolve, reject) => {
    const tx = db.transaction(['outbox', 'meta'], 'readonly');
    const totals = tx.objectStore('meta').get('totals');
    let rejectedCount = 0;
    const cursor = tx.objectStore('outbox').openCursor();
    cursor.onsuccess = () => { if (cursor.result) { if (cursor.result.value.rejected) rejectedCount++; cursor.result.continue(); } };
    tx.oncomplete = () => resolve({...((totals.result) || {pendingCount:0, queueBytes:0}), rejectedCount});
    tx.onerror = () => reject(tx.error);
  });
}
export async function listRejected() {
  const db = await database();
  return new Promise((resolve, reject) => {
    const tx = db.transaction('outbox', 'readonly'); const result = [];
    const request = tx.objectStore('outbox').index('createdAt').openCursor();
    request.onsuccess = () => {
      const cursor = request.result;
      if (!cursor) return;
      const item = cursor.value;
      if (item.rejected) result.push({id:item.id, revision:item.revision, createdAt:item.createdAt, bytes:item.bytes, error:item.error, url:String(item.capture.url || '').slice(0, 2048), title:String(item.capture.title || '').slice(0, 500)});
      cursor.continue();
    };
    tx.oncomplete = () => resolve(result); tx.onerror = () => reject(tx.error);
  });
}
export async function getRevision(id, revision) {
  const db = await database();
  return new Promise((resolve, reject) => {
    const request = db.transaction('outbox', 'readonly').objectStore('outbox').get(id);
    request.onsuccess = () => resolve(request.result?.revision === revision ? request.result.capture : null);
    request.onerror = () => reject(request.error);
  });
}
export async function retry(id, revision) {
  const db = await database();
  return new Promise((resolve, reject) => {
    const tx = db.transaction('outbox', 'readwrite'); const store = tx.objectStore('outbox');
    const request = store.get(id); let changed = false;
    request.onsuccess = () => { const item = request.result; if (item && item.revision === revision && item.rejected) { store.put({...item, rejected:false, error:null}); changed = true; } };
    tx.oncomplete = () => resolve(changed); tx.onerror = () => reject(tx.error);
  });
}

export function discard(id, revision) { return remove(id, revision, {rejectedOnly:true}); }
