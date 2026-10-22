"""Known Core readback plus exact FK qualification; no locks/commands/mutations."""
import datetime as dt,hashlib,json,re
from pathlib import Path

def collect(m,out):
 sql=(m.ROOT/'docs/acceptance/evidence/issue307-d1/delete-proposal-5d2274ee/core-readback.sql').read_text(encoding='utf-8-sig')
 query=sql.split('\n',1)[1].rsplit('ROLLBACK;',1)[0]
 core=json.loads(m.source_read(m.PG,query));m.write(out/'current-core-readback.json',core)
 extra="""SELECT jsonb_build_object('provider_observations',(SELECT jsonb_agg(to_jsonb(o) ORDER BY o.attempt_id) FROM lifecycle_provider_observation o),'dispatch_releases',(SELECT jsonb_agg(to_jsonb(x) ORDER BY x.dispatch_id) FROM lifecycle_dispatch_release x),'requirements',(SELECT jsonb_agg(to_jsonb(q) ORDER BY q.id) FROM lifecycle_fixture_reference_requirement q),'reference_releases',(SELECT jsonb_agg(to_jsonb(r) ORDER BY r.requirement_id) FROM lifecycle_fixture_reference_release r),'unreleased_requirements',(SELECT count(*) FROM lifecycle_fixture_reference_requirement q WHERE NOT EXISTS(SELECT 1 FROM lifecycle_fixture_reference_release x WHERE x.workspace_id=q.workspace_id AND x.requirement_id=q.id)))::text;"""
 facts=json.loads(m.source_read(m.PG,extra));m.write(out/'owner-closure-readback.json',facts)
 discovery="""SELECT coalesce(jsonb_agg(to_jsonb(x) ORDER BY x.schema,x.table,x.column),'[]')::text FROM (SELECT DISTINCT ns.nspname AS schema,t.relname AS table,a.attname AS column FROM pg_constraint f JOIN pg_class t ON t.oid=f.conrelid JOIN pg_namespace ns ON ns.oid=t.relnamespace JOIN LATERAL unnest(f.conkey,f.confkey) AS k(local_col,foreign_col) ON true JOIN pg_attribute a ON a.attrelid=t.oid AND a.attnum=k.local_col JOIN pg_attribute b ON b.attrelid=f.confrelid AND b.attnum=k.foreign_col WHERE f.contype='f' AND f.confrelid='dataset_version'::regclass AND b.attname='id' AND NOT (ns.nspname='public' AND a.attname='dataset_version_id' AND t.relname IN ('lifecycle_fixture_copy','lifecycle_content_use_restriction','lifecycle_copy_policy','lifecycle_restore_checkpoint','lifecycle_fixture_reference_requirement'))) x;"""
 fk=json.loads(m.source_read(m.PG,discovery));source=(m.ROOT/'apps/platform/internal/platform/contentreference/reference.go').read_text(encoding='utf-8-sig');specs={(table,column):(kind,fact) for kind,table,column,fact in re.findall(r'\{"([A-Z_]+)", "([a-z_]+)", "([a-z_]+)", "([a-z_]+)",',source)};m.require(len(specs)==8,'Core reference owner contract drift')
 guarded=[];counts=[];queries=[query,extra,discovery]
 targets={'8b1ff30c-3695-4847-b4cf-cc92cd6a399c':'dd7aa6eb-50ea-4c5c-a6ff-79a244bcc152','0a239dc6-9e1d-49eb-8604-e22a93e7b6a0':'35b26496-f832-4496-bb91-1f50fb56e7fa'}
 for rel in fk:
  schema,table,column=(rel[x] for x in ['schema','table','column']);m.require(all(re.fullmatch('[a-z_][a-z_0-9]*',x) for x in [schema,table,column]),'Unknown SQL identifier contract')
  guard=f"""SELECT jsonb_build_object('qualified',EXISTS(SELECT 1 FROM pg_trigger g JOIN pg_class t ON t.oid=g.tgrelid JOIN pg_namespace n ON n.oid=t.relnamespace JOIN pg_attribute a ON a.attrelid=t.oid AND a.attname='{column}' AND NOT a.attisdropped WHERE n.nspname='{schema}' AND t.relname='{table}' AND g.tgname='aa_lifecycle_ref_{column}' AND g.tgenabled='O' AND g.tgfoid='lifecycle_reference_admission_guard()'::regprocedure AND g.tgtype=23 AND g.tgqual IS NULL AND g.tgnargs=1 AND g.tgargs=decode(encode(convert_to('{column}','UTF8'),'hex')||'00','hex') AND cardinality(g.tgattr)=1 AND g.tgattr[0]=a.attnum),'trigger',(SELECT pg_get_triggerdef(g.oid) FROM pg_trigger g JOIN pg_class t ON t.oid=g.tgrelid JOIN pg_namespace n ON n.oid=t.relnamespace WHERE n.nspname='{schema}' AND t.relname='{table}' AND g.tgname='aa_lifecycle_ref_{column}'))::text;"""
  g=json.loads(m.source_read(m.PG,guard));queries.append(guard);guarded.append(dict(rel,**g))
  for copy,dv in targets.items():
   predicate=f'c."{column}"=\'{dv}\'';identity=None
   if schema=='public' and (table,column) in specs:
    kind,fact=specs[(table,column)];identity='q.execution_id=c.execution_id AND q.input_name=c.input_name' if kind=='EXECUTION_INPUT' else f'q."{fact}"=c.id'
   unreleased='count(*)' if identity is None else f"count(*) FILTER(WHERE NOT EXISTS(SELECT 1 FROM lifecycle_fixture_reference_requirement q JOIN lifecycle_fixture_reference_release x ON x.requirement_id=q.id AND x.workspace_id=q.workspace_id WHERE q.workspace_id='89eba3fd-9763-4f66-8cec-1a779104f155' AND q.copy_id='{copy}' AND q.kind='{kind}' AND {identity}))"
   count=f'SELECT jsonb_build_object(\'count\',count(*),\'unreleased\',{unreleased})::text FROM "{schema}"."{table}" c WHERE {predicate};';n=json.loads(m.source_read(m.PG,count));queries.append(count);counts.append(dict(rel,copy_id=copy,managed=identity is not None,**n))
 m.require(all(x['qualified'] for x in guarded) and all(x['unreleased']==0 for x in counts),'Current reference guard/owner closure blocked')
 definitions=json.loads(m.source_read(m.PG,"SELECT jsonb_agg(jsonb_build_object('name',p.proname,'body',p.prosrc,'security_definer',p.prosecdef,'owner',pg_get_userbyid(p.proowner)) ORDER BY p.proname)::text FROM pg_proc p JOIN pg_namespace n ON n.oid=p.pronamespace WHERE n.nspname='public' AND p.proname IN ('lifecycle_reference_lock','lifecycle_reference_admission_guard','lifecycle_pending_copy_admission_guard');"))
 migration=(m.ROOT/'migrations/000067_lifecycle_fixture_reference_demand.up.sql').read_text(encoding='utf-8-sig');m.require(len(definitions)==3,'Reference function membership drift')
 for f in definitions:
  expected=re.search(r'CREATE FUNCTION '+f['name']+r'\(.*?AS \$\$(.*?)\$\$',migration,re.S);m.require(expected is not None and expected.group(1).replace('\r\n','\n')==f['body'].replace('\r\n','\n'),'Reference fence function body drift');f['body_sha256']=hashlib.sha256(f['body'].encode()).hexdigest();f['matches_migration_67']=True
 m.write(out/'reference-qualification-readback.json',{'at':dt.datetime.now(dt.timezone.utc).isoformat(),'mode':'REPEATABLE READ READ ONLY; no authorization fence/row lock/Command','guards':guarded,'target_fk_counts':counts,'function_definitions':definitions,'status':'PASS','authorization':'NOT_GRANTED'})
 (out/'current-preconditions-readonly.sql').write_text('\n\n'.join('BEGIN ISOLATION LEVEL REPEATABLE READ READ ONLY;\n'+q+'\nROLLBACK;' for q in queries),encoding='utf-8',newline='\n')
 m.require(core['schema']==68 and core['counts']['storage_calls']==92 and len(facts['provider_observations'])==len(facts['dispatch_releases'])==2 and facts['unreleased_requirements']==0,'Core closed stage changed')
 m.require(all(x['certainty']==x['outcome']=='NOT_SENT' and x['invoked_at'] is None for x in facts['provider_observations']),'Old trial has invocation/unknown')
 return core
