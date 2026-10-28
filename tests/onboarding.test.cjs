const {test,before,after}=require('node:test');
const assert=require('node:assert/strict');
const {playwright,browserOptions,serveProject}=require('./helpers.cjs');
let browser,server;
before(async()=>{server=await serveProject();browser=await playwright().chromium.launch(browserOptions());});
after(async()=>{await browser?.close();await server?.close();});

async function fixture(options={}) {
  const page=await browser.newPage();
  page.setDefaultTimeout(3000);
  await page.addInitScript(options=>{
    window.fixture={host:{connected:false,error:'Native host unavailable'},reject:false,pending:false,calls:0,resolvers:[],...options};
    window.releaseChecks=()=>{for(const resolve of fixture.resolvers.splice(0))resolve();};
    window.chrome={runtime:{id:'ijkioljfdaaplhaldbdpabckkfcieiee',getManifest:()=>({version:'1.3.0'}),getPlatformInfo:async()=>{
      if(fixture.platformFailure)throw Error('platform lookup failed');
      return {os:'mac'};
    },sendMessage:async message=>{
      if(message.type!=='status')throw Error('Unexpected message '+message.type);
      fixture.calls++;
      if(fixture.pending)await new Promise(resolve=>fixture.resolvers.push(resolve));
      if(fixture.reject)throw Error('Controlled status failure');
      return {ok:true,data:{host:fixture.host,pendingCount:3,queueBytes:256,enabledOrigins:[],settings:{saveHtml:true,captureIntervalSeconds:30,debounceMs:1200,maxBlocks:5000,maxTextChars:1000000,maxHtmlChars:2000000}}};
    }}};
  },options);
  await page.goto(server.url+'/extension/ui/options.html');
  await page.waitForFunction(()=>!document.querySelector('#settings-fields').disabled);
  return page;
}

test('unavailable helper shows install flow, actual extension ID, download page and macOS steps',async()=>{
  const page=await fixture();
  try {
    assert.equal(await page.locator('#open-setup').isVisible(),true);
    assert.equal(await page.locator('#host-version').innerText(),'Неизвестна');
    assert.match(await page.locator('#host-error').innerText(),/Native host unavailable/);
    assert.equal(await page.locator('#extension-id').innerText(),'ijkioljfdaaplhaldbdpabckkfcieiee');
    assert.equal(await page.locator('#download-link').getAttribute('href'),'https://github.com/akarazhev/pagevault/releases');
    assert.equal(await page.locator('#download-link').getAttribute('target'),'_blank');
    assert.match(await page.locator('#download-link').getAttribute('rel'),/noopener/);
    await page.waitForFunction(()=>document.querySelector('#download-platform').value==='mac');
    assert.match(await page.locator('#platform-detail').innerText(),/\.pkg/);
    assert.equal(await page.locator('#platform-command').isVisible(),false);
    assert.match(await page.locator('#queue-status').innerText(),/3 снимков/);
  } finally {await page.close();}
});

test('a shared connection check disables both buttons, recovers and preserves unsaved settings',async()=>{
  const page=await fixture();
  try {
    await page.locator('#captureIntervalSeconds').fill('77');
    await page.evaluate(()=>{fixture.pending=true;document.querySelector('#check-connection').click();document.querySelector('#setup-check-connection').click();});
    assert.equal(await page.locator('#check-connection').isDisabled(),true);
    assert.equal(await page.locator('#setup-check-connection').isDisabled(),true);
    assert.equal(await page.evaluate(()=>fixture.calls),2);
    await page.evaluate(()=>{fixture.host={connected:true,version:'1.2.0',schema_version:1,db_path:'/tmp/сохранённый архив.sqlite3'};fixture.pending=false;releaseChecks();});
    await page.waitForFunction(()=>!document.querySelector('#setup-check-connection').disabled);
    assert.match(await page.locator('#host-status').innerText(),/SQLite подключена/);
    assert.equal(await page.locator('#host-version').innerText(),'1.2.0');
    assert.equal(await page.locator('#db-path').innerText(),'/tmp/сохранённый архив.sqlite3');
    assert.equal(await page.locator('#captureIntervalSeconds').inputValue(),'77');
    assert.match(await page.locator('#connection-guidance').innerText(),/обнов/i);
    await page.evaluate(()=>{fixture.reject=true;});
    await page.locator('#setup-check-connection').click();
    await page.waitForFunction(()=>!document.querySelector('#setup-check-connection').disabled);
    assert.match(await page.locator('#host-error').innerText(),/Controlled status failure/);
    assert.doesNotMatch(await page.locator('#db-path').innerText(),/сохранённый/);
    await page.evaluate(()=>{fixture.reject=false;fixture.host={connected:true,version:'1.3.0',schema_version:1,db_path:'/restored.sqlite3'};});
    await page.locator('#setup-check-connection').click();
    await page.waitForFunction(()=>document.querySelector('#db-path').textContent==='/restored.sqlite3');
    assert.equal(await page.locator('#open-setup').isVisible(),false);
  } finally {await page.close();}
});

test('unsupported schema and untrusted helper values render safely',async()=>{
  const text='<img src=x onerror="window.pwned=true">';
  const page=await fixture({host:{connected:true,version:text,schema_version:2,db_path:text,error:text}});
  try {
    assert.match(await page.locator('#host-status').innerText(),/несовместим/i);
    assert.equal(await page.locator('#host-dot').evaluate(n=>n.classList.contains('online')),false);
    assert.equal(await page.locator('#open-setup').isVisible(),true);
    assert.equal(await page.locator('#host-version').innerText(),text);
    assert.equal(await page.locator('#db-path').innerText(),text);
    assert.equal(await page.locator('img').count(),0);
    assert.equal(await page.evaluate(()=>!!window.pwned),false);
  } finally {await page.close();}
});

test('platform failure leaves a manual selector and Windows/Linux instructions',async()=>{
  const page=await fixture({platformFailure:true});
  try {
    assert.equal(await page.locator('#download-platform').inputValue(),'unknown');
    await page.locator('#download-platform').selectOption('win');
    assert.match(await page.locator('#platform-command').innerText(),/install-windows/);
    await page.locator('#download-platform').selectOption('linux');
    assert.match(await page.locator('#platform-command').innerText(),/bash install.sh chrome/);
    await page.locator('#download-platform').selectOption('mac');
    assert.equal(await page.locator('#platform-command').isVisible(),false);
  } finally {await page.close();}
});
