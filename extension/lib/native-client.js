const UNAVAILABLE = 'SQLite helper unavailable. Check the installation: ';
const INVALID_RESPONSE = 'Invalid response from the local helper.';
const READ_TIMEOUT =
  'The local helper did not respond within 20 seconds. The capture remains queued.';

// One instance belongs to one background worker. Requests share a lazy native
// session; the host keeps its SQLite connection until the actual port closes.
export function createNativeClient(runtime, hostName, options = {}) {
  const randomUUID = options.randomUUID || (() => crypto.randomUUID());
  const schedule = options.setTimeout || globalThis.setTimeout;
  const cancel = options.clearTimeout || globalThis.clearTimeout;
  const pending = new Map();
  let session = null;

  function finish(id, entry, error, data) {
    if (pending.get(id) !== entry) return;
    pending.delete(id);
    if (entry.timer !== null) cancel(entry.timer);
    if (error) entry.reject(error);
    else entry.resolve(data);
  }

  function connect() {
    if (session) return session;
    const generation = { port: runtime.connectNative(hostName) };
    generation.port.onMessage.addListener((response) => {
      if (session !== generation) return;
      const entry = pending.get(response?.id);
      // Unknown IDs include late responses to reads that already timed out.
      if (!entry || entry.generation !== generation) return;
      if (typeof response.ok !== 'boolean') {
        finish(response.id, entry, new Error(INVALID_RESPONSE));
      } else if (!response.ok) {
        const error = new Error(response.error?.message || 'Local archive error.');
        error.code = response.error?.code;
        finish(response.id, entry, error);
      } else {
        finish(response.id, entry, null, response.data);
      }
    });
    generation.port.onDisconnect.addListener(() => {
      // lastError only exists during the browser's disconnect callback. Read it
      // even for an old port, but never invalidate a replacement generation.
      const reason = runtime.lastError?.message || 'The local helper connection closed.';
      if (session === generation) session = null;
      for (const [id, entry] of pending) {
        if (entry.generation === generation) finish(id, entry, new Error(UNAVAILABLE + reason));
      }
    });
    session = generation;
    return generation;
  }

  function request(message) {
    return new Promise((resolve, reject) => {
      let generation;
      let id;
      try {
        generation = connect();
        id = randomUUID();
      } catch (error) {
        reject(new Error(UNAVAILABLE + (error?.message || String(error))));
        return;
      }
      const entry = { generation, resolve, reject, timer: null };
      pending.set(id, entry);
      // A synthetic mutation timeout would release nativeWrite while a save or
      // delete can still commit. Only reads have a deadline; none close the port.
      if (message.type !== 'capture.save' && message.type !== 'pages.delete') {
        entry.timer = schedule(() => finish(id, entry, new Error(READ_TIMEOUT)), 20000);
      }
      try {
        generation.port.postMessage({ ...message, id });
      } catch (error) {
        // Serialization/send failure does not prove other requests disconnected.
        // The port's actual onDisconnect event owns shared session cleanup.
        finish(id, entry, new Error(UNAVAILABLE + (error?.message || String(error))));
      }
    });
  }

  return { request };
}
