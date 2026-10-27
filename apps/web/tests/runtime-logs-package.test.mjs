import {test} from 'node:test';
import assert from 'node:assert/strict';
import {mkdtempSync,mkdirSync,copyFileSync,rmSync,unlinkSync} from 'node:fs';
import {tmpdir} from 'node:os';
import {resolve,dirname,join} from 'node:path';
import {fileURLToPath} from 'node:url';
import {spawnSync} from 'node:child_process';
test('runtime log unit tests run from a web-only package without repository-root fixtures',()=>{
 const web=fileURLToPath(new URL('../',import.meta.url)),directory=mkdtempSync(join(tmpdir(),'dpp-web-log-package-'));
 try{
  for(const path of ['tests/runtime-logs.test.mjs','tests/helpers/runtime-log-source.mjs','.runtime-log-tests/lib/runtime-logs.js']){const target=join(directory,path);mkdirSync(dirname(target),{recursive:true});copyFileSync(join(web,path),target)}
  const env={...process.env};delete env.NODE_TEST_CONTEXT;
  const result=spawnSync(process.execPath,['--test','tests/runtime-logs.test.mjs'],{cwd:directory,encoding:'utf8',timeout:15000,env});
  assert.equal(result.status,0,result.stdout+'\n'+result.stderr);
  unlinkSync(join(directory,'tests/helpers/runtime-log-source.mjs'));
  const missing=spawnSync(process.execPath,['--test','tests/runtime-logs.test.mjs'],{cwd:directory,encoding:'utf8',timeout:15000,env});
  assert.notEqual(missing.status,0,'missing packaged fixture must fail in the independent child runner');
 }finally{if(dirname(resolve(directory))!==resolve(tmpdir())||!directory.split(/[\\/]/).at(-1).startsWith('dpp-web-log-package-'))throw Error('unsafe cleanup target');rmSync(directory,{recursive:true,force:true})}
});
