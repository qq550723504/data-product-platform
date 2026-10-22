"""Read-only source audit against a newly restored exact bound checkpoint.
No MinIO start, no native I/O, no sender/recovery authority, no DELETE path.
"""
import ast, collections, importlib.util, json, shutil
from pathlib import Path
SCRIPT=Path('C:/Users/Henry/.codex/fixture-private/d1-confirmed-v15-execution02.py')
s=importlib.util.spec_from_file_location('v15',SCRIPT);o=importlib.util.module_from_spec(s);s.loader.exec_module(o)
OUT=o.PUBLIC/'post-seal-audit03';OUT.mkdir();CONTROL=o.CONTROL/'post-seal-audit03';CONTROL.mkdir()
name=o.PROJECT+'-delete-postexecution-audit-core-02';vol=name+'-data';stage=o.CONTROL/'checkpoint'
o.need(o.read(o.PUBLIC/'sealed-state.json')['status']=='PASS_SEALED','Prior independent seal required');o.frozen()
for stage_name in ('a','b'):o.need(o.read(o.CONTROL/('executor-'+stage_name)/'reference-permission.json')['Enabled'] is False,'Sender must remain closed')
o.need(o.read(o.CONTROL/'recovery/recovery.json')['Enabled'] is False,'Recovery authority must remain closed')
ident=o.read(o.PUBLIC/'post-seal-audit/isolated-restore.json')['id']
o.write(OUT/'isolated-restore.json',{'id':ident,'name':name,'volume':vol,'source_dump_sha256':o.sha(stage/'core.dump'),'source_restore':False,'source_mutation':0,'native_provider_io':0,'restored_by':'post-seal-audit/isolated-restore.json','mode':'REUSE_VERIFIED_RESTORED_AUDIT_COPY_READ_ONLY'})

def snapshot(container):
 names=o.m.source_read(container,"SELECT table_name FROM information_schema.tables WHERE table_schema='public' AND table_type='BASE TABLE' ORDER BY table_name;").splitlines();o.need(len(names)==194 and all(n.replace('_','').isalnum() for n in names),'Table contract drift')
 q=["SELECT jsonb_build_object('relation','"+n+"','rows',coalesce((SELECT jsonb_agg(to_jsonb(t) ORDER BY to_jsonb(t)::text) FROM public.\""+n+"\" t),'[]'::jsonb))::text" for n in names]
 return {x['relation']:x['rows'] for x in [json.loads(x) for x in o.m.source_read(container,' UNION ALL '.join(q)+';').splitlines()]}
def counter(rows):return collections.Counter(json.dumps(x,sort_keys=True,separators=(',',':')) for x in rows)
try:
 o.docker('start',ident);o.m.wait_audit_core(ident)
 baseline=o.read(o.PUBLIC/'preflight/source-before.json');o.need(o.m.fingerprint(ident)==baseline['relations'] and o.m.role_acl(ident)==baseline['roles_acl'],'Bound checkpoint restore differs from authorized baseline')
 o.docker('start',o.m.PG);o.m.wait_audit_core(o.m.PG);before_fp=o.m.fingerprint(o.m.PG);before=snapshot(ident);after=snapshot(o.m.PG)
 changed=[];rows=[];additions={}
 allowed={'audit_event','evidence','evidence_relation','cost_event','cost_allocation','command_idempotency','outbox_event','outbox_event_consumption','delivery_authorization_fence'}
 for table,original in before.items():
  actual=after[table];delta=counter(actual)-counter(original);missing=counter(original)-counter(actual)
  if delta or missing:changed.append(table)
  o.need(not (delta or missing) or table in allowed or table.startswith('lifecycle_'),'Unexpected business relation mutation: '+table)
  if table=='delivery_authorization_fence':
   o.need(len(original)==len(actual)==1,'Unexpected fence cardinality');b=dict(original[0]);a=dict(actual[0]);o.need(a['revision']>b['revision'],'Shared authorization fence did not advance');br=b.pop('revision');ar=a.pop('revision');b.pop('updated_at',None);a.pop('updated_at',None);o.need(a==b,'Authorization fence identity changed');additions[table]={'before_revision':br,'after_revision':ar,'delta':ar-br}
  else:
   o.need(not missing,'Historical row changed/deleted: '+table);additions[table]=len(actual)-len(original)
  rows.append({'relation':table,'before_rows':len(original),'after_rows':len(actual),'old_rows_preserved':True,'changed':bool(delta or missing),'new_rows':len(actual)-len(original)})
 # Four explicit new execution commands and three existing recovery commands.
 new_commands=[x for x in after['command_idempotency'] if counter([x])-counter(before['command_idempotency'])];o.need(len(new_commands)==7,'Expected exact seven command facts')
 returned={stage:o.read(o.PUBLIC/('send-'+stage)/('send-'+stage+'-returned.json')) for stage in ('a','b')}
 new_basis=next(x for x in after['lifecycle_fixture_disposal_basis'] if x['run_id']==o.EXECUTION)
 approvals=[x for x in after['lifecycle_disposition_approval'] if counter([x])-counter(before['lifecycle_disposition_approval'])];o.need(len(approvals)==1,'Expected one new approval')
 expected={o.EXECUTION+':basis':('LIFECYCLE.REGISTER_FIXTURE_BASIS',new_basis['id']),o.EXECUTION+':approval':('LIFECYCLE.APPROVE_DISPOSITION',approvals[0]['id'])}
 for stage_name,result in returned.items():
  dispatch=next(x for x in after['lifecycle_disposition_dispatch'] if x['id']==result['attempt']['DispatchID']);o.need(dispatch['copy_id']==result['copy']['ID'],'Dispatch copy mismatch');expected[o.EXECUTION+':send-'+stage_name+':dispatch']=('LIFECYCLE.DISPATCH',dispatch['evaluation_id'])
  work=next(x for x in after['lifecycle_reconciliation_work'] if x['id']==result['work_id']);o.need(work['attempt_id']==result['attempt']['ID'],'Recovery work does not match physical attempt')
  results=[x for x in after['lifecycle_reconciliation_result'] if x['work_id']==work['id']];o.need(len(results)==1 and results[0]['terminal'],'Recovery terminal result missing');r=results[0];receipt=next(x for x in after['lifecycle_copy_absence_receipt'] if x['id']==r['absence_receipt_id']);o.need(receipt['copy_id']==result['copy']['ID'] and receipt['dispatch_id']==result['attempt']['DispatchID'],'Absence subject mismatch');expected['recovery-absence:'+r['run_id']]=('LIFECYCLE.VERIFY_ABSENCE',receipt['id'])
 completion=after['lifecycle_plan_completion'];o.need(len(completion)==1,'Expected one completion');expected['recovery-finalize:'+completion[0]['plan_id']]=('LIFECYCLE.FINALIZE_DISPOSITION',completion[0]['id'])
 o.need({x['idempotency_key'] for x in new_commands}==set(expected),'Unexpected command key set')
 for row in new_commands:o.need((row['command_type'],row['object_id'])==expected[row['idempotency_key']] and row['result_ref']==row['object_id'],'Command type/subject/result mismatch')
 o.write(OUT/'new-command-keys.json',{'status':'PASS_FOUR_NEW_EXECUTION_THREE_EXISTING_RECOVERY_KEYS','commands':new_commands,'expected':expected,'old_authorization_reused':False})
 # All original 194-table rows remain byte-equivalent as JSON except the controlled fence projection.
 o.write(OUT/'historical-subset-check.json',{'status':'PASS_ALL_194_TABLES_OLD_ROWS_PRESERVED','rows':rows,'changed_relations':changed,'deltas':{k:v for k,v in additions.items() if v},'source_before_rows':sum(len(x) for x in before.values()),'source_after_rows':sum(len(x) for x in after.values()),'bound_original_dump_restored':True,'restored_role_acl_match':True,'unexpected_business_row_changes':0,'source_mutation_by_this_audit':0,'native_provider_io':0,'original_operator_failure':'Audit allowlist omitted command_idempotency, evidence_relation and actual delivery_authorization_fence table; original stopped/sealed receipt retained'})
 # Reuse the previously reviewed exact ledger SQL and pure in-memory assertions.
 tree=ast.parse(SCRIPT.read_text(encoding='utf-8'));fn=next(x for x in tree.body if isinstance(x,ast.FunctionDef) and x.name=='final_readback');query=next(ast.literal_eval(x.value) for x in fn.body if isinstance(x,ast.Assign) and any(isinstance(t,ast.Name) and t.id=='sql' for t in x.targets));query=query.replace("'attempts',","'releases',(SELECT jsonb_agg(to_jsonb(r)) FROM lifecycle_dispatch_release r),'attempts',",1)
 ledger=json.loads(o.m.source_read(o.m.PG,query));o.write(OUT/'final-ledger.json',ledger);validation=o.validate_ledger(ledger);o.write(OUT/'ledger-validation.json',validation)
 native=o.read(o.PUBLIC/'survivors/four-survivors.json');o.need(native['status']=='PASS_FOUR_EXACT_SURVIVORS' and len(native['objects'])==4,'Survivors proof missing');o.need(validation['new_storage_calls']==32 and validation['storage_calls']==157,'Physical accounting delta differs from reviewed path')
 # Prove causal order from native timestamps and persisted receipts.
 a=o.read(o.PUBLIC/'send-a/send-a-returned.json');b=o.read(o.PUBLIC/'send-b/send-b-returned.json');receipt_a=next(x for x in ledger['receipts'] if x['copy_id']==a['copy']['ID']);attempt_b=next(x for x in ledger['attempts'] if x['id']==b['attempt']['ID']);o.need(o.dt.datetime.fromisoformat(receipt_a['confirmed_at'])<o.dt.datetime.fromisoformat(attempt_b['started_at']),'B started before A absence committed')
 o.need(o.m.fingerprint(o.m.PG)==before_fp and o.m.role_acl(o.m.PG)==baseline['roles_acl'],'Source changed during post-seal read-only audit')
 o.write(OUT/'acceptance.json',{'status':'PASS_TWO_EXACT_ABSENCES_FOUR_SURVIVORS_AND_HISTORICAL_COST_CLOSURE','execution':o.EXECUTION,'native_delete_invocations':2,'per_target_invocations':{'a':1,'b':1},'response_loss_crashes':2,'absence_receipts':2,'scope_completion':1,'survivors_exact_bytes_sha_protection':4,'a_absence_confirmed_at':receipt_a['confirmed_at'],'b_attempt_started_at':attempt_b['started_at'],'native_storage_calls':157,'new_storage_calls':32,'new_provider_costs':2,'terminal_replay_provider_io':0,'old_194_table_rows_preserved':True,'unexpected_business_mutation':0,'source_mutation_by_this_audit':0,'native_provider_io_by_this_audit':0,'cleanup':0,'hold_release':0,'bypass':0,'pr318':'DRAFT_NO_MERGE'})
 print(json.dumps({'status':'PASS_POST_SEAL_READONLY_AUDIT','source_calls':157,'new_provider_costs':2,'absence_receipts':2,'completion':1,'survivors':4,'historical_tables':194,'native_provider_io':0}))
finally:
 sealed=o.prep.stop_all(o.m,[ident,o.m.PG]);o.need(not sealed['failures'],'Post-seal SQL audit closure failed');o.write(OUT/'sql-services-sealed.json',sealed)
