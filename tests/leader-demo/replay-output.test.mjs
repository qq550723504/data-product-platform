import test from 'node:test';import assert from 'node:assert/strict';
import {mkdtemp,readFile,writeFile} from 'node:fs/promises';import {tmpdir} from 'node:os';import {join} from 'node:path';
import {createReplayOutput} from './replay-output.mjs';
test('replay output is unique and refuses to overwrite delivered evidence',async()=>{
 const root=await mkdtemp(join(tmpdir(),'leader-replay-'));
 const first=await createReplayOutput(root),second=await createReplayOutput(root);assert.notEqual(first.href,second.href);
 const delivery=await createReplayOutput(root,'docs/delivery');await writeFile(new URL('acceptance.json',delivery),'original');
 await assert.rejects(createReplayOutput(root,'docs/delivery'),{code:'EEXIST'});
 assert.equal(await readFile(new URL('acceptance.json',delivery),'utf8'),'original');
 await assert.rejects(createReplayOutput(root,'../outside'),/inside this checkout/);
});
