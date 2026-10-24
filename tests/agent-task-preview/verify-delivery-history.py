"""Read-only DB evidence for the API/browser acceptance window."""
import importlib.util,json
from pathlib import Path
spec=importlib.util.spec_from_file_location('readonly_prepare',Path(__file__).with_name('prepare-readonly.py'))
p=importlib.util.module_from_spec(spec);spec.loader.exec_module(p)
original=json.loads((p.RUN/'delivery-original-before.json').read_text())
prepared=json.loads((p.RUN/'delivery-readonly-before.json').read_text())
result={'originalUnchanged':p.fingerprint('agent_q2_test')==original,'acceptanceReadWindowUnchanged':p.fingerprint(p.DATABASE)==prepared}
result['counts']=json.loads(p.sql(p.DATABASE,"""BEGIN READ ONLY;
 SELECT json_build_object('executions',(SELECT count(*) FROM execution),'agentTasks',(SELECT count(*) FROM agent_task),'actionRequests',(SELECT count(*) FROM agent_action_request),'providerAttempts',(SELECT count(*) FROM delivery_provider_attempt),'outbox',(SELECT count(*) FROM outbox_event),'syntheticBlockedOperations',(SELECT count(*) FROM delivery_operation WHERE purpose='SYNTHETIC_HISTORY' AND status='BLOCKED'));
 ROLLBACK;"""))
result['privileges']=json.loads(p.sql(p.DATABASE,"""BEGIN READ ONLY;
 SELECT json_build_object('writeTables',(SELECT count(*) FROM information_schema.role_table_grants WHERE grantee='agent_q2_reader_20261024' AND privilege_type<>'SELECT'),'writeColumns',(SELECT count(*) FROM information_schema.role_column_grants WHERE grantee='agent_q2_reader_20261024' AND privilege_type<>'SELECT'),'schemaCreate',has_schema_privilege('agent_q2_reader_20261024','public','CREATE'),'temp',has_database_privilege('agent_q2_reader_20261024','agent_q2_acceptance_20261024','TEMP'));
 ROLLBACK;"""))
assert result['originalUnchanged'] and result['acceptanceReadWindowUnchanged'], 'database fingerprint changed'
assert result['counts']=={'executions':0,'agentTasks':0,'actionRequests':0,'providerAttempts':0,'outbox':0,'syntheticBlockedOperations':1},'unexpected activity'
assert result['privileges']=={'writeTables':0,'writeColumns':0,'schemaCreate':False,'temp':False},'unexpected privileges'
(p.ROOT/'.data/governance-delivery-no-write-proof.json').write_text(json.dumps(result,indent=2))
print(json.dumps(result))
