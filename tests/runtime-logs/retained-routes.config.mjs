import {defineConfig} from '../browser/node_modules/@playwright/test/index.mjs';
import base from './default-preview.config.mjs';
export default defineConfig({...base,testMatch:['modern-core.spec.mjs','business-acceptance.spec.mjs','business-acceptance-exceptions.spec.mjs'],
 outputDir:'../../.data/runtime-logs-20261026/retained-traces',reporter:[['list'],['json',{outputFile:'../../.data/runtime-logs-20261026/retained-results.json'}]]});
