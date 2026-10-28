// The queue is acknowledged only after the transport reports a committed capture.
// remove(id, revision) prevents an in-flight acknowledgement erasing a newer enqueue.
export async function deliverBatch(queue, send, { maxItems = 20 } = {}) {
  let sent = 0,
    rejected = 0,
    attempted = 0;
  while (attempted < maxItems) {
    const [item] = await queue.peek(1, { includeRejected: false });
    if (!item) return { sent, rejected, error: null };
    attempted++;
    try {
      const receipt = await send(item.capture);
      if (!receipt || !Number.isInteger(receipt.capture_id))
        throw new Error('The helper did not confirm the capture was saved.');
      await queue.remove(item.id, item.revision);
      sent++;
    } catch (error) {
      const message = error?.message || String(error);
      const permanent = error?.code === 'invalid_request';
      await queue.markError(item.id, item.revision, message, { permanent });
      if (permanent) {
        rejected++;
        continue;
      }
      return { sent, rejected, error: message };
    }
  }
  return { sent, rejected, error: null };
}
