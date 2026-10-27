import test from 'node:test';
import assert from 'node:assert/strict';
import {businessLabel,businessText,referenceLabel,displayText} from '../../lib/business-language.mjs';
test('authorization release, receipt and acceptance retain separate meanings',()=>{
 assert.equal(businessLabel('ISSUED'),'授权已释放');
 assert.doesNotMatch(businessLabel('ISSUED'),/收到|验收通过/);
 assert.equal(businessLabel('UNKNOWN'),'无法确认');assert.equal(businessLabel('CONTAINMENT_PENDING'),'访问能力待收回');
 assert.equal(businessLabel('MATCH'),'结构一致');assert.doesNotMatch(businessLabel('MATCH'),/来源正常|健康/);
 assert.equal(businessText('UNAVAILABLE 35 条 · INTERRUPTED 35 条'),'暂不可用 35 条 · 已中断 35 条');
});
test('reference labels preserve distinct historical identities and leave actual inputs unchanged',()=>{
 const input={id:'DEMO-SUB-2',versionId:'DEMO-VER-01',code:'future-status'};const original=JSON.stringify(input);
 assert.equal(referenceLabel(input.id),'验收提交 2');assert.notEqual(referenceLabel('DEMO-SUB-1'),referenceLabel(input.id));
 assert.equal(referenceLabel('cccccccc-cccc-4ccc-8ccc-cccccccccccc'),'cccccccc-cccc-4ccc-8ccc-cccccccccccc');
 assert.equal(businessLabel(input.code),input.code);assert.equal(JSON.stringify(input),original);
});
test('unfamiliar values and human evidence are not converted into successful outcomes',()=>{
 assert.equal(businessLabel('UNRECOGNIZED_FAILURE'),'UNRECOGNIZED_FAILURE');
 assert.equal(businessText('DEMO-LEADER-COMPLETE'),'DEMO-LEADER-COMPLETE');
 assert.equal(businessText('STORAGE-UNAVAILABLE'),'STORAGE-UNAVAILABLE');
 assert.equal(businessText('材料 STORAGE-UNAVAILABLE，状态 UNKNOWN'),'材料 STORAGE-UNAVAILABLE，状态 无法确认');
 assert.equal(businessText('引用 Workbench-001；类型 Workbench'),'引用 Workbench-001；类型 业务工作台');
 assert.equal(businessText('gold/schema / 1'),'gold/schema / 1');
 assert.equal(businessText('V2 · REPAIR'),'V2 · 修复方案');assert.equal(businessText('演示记录已更新：签收登记'),'当前记录已更新：签收登记');
});
test('presentation keeps elements, numeric counts and original arrays intact',()=>{
 const element={type:'span',props:{children:'RAW'}},values=['RAW',element,0];
 assert.equal(displayText(element),element);assert.equal(displayText(0),0);
 assert.deepEqual(displayText(values),['原始数据',element,0]);assert.equal(values[0],'RAW');
});
