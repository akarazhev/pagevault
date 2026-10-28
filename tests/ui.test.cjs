const {test,before,after}=require('node:test');
const assert=require('node:assert/strict');
const {playwright,browserOptions,serveProject}=require('./helpers.cjs');
let browser,server;
before(async()=>{server=await serveProject();browser=await playwright().chromium.launch(browserOptions());});
after(async()=>{await browser?.close();await server?.close();});

// These regressions exercise the actual renderer and archive modules in Chromium.
// The bridge replaces only chrome.runtime's unavailable extension/native transport.
test('rich table cells and list items retain safe ordered nested blocks without duplicate projections',async()=>{
  const page=await browser.newPage();
  try{
    await page.goto(server.url+'/blank');
    const result=await page.evaluate(async()=>{
      const {renderBlocks}=await import('/extension/ui/common.js');
      const target=document.createElement('article');document.body.append(target);
      renderBlocks([
        {type:'table',rows:[['cell text fallback','plain fallback']],cell_blocks:[[[
          {type:'text',text:'cell text'},
          {type:'code',language:'python',text:'  SECRET_CODE\n    Unicode 🦊'},
          {type:'image',src:'https://example.org/diagram.png',alt:'DIAGRAM_LABEL'},
          {type:'table',rows:[['nested cell']]},
          {type:'link',url:'javascript:window.pwned=true',text:'unsafe destination'}
        ],[]]]},
        {type:'list',ordered:true,items:[{text:'item projection must not duplicate',blocks:[
          {type:'text',text:'rich item'},
          {type:'code',text:'ITEM_CODE'},
          {type:'list',items:['inner item']}
        ]},{text:'plain item fallback'},'plain string item']}
      ],target);
      return {text:target.innerText,code:[...target.querySelectorAll('pre code')].map(x=>x.textContent),links:[...target.querySelectorAll('a')].map(x=>x.href),images:target.querySelectorAll('img').length,unsafe:target.querySelectorAll('[href^="javascript:"]').length,pwned:!!window.pwned};
    });
    assert.deepEqual(result.code,['  SECRET_CODE\n    Unicode 🦊','ITEM_CODE']);
    assert.match(result.text,/DIAGRAM_LABEL/);assert.match(result.text,/nested cell/);assert.match(result.text,/plain fallback/);
    assert.match(result.text,/rich item/);assert.match(result.text,/inner item/);assert.match(result.text,/plain item fallback/);assert.match(result.text,/plain string item/);
    assert.doesNotMatch(result.text,/cell text fallback|item projection must not duplicate/);
    assert.deepEqual(result.links,['https://example.org/diagram.png']);assert.equal(result.images,0);assert.equal(result.unsafe,0);assert.equal(result.pwned,false);
  }finally{await page.close();}
});
async function archiveFixture(){
  const page=await browser.newPage();
  await page.addInitScript(()=>{
    window.fixture={total:26,fail:false,pending:[],offsets:[]};
    const rows=(total,offset,limit)=>Array.from({length:Math.max(0,Math.min(limit,total-offset))},(_,i)=>({id:offset+i+1,url:'https://example.org/'+(offset+i+1),title:'Page '+(offset+i+1),hostname:'example.org',capture_count:1,last_seen:'2026-10-27T12:00:00Z'}));
    window.releaseList=()=>{for(const release of fixture.pending.splice(0))release();};
    window.chrome={runtime:{sendMessage:async message=>{
      if(message.type==='status')return{ok:true,data:{pendingCount:0}};
      const r=message.request;
      if(r.type==='stats')return{ok:true,data:{pages:fixture.total,captures:fixture.total,blocks:fixture.total,db_bytes:0}};
      if(r.type==='pages.domains')return{ok:true,data:{items:[]}};
      if(r.type==='pages.list'){
        fixture.offsets.push(r.offset);
        if(fixture.offsets.length>1)await new Promise(resolve=>fixture.pending.push(resolve));
        if(fixture.fail)return{ok:false,error:'Controlled native failure'};
        return{ok:true,data:{items:rows(fixture.total,r.offset,r.limit),total:fixture.total,offset:r.offset,limit:r.limit}};
      }
      throw Error('Unexpected command '+r.type);
    }}};
  });
  await page.goto(server.url+'/extension/ui/archive.html');
  await page.waitForFunction(()=>document.querySelectorAll('.page-item').length===25);
  return page;
}
test('repeated Next clicks keep the last valid page while a list request is in flight',async()=>{
  const page=await archiveFixture();
  try{
    await page.evaluate(()=>{document.querySelector('#next').click();document.querySelector('#next').click();});
    assert.equal(await page.locator('#next').isDisabled(),true);
    await page.evaluate(()=>releaseList());
    await page.waitForFunction(()=>!document.querySelector('#page-list').hasAttribute('aria-busy'));
    assert.equal(await page.locator('.page-item').count(),1);assert.match(await page.locator('#page-range').innerText(),/^26–26 из 26$/);
    assert.deepEqual(await page.evaluate(()=>fixture.offsets),[0,25]);
  }finally{await page.close();}
});
test('a shrinking total recovers from an out-of-range page instead of showing onboarding',async()=>{
  const page=await archiveFixture();
  try{
    await page.locator('#next').click();await page.evaluate(()=>{fixture.total=10;releaseList();});
    await page.waitForFunction(()=>fixture.offsets.length>=3);
    await page.evaluate(()=>releaseList());await page.waitForFunction(()=>!document.querySelector('#page-list').hasAttribute('aria-busy'));
    assert.equal(await page.locator('.page-item').count(),10);assert.match(await page.locator('#page-range').innerText(),/^1–10 из 10$/);
    assert.deepEqual(await page.evaluate(()=>fixture.offsets),[0,25,0]);
    assert.doesNotMatch(await page.locator('#page-list').innerText(),/Пока здесь пусто/);
  }finally{await page.close();}
});
test('failed list requests disable stale pagination and show an explicit unavailable state',async()=>{
  const page=await archiveFixture();
  try{
    await page.locator('#next').click();await page.evaluate(()=>{fixture.fail=true;releaseList();});
    await page.waitForFunction(()=>!document.querySelector('#page-list').hasAttribute('aria-busy'));
    assert.equal(await page.locator('#next').isDisabled(),true);assert.equal(await page.locator('#previous').isDisabled(),true);
    assert.match(await page.locator('#page-range').innerText(),/Недоступно/);assert.match(await page.locator('#message').innerText(),/Controlled native failure/);
    assert.doesNotMatch(await page.locator('#page-list').innerText(),/Пока здесь пусто/);
  }finally{await page.close();}
});
test('chunk reader discards same-length changed revisions and restarts without corrupting Unicode',async()=>{
  const page=await browser.newPage();
  try{
    await page.goto(server.url+'/blank');
    const result=await page.evaluate(async()=>{
      const {readChunked}=await import('/extension/ui/common.js');
      const old=Array.from(JSON.stringify({text:'Старое 🦊 значение'}));
      const fresh=Array.from(JSON.stringify({text:'Новое! 🌍 значение'}));
      const offsets=[];let switched=false;
      window.chrome={runtime:{sendMessage:async m=>{
        const r=m.request;offsets.push(r.offset);
        if(r.offset>0)switched=true;
        const chars=switched?fresh:old;const next=Math.min(r.offset+14,chars.length);
        return{ok:true,data:{offset:r.offset,json_chunk:chars.slice(r.offset,next).join(''),next_offset:next,total_chars:chars.length,done:next===chars.length,document_hash:switched?'new-hash':'old-hash'}};
      }}};
      return{value:await readChunked('pages.export',{page_id:1}),offsets};
    });
    assert.equal(result.value.text,'Новое! 🌍 значение');
    assert.equal(result.offsets.filter(x=>x===0).length,2);
  }finally{await page.close();}
});
test('chunk reader stops after two restarts when the archive keeps changing',async()=>{
  const page=await browser.newPage();
  try{
    await page.goto(server.url+'/blank');
    const result=await page.evaluate(async()=>{
      const {readChunked}=await import('/extension/ui/common.js');
      let calls=0,starts=0;const chars=Array.from('{"text":"changing"}');
      window.chrome={runtime:{sendMessage:async m=>{
        const r=m.request;calls++;if(!r.offset)starts++;
        const next=Math.min(r.offset+14,chars.length);
        return{ok:true,data:{offset:r.offset,json_chunk:chars.slice(r.offset,next).join(''),next_offset:next,total_chars:chars.length,done:next===chars.length,document_hash:'revision-'+calls}};
      }}};
      try{await readChunked('pages.export',{page_id:1});return{success:true,starts};}catch(e){return{error:e.message,starts};}
    });
    assert.match(result.error,/измен/i);assert.equal(result.starts,3);
  }finally{await page.close();}
});
test('bounded page summaries display the URL preview and restore the full safe source link from export',async()=>{
  const page=await browser.newPage();
  try{
    await page.addInitScript(()=>{
      const original={id:1,url:'https://example.org/full-original?state=long',title:'',hostname:'example.org',capture_count:1,last_seen:'2026-10-27T12:00:00Z'};
      const summary={...original,url:'',url_display:'https://example.org/full…',summary_truncated:['url']};
      window.chrome={runtime:{sendMessage:async m=>{
        if(m.type==='status')return{ok:true,data:{pendingCount:0}};
        const r=m.request;let data;
        if(r.type==='stats')data={pages:1,captures:1,blocks:1,db_bytes:0};
        if(r.type==='pages.domains')data={items:[]};
        if(r.type==='pages.list')data={items:[summary],total:1,limit:25,offset:0};
        if(r.type==='pages.get')data={page:summary,captures:[],total:0};
        if(r.type==='pages.export'){
          const json_chunk=JSON.stringify({page:original,blocks:[{type:'text',text:'body'}]});
          data={json_chunk,offset:0,next_offset:json_chunk.length,total_chars:json_chunk.length,done:true,document_hash:'stable'};
        }
        return{ok:true,data};
      }}};
    });
    await page.goto(server.url+'/extension/ui/archive.html');
    await page.locator('.page-item').waitFor();
    assert.equal(await page.locator('.page-item-title').innerText(),'https://example.org/full…');
    await page.locator('.page-item').click();await page.waitForFunction(()=>document.querySelector('#blocks').textContent==='body');
    assert.equal(await page.locator('#detail-link a').getAttribute('href'),'https://example.org/full-original?state=long');
  }finally{await page.close();}
});
