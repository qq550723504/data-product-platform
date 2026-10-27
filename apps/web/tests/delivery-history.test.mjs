import test from 'node:test';
import assert from 'node:assert/strict';
import {createRequire} from 'node:module';
const require=createRequire(import.meta.url);
const {validHistoricalPage,deliverySelection}=require('../.history-tests/delivery-history.js');
const ws='11111111-1111-4111-8111-111111111111',id='40404040-4040-4040-8040-404040404040';
const row={id,workspaceId:ws,datasetVersionId:id,status:'ISSUED',gateDecision:'ALLOWED',purpose:'TEST',action:'READ',channel:'API',mode:'DIRECT_DATA',createdAt:'2026-10-24T00:00:00Z',evaluations:[],transitions:[]};
const page={workspaceId:ws,items:[row],page:{limit:25,offset:0,total:1}};
test('delivery history validates scope, exact record, non-null historical arrays and pagination',()=>{
 assert.equal(validHistoricalPage(page,ws,false,id,0),true);
 for(const changed of [{workspaceId:id},{id:ws},{evaluations:null},{status:'DELIVERED'},{transitions:[{id,from:'ISSUED',to:'ACCEPTED',createdAt:row.createdAt}]}])assert.equal(validHistoricalPage({...page,items:[{...row,...changed}]},ws,false,id,0),false);
 assert.equal(validHistoricalPage(page,ws,false,id,25),false);
 assert.equal(validHistoricalPage({...page,items:[],page:{limit:25,offset:25,total:1}},ws,false,undefined,25),true);
});
test('archive facts are not acceptance outcomes; malformed hash and foreign release references fail closed',()=>{
 const archive={id,workspaceId:ws,objectType:'PRODUCT_RELEASE',objectId:id,rootHash:'a'.repeat(64),createdAt:row.createdAt,itemCount:0,releaseIds:[id]};
 assert.equal(validHistoricalPage({...page,items:[archive]},ws,true,id,0),true);
 for(const patch of [{rootHash:''},{releaseIds:['REL-1']},{itemCount:-1}])assert.equal(validHistoricalPage({...page,items:[{...archive,...patch}]},ws,true,id,0),false);
});
test('invalid and repeated references never select latest',()=>{
 for(const q of [{id:''},{id:'DEL-1'},{id:[id,id]},{offset:'-1'},{offset:'1.5'},{status:'ISSUED'},{id:'00000000-0000-0000-0000-000000000000'}])assert.throws(()=>deliverySelection(q));
 assert.deepEqual(deliverySelection({id,offset:'25'}),{id,offset:25});
});
