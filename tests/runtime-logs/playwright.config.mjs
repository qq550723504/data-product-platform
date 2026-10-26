import {defineConfig} from '../browser/node_modules/@playwright/test/index.mjs';
const channel=process.env.RUNTIME_LOG_BROWSER_CHANNEL??(process.platform==='win32'?'msedge':'chromium');
if(!['msedge','chromium'].includes(channel))throw Error('explicit browser required');
export default defineConfig({testDir:'.',testMatch:'browser.spec.mjs',workers:1,fullyParallel:false,retries:0,forbidOnly:!!process.env.CI,timeout:30000,expect:{timeout:7000},
 reporter:[['list'],['json',{outputFile:'../../.data/runtime-logs-20261026/browser-results.json'}]],
 outputDir:'../../.data/runtime-logs-20261026/traces',use:{browserName:'chromium',...(channel==='msedge'?{channel}:{}),ignoreHTTPSErrors:true,serviceWorkers:'block',viewport:{width:1600,height:950},screenshot:'only-on-failure',trace:'retain-on-failure'}});
