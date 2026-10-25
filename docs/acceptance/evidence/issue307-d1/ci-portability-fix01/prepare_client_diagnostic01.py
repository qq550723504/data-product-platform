"""Private Next instrumentation image; never shipped or counted as acceptance."""
from pathlib import Path
root=Path(__file__).resolve().parents[5];out=Path(__file__).parent
context=Path.home()/'.codex/fixture-private/ci-portability-fix01/client-diagnostic01';context.mkdir(exist_ok=False)
patch=r'''const fs=require('node:fs');
function edit(name,before,after){const p='/work/apps/web/node_modules/next/dist/'+name;const s=fs.readFileSync(p,'utf8');if(!s.includes(before))throw new Error('diagnostic insertion missing '+name);fs.writeFileSync(p,s.replace(before,after));}
const log=(phase,body)=>`globalThis.__dppNavDiag?.push({at:performance.now(),phase:'${phase}',${body}});`;
for(const prefix of ['','esm/']){
const q=prefix+'client/components/app-router-instance.js';
edit(q,'const prevState = actionQueue.state;',log('run','kind:action.payload.type,target:action.payload.url?.href,canonical:actionQueue.state.canonicalUrl')+'const prevState = actionQueue.state;');
edit(q,'function handleResult(nextState) {','function handleResult(nextState) {'+log('resolved','kind:action.payload.type,target:action.payload.url?.href,canonical:nextState.canonicalUrl,discarded:action.discarded===true,tree:nextState.tree'));
edit(q,'action.reject(err);',log('rejected','kind:action.payload.type,message:String(err)')+'action.reject(err);');
edit(q,'function dispatchAction(actionQueue, payload, setState) {','function dispatchAction(actionQueue, payload, setState) {'+log('dispatch','kind:payload.type,target:payload.url?.href,canonical:actionQueue.state.canonicalUrl'));
const a=prefix+'client/components/app-router.js';
edit(a,'let { appRouterState } = param;','let { appRouterState } = param;'+log('history-render','canonical:appRouterState.canonicalUrl'));
edit(a,'const { tree, pushRef, canonicalUrl } = appRouterState;','const { tree, pushRef, canonicalUrl } = appRouterState;'+log('history-commit','canonical:canonicalUrl,pendingPush:pushRef.pendingPush'));
}
'''
(context/'patch.cjs').write_text(patch,encoding='utf-8')
spec=(root/'tests/browser/schema-attempts.spec.mjs').read_text(encoding='utf-8')
hook=r'''
test.beforeEach(async({page})=>{
  await page.addInitScript(()=>{window.__dppNavDiag=[];for(const method of ['pushState','replaceState']){const original=history[method];history[method]=function(state,unused,url){window.__dppNavDiag.push({at:performance.now(),phase:method,url:String(url),tree:state?.__PRIVATE_NEXTJS_INTERNALS_TREE});return original.apply(this,arguments);};}});
});
test.afterEach(async({page},info)=>{const value=await page.evaluate(()=>({url:location.href,history:history.state,actions:window.__dppNavDiag}));await info.attach('client-nav-diagnostic',{body:JSON.stringify(value,null,2),contentType:'application/json'});});
'''
(context/'schema-attempts.spec.mjs').write_text(spec+hook,encoding='utf-8')
(context/'Dockerfile').write_text('''FROM dpp307-ci-browser:portability03
USER root
COPY patch.cjs /diagnostics/patch.cjs
COPY --chown=10001:10001 schema-attempts.spec.mjs /work/tests/browser/schema-attempts.spec.mjs
RUN node /diagnostics/patch.cjs && cd /work/apps/web && node node_modules/next/dist/bin/next build && chown -R 10001:10001 /work
USER 10001:10001
CMD ["sh","-ec","cd /work/tests/browser; node node_modules/playwright/cli.js test schema-attempts.spec.mjs -g compact --repeat-each=8 --retries=0 --output=/out/test-results"]
''',encoding='utf-8')
print(context)
