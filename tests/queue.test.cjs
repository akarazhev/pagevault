const {test,before,after}=require('node:test');
const assert=require('node:assert/strict');
const {playwright,browserOptions,serveProject}=require('./helpers.cjs');
let server,browser;
before(async()=>{server=await serveProject();browser=await playwright().chromium.launch(browserOptions());});
after(async()=>{await browser?.close();await server?.close();});

test('outbox persists across page reload and preserves newer revision after old acknowledgement',async()=>{
  const context=await browser.newContext();const page=await context.newPage();await page.goto(server.url+'/blank');
  const original=await page.evaluate(async()=>{
    const q=await import('/extension/lib/outbox.js');
    const capture={schema_version:1,url:'https://example.org/',title:'Сохранение',blocks:[{type:'paragraph',text:'Тест'}],captured_at:'2026-10-27T10:00:00Z'};
    await q.enqueue(capture);return (await q.peek())[0];
  });
  await page.reload();
  const result=await page.evaluate(async(first)=>{
    const q=await import('/extension/lib/outbox.js');
    const before=await q.getStats();
    await q.enqueue({...first.capture,captured_at:'2026-10-27T11:00:00Z'});
    await q.remove(first.id,first.revision);
    const afterOld=await q.getStats();const [newer]=await q.peek();
    await q.remove(newer.id,newer.revision);
    return {before,afterOld,afterNew:await q.getStats(),newTime:newer.capture.captured_at};
  },original);
  assert.equal(result.before.pendingCount,1);assert.equal(result.afterOld.pendingCount,1);
  assert.equal(result.afterNew.pendingCount,0);assert.equal(result.newTime,'2026-10-27T11:00:00Z');
  await context.close();
});

test('offline snapshots with changed content remain ordered and survive failed delivery',async()=>{
  const context=await browser.newContext();const page=await context.newPage();await page.goto(server.url+'/blank');
  const result=await page.evaluate(async()=>{
    const q=await import('/extension/lib/outbox.js');const {deliverBatch}=await import('/extension/lib/delivery.js');
    const originalNow=Date.now;Date.now=()=>1777777777777;
    try{
      for(const text of ['Первое сообщение','Новый ответ'])await q.enqueue({url:'https://forum.example/thread',blocks:[{type:'paragraph',text}]});
    }finally{Date.now=originalNow;}
    const fail=await deliverBatch(q,async()=>{throw new Error('offline');});
    return {fail,stats:await q.getStats(),items:(await q.peek()).map(x=>x.capture.blocks[0].text)};
  });
  assert.equal(result.stats.pendingCount,2);assert.equal(result.fail.sent,0);
  assert.deepEqual(result.items,['Первое сообщение','Новый ответ']);assert.ok(result.stats.queueBytes>0);
  await context.close();
});

test('URL purge atomically removes every accepted matching revision and retains unrelated queue bytes',async()=>{
  const context=await browser.newContext();const page=await context.newPage();await page.goto(server.url+'/blank');
  const result=await page.evaluate(async()=>{
    const q=await import('/extension/lib/outbox.js');
    const url='https://example.org/page?q=1#state';
    await q.enqueue({url,blocks:[{text:'old'}]});await q.enqueue({url,blocks:[{text:'changed'}]});
    await q.enqueue({url:url+'-other',blocks:[{text:'keep'}]});
    const removed=await q.removeUrl(url);
    const [remaining]=await q.peek();
    return{removed,stats:await q.getStats(),url:remaining.capture.url,remainingBytes:remaining.bytes};
  });
  assert.equal(result.removed,2);assert.equal(result.stats.pendingCount,1);
  assert.equal(result.stats.queueBytes,result.remainingBytes);assert.equal(result.url,'https://example.org/page?q=1#state-other');
  await context.close();
});


test('outbox ordering survives revisions, acknowledgement, URL purge, reload and clock rollback',async()=>{
  const context=await browser.newContext();const page=await context.newPage();await page.goto(server.url+'/blank');
  const before=await page.evaluate(async()=>{
    const q=await import('/extension/lib/outbox.js');
    const originalNow=Date.now;Date.now=()=>1000;
    try{
      const a={url:'https://example.org/a',blocks:[{text:'first'}]};
      await q.enqueue(a);await q.enqueue({url:'https://example.org/b',blocks:[{text:'second'}]});
      await q.enqueue({...a,captured_at:'later revision'});
      const entries=await q.peek();
      await q.remove(entries[0].id,entries[0].revision);
      await q.removeUrl('https://example.org/b');
      return {entries,stats:await q.getStats()};
    }finally{Date.now=originalNow;}
  });
  assert.deepEqual(before.entries.map(x=>x.capture.url),['https://example.org/a','https://example.org/b']);
  assert.equal(before.entries[0].revision,2);assert.equal(before.entries[0].createdAt,1000);
  assert.equal(before.entries[1].createdAt,1001);assert.equal(before.stats.pendingCount,0);
  await page.reload();
  const after=await page.evaluate(async()=>{
    const q=await import('/extension/lib/outbox.js');
    const originalNow=Date.now;Date.now=()=>1;
    try{
      await q.enqueue({url:'https://example.org/c',blocks:[]});return(await q.peek())[0];
    }finally{Date.now=originalNow;}
  });
  assert.equal(after.createdAt,1002);
  await context.close();
});
