import {defineConfig} from '@playwright/test';
process.env.SETTINGS_INTERACTION_EVIDENCE_RUN??=new Date().toISOString().replaceAll(':','-');
const evidence='../../.data/settings-interaction-browser/run-'+process.env.SETTINGS_INTERACTION_EVIDENCE_RUN;
export default defineConfig({
 testDir:'.',testMatch:'settings-interaction.spec.mjs',workers:1,retries:0,timeout:30000,
 expect:{timeout:8000},reporter:[['list'],['json',{outputFile:evidence+'/results.json'}]],
 outputDir:evidence+'/artifacts',
 use:{browserName:'chromium',channel:process.env.SETTINGS_INTERACTION_BROWSER_CHANNEL??'msedge',baseURL:'http://127.0.0.1:4445',viewport:{width:1600,height:1000},trace:'retain-on-failure',screenshot:'only-on-failure'},
});
