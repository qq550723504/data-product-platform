// Local exact-Next smoke with Edge. The existing browser CI runs this same case.
import {defineConfig} from '../browser/node_modules/@playwright/test/index.mjs';
import base from '../browser/playwright.config.mjs';
import {fileURLToPath} from 'node:url';
export default defineConfig({...base,webServer:base.webServer.map(server=>({...server,cwd:server.cwd??fileURLToPath(new URL('../browser/',import.meta.url))})),testDir:fileURLToPath(new URL('../browser/',import.meta.url)),testMatch:'management-readonly.spec.mjs',
 outputDir:'../../.data/runtime-logs-20261026/default-next-traces',reporter:[['list'],['json',{outputFile:'../../.data/runtime-logs-20261026/default-next-results.json'}]],
 use:{...base.use,channel:'msedge'}});
