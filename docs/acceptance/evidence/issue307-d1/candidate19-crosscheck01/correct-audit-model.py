"""Preserve the first diagnostic; apply the existing18-history/17-present contract."""
import json
from pathlib import Path

HERE=Path(__file__).parent
NEXT=HERE.with_name('candidate19-crosscheck02');NEXT.mkdir(exist_ok=False)
source=(HERE/'crosscheck.py').read_text(encoding='utf-8')
old="        need(set(seen)=={cp['ID'] for cp in group['Copies']},'Stored native membership differs')\n        for cp in group['Copies']:\n"
new="""        present=[cp for cp in group['Copies'] if cp['Key']!='fixture/p1.csv']
        historical_p1=[cp for cp in group['Copies'] if cp['Key']=='fixture/p1.csv']
        if historical_p1:
            need(group['Group']=='business-races' and len(historical_p1)==1 and native['p1_absence']['copy']==historical_p1[0]
                 and native['p1_absence']['status']==404 and native['p1_absence']['code']=='NoSuchVersion','Historical P1 absence differs')
        else:need(native['p1_absence'] is None,'Unexpected P1 absence scope')
        need(set(seen)=={cp['ID'] for cp in present},'Stored17-present native membership differs')
        for cp in present:
"""
if source.count(old)!=1:raise ValueError('Unexpected diagnostic membership contract')
source=source.replace(old,new)
old="        readonly_private=next(p for p in PRIVATE.glob('complete-*/'+group['Group']) if sha(p/'six-readonly.json')==load(folder/'allocated-owner.json')['profile_sha256'])"
new="""        expected_audit=load(folder/'expectation.json')['audit_id']
        readonly_private=next(p for p in PRIVATE.glob('complete-*/'+group['Group']) if load(p/'six-readonly.json')['AuditID']==expected_audit)
        need(load(readonly_private/'six-readonly.json')['Enabled'] is False,'Completed audit profile not revoked')"""
if source.count(old)!=1:raise ValueError('Unexpected historical read-profile selector')
source=source.replace(old,new).replace("'targets':6,'survivors':11,'target_stage_order'","'historical_copies':18,'targets':6,'survivors':11,'target_stage_order'")
for name in ('prior-display.md','prior-display-source.json'):
    with (NEXT/name).open('xb') as f:f.write((HERE/name).read_bytes())
with (NEXT/'crosscheck.py').open('x',encoding='utf-8',newline='\n') as f:f.write(source)
with (NEXT/'diagnostic-correction.json').open('x',encoding='utf-8',newline='\n') as f:
    json.dump({'original_failure':'candidate19-crosscheck01/failure.json','classification':'AUDIT_MODEL_ERROR_NOT_RUNTIME_OR_CANDIDATE_DRIFT',
        'historical_copies':18,'present_versions':17,'historical_P1':'Retain exact identity and saved NoSuchVersion404; exclude from present/targets/budgets',
        'completed_read_profile':'Locate by immutable AuditID; current Enabled=false is the legitimate finally revocation, not an unchanged original enabled-profile hash',
        'candidate_source_binary_config_changed':False,'gate_relaxed':False,'source_start':0,'native_delete':0},f,indent=2)
print('Corrected only the offline diagnostic model; original failure retained.')
