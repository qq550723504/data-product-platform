"""One-time synthetic BLOCKED history in the independent acceptance DB only.
No delivery command, provider, credential, model or execution is invoked.
"""
import importlib.util, json, os
from pathlib import Path
spec=importlib.util.spec_from_file_location('readonly_prepare',Path(__file__).with_name('prepare-readonly.py'))
p=importlib.util.module_from_spec(spec);spec.loader.exec_module(p)
def main():
 if os.environ.get('READONLY_DELIVERY_FIXTURE_ACK')!='SYNTHETIC_BLOCKED_ONLY':raise RuntimeError('explicit synthetic fixture acknowledgement required')
 profile=json.loads((p.RUN/'readonly-profile.json').read_text())
 if profile['database']!=p.DATABASE or profile['role']!=p.ROLE:raise RuntimeError('independent profile mismatch')
 before=p.fingerprint('agent_q2_test')
 (p.RUN/'delivery-original-before.json').write_text(json.dumps(before))
 if p.sql(p.DATABASE,"SELECT count(*) FROM delivery_operation WHERE id='40404040-4040-4040-8040-404040404040'")!='0':raise RuntimeError('fixture already exists; never reset it')
 p.sql(p.DATABASE,"""BEGIN;
 INSERT INTO delivery_operation(id,workspace_id,dataset_version_id,idempotency_key,provider_name,provider_request_key,status,current_gate_decision,principal_ref,effective_consumer_ref,purpose,action,scope_ref,delivery_channel,delivery_mode,requested_expires_at,terminal_reason)
 VALUES('40404040-4040-4040-8040-404040404040','11111111-1111-4111-8111-111111111111','bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb','synthetic-readonly-history','SYNTHETIC_NO_PROVIDER','synthetic-no-capability','BLOCKED','BLOCKED','SYNTHETIC_ACCEPTANCE','SYNTHETIC_ACCEPTANCE','SYNTHETIC_HISTORY','READ','SYNTHETIC_ONLY','API','DIRECT_DATA',now()+interval '1 hour','SYNTHETIC_ACCEPTANCE_FIXTURE');
 INSERT INTO delivery_gate_evaluation(id,delivery_operation_id,evaluation_key,stage,decision,blockers,dependency_revision,principal_ref,effective_consumer_ref)
 VALUES('50505050-5050-4050-8050-505050505050','40404040-4040-4040-8040-404040404040','synthetic-gate','INITIAL','BLOCKED',ARRAY['SYNTHETIC_NO_DELIVERY'],0,'SYNTHETIC_ACCEPTANCE','SYNTHETIC_ACCEPTANCE');
 INSERT INTO delivery_transition(id,delivery_operation_id,transition_key,from_status,to_status,gate_evaluation_id,reason)
 VALUES('60606060-6060-4060-8060-606060606060','40404040-4040-4040-8040-404040404040','synthetic-transition','PREPARED','BLOCKED','50505050-5050-4050-8050-505050505050','SYNTHETIC_ACCEPTANCE_FIXTURE');
 GRANT SELECT ON delivery_operation,delivery_gate_evaluation,delivery_transition TO agent_q2_reader_20261024;
 COMMIT;""")
 if p.fingerprint('agent_q2_test')!=before:raise RuntimeError('original fingerprint changed')
 (p.RUN/'delivery-readonly-before.json').write_text(json.dumps(p.fingerprint(p.DATABASE)))
 print('Synthetic BLOCKED history prepared; original unchanged; no provider invoked.')
if __name__=='__main__':main()
