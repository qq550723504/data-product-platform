// Process-only ephemeral TLS, using the same pattern as the existing RP tests.
import {execFileSync,spawnSync} from 'node:child_process';
import {mkdtempSync,rmSync} from 'node:fs';
import {tmpdir} from 'node:os';
import {resolve,dirname,join} from 'node:path';
import {fileURLToPath} from 'node:url';
const directory=mkdtempSync(join(tmpdir(),'dpp-runtime-logs-')),root=fileURLToPath(new URL('../../',import.meta.url));
try{
 const cert=join(directory,'certificate.pem'),key=join(directory,'private-key.pem');
 execFileSync('openssl',['req','-x509','-newkey','rsa:2048','-nodes','-days','1','-keyout',key,'-out',cert,'-subj','/CN=localhost','-addext','subjectAltName=DNS:localhost,IP:127.0.0.1'],{stdio:'ignore'});
 const result=spawnSync(process.execPath,[resolve(root,'tests/browser/node_modules/@playwright/test/cli.js'),'test','--config',resolve(root,'tests/runtime-logs/playwright.config.mjs')],{cwd:resolve(root,'tests/runtime-logs'),stdio:'inherit',env:{...process.env,NODE_EXTRA_CA_CERTS:cert,IAM_FIXTURE_CERT:cert,IAM_FIXTURE_KEY:key}});process.exitCode=result.status??1;
}finally{if(dirname(resolve(directory))!==resolve(tmpdir())||!directory.split(/[\\/]/).at(-1).startsWith('dpp-runtime-logs-'))throw Error('unsafe cleanup target');rmSync(directory,{recursive:true,force:true})}
