// The queue is acknowledged only after the transport reports a committed capture.
// remove(id, revision) prevents an in-flight acknowledgement erasing a newer enqueue.
export async function deliverBatch(queue, send, { maxItems = 20 } = {}) {
  let sent = 0;
  while (sent < maxItems) {
    const [item] = await queue.peek(1);
    if (!item) return { sent, error: null };
    try {
      const receipt = await send(item.capture);
      if (!receipt || !Number.isInteger(receipt.capture_id)) throw new Error('Помощник не подтвердил запись снимка.');
      await queue.remove(item.id, item.revision);
      sent++;
    } catch (error) {
      const message = error?.message || String(error);
      await queue.markError(item.id, message);
      return { sent, error: message };
    }
  }
  return { sent, error: null };
}
