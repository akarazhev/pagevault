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
  if (bytes > MAX_CAPTURE_BYTES) throw new Error('Снимок превышает 24 МиБ. Уменьшите лимит HTML или текста в настройках.');
  const id = await captureDigest(capture);
  const db = await database();
  return new Promise((resolve, reject) => {
    const tx = db.transaction(['outbox', 'meta'], 'readwrite');
    const store = tx.objectStore('outbox'); const meta = tx.objectStore('meta');
    let result, failure;
    tx.oncomplete = () => resolve(result);
    tx.onabort = tx.onerror = () => reject(failure || tx.error || new Error('Не удалось записать очередь.'));
    const existingRequest = store.get(id);
    existingRequest.onsuccess = () => {
      const existing = existingRequest.result;
      const totalsRequest = meta.get('totals');
      totalsRequest.onsuccess = () => {
        const totals = totalsRequest.result || { pendingCount: 0, queueBytes: 0 };
        const pendingCount = totals.pendingCount + (existing ? 0 : 1);
        const queueBytes = totals.queueBytes - (existing?.bytes || 0) + bytes;
        if (pendingCount > MAX_ITEMS || queueBytes > MAX_BYTES) {
          failure = new Error('Очередь заполнена (500 снимков или 128 МиБ). Подключите локальный помощник и повторите сохранение. Уже принятые снимки сохранены.');
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

export async function peek(limit = 20) {
  const db = await database();
  return new Promise((resolve, reject) => {
    const tx = db.transaction('outbox', 'readonly');
    const result = [];
    const request = tx.objectStore('outbox').index('createdAt').openCursor();
    request.onsuccess = () => { const cursor = request.result; if (cursor && result.length < limit) { result.push(cursor.value); cursor.continue(); } };
    tx.oncomplete = () => resolve(result);
    tx.onerror = () => reject(tx.error);
  });
}

export async function remove(id, revision) {
  const db = await database();
  return new Promise((resolve, reject) => {
    const tx = db.transaction(['outbox', 'meta'], 'readwrite');
    const store = tx.objectStore('outbox'); const meta = tx.objectStore('meta');
    const request = store.get(id);
    request.onsuccess = () => {
      const item = request.result;
      if (!item || item.revision !== revision) return;
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
    tx.onabort = tx.onerror = () => reject(tx.error || new Error('Не удалось удалить снимки из очереди.'));
  });
}

export async function markError(id, error) {
  const db = await database();
  return new Promise((resolve, reject) => {
    const tx = db.transaction('outbox', 'readwrite'); const store = tx.objectStore('outbox');
    const request = store.get(id);
    request.onsuccess = () => { const item = request.result; if (item) store.put({ ...item, attempts: item.attempts + 1, error: String(error).slice(0, 1000) }); };
    tx.oncomplete = () => resolve(); tx.onerror = () => reject(tx.error);
  });
}

export async function getStats() {
  const db = await database();
  return new Promise((resolve, reject) => {
    const request = db.transaction('meta', 'readonly').objectStore('meta').get('totals');
    request.onsuccess = () => resolve(request.result || { pendingCount: 0, queueBytes: 0 });
    request.onerror = () => reject(request.error);
  });
}
