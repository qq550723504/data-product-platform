BEGIN ISOLATION LEVEL REPEATABLE READ READ ONLY;
SELECT jsonb_build_object(
 'as_of',clock_timestamp(),'schema',(SELECT max(version) FROM schema_migration),
 'counts',jsonb_build_object(
  'copies',(SELECT count(*) FROM lifecycle_fixture_copy),'requirements',(SELECT count(*) FROM lifecycle_fixture_reference_requirement),'reference_releases',(SELECT count(*) FROM lifecycle_fixture_reference_release),
  'pins',(SELECT count(*) FROM lifecycle_reader_pin),'terminations',(SELECT count(*) FROM lifecycle_reader_termination),'active_pins',(SELECT count(*) FROM lifecycle_reader_pin p WHERE NOT EXISTS(SELECT 1 FROM lifecycle_reader_termination t WHERE t.pin_id=p.id)),
  'copy_operations',(SELECT count(*) FROM lifecycle_copy_operation),'processing_output_intents',(SELECT count(*) FROM lifecycle_processing_output_intent),'processing_output_ready',(SELECT count(*) FROM lifecycle_processing_output_ready),
  'storage_calls',(SELECT count(*) FROM lifecycle_storage_call),'storage_observations',(SELECT count(*) FROM lifecycle_storage_observation),'typed_storage_costs',(SELECT count(*) FROM cost_allocation WHERE lifecycle_storage_call_id IS NOT NULL),
  'send_claims',(SELECT count(*) FROM lifecycle_provider_send_claim),'provider_observations',(SELECT count(*) FROM lifecycle_provider_observation),'absence_receipts',(SELECT count(*) FROM lifecycle_copy_absence_receipt),'completion',(SELECT count(*) FROM lifecycle_plan_completion)),
 'copies',(SELECT jsonb_agg(to_jsonb(c)||jsonb_build_object('namespace',n.namespace,'provider_instance_id',n.provider_instance_id,'incarnation_id',n.incarnation_id) ORDER BY n.namespace,c.object_key,c.provider_version) FROM lifecycle_fixture_copy c JOIN lifecycle_fixture_namespace n ON n.id=c.namespace_id),
 'plans',(SELECT jsonb_agg(to_jsonb(p)||jsonb_build_object('computed_hash',lifecycle_plan_scope_hash(p.id),'targets',(SELECT jsonb_agg(t.copy_id ORDER BY t.copy_id) FROM lifecycle_disposition_plan_target t WHERE t.plan_id=p.id))) FROM lifecycle_disposition_plan p),
 'approvals',(SELECT jsonb_agg(to_jsonb(a)||jsonb_build_object('expired_as_of_readback',a.expires_at<=clock_timestamp())) FROM lifecycle_disposition_approval a),
 'basis',(SELECT jsonb_agg(to_jsonb(b)) FROM lifecycle_fixture_disposal_basis b),
 'dispatches',(SELECT jsonb_agg(to_jsonb(d)) FROM lifecycle_disposition_dispatch d),
 'attempts',(SELECT jsonb_agg(to_jsonb(a)) FROM lifecycle_provider_attempt a),
 'restrictions',(SELECT jsonb_agg(to_jsonb(r)) FROM lifecycle_content_use_restriction r),
 'holds',(SELECT jsonb_agg(to_jsonb(h)) FROM lifecycle_content_hold h),
 'copy_operations',(SELECT jsonb_agg(to_jsonb(o)) FROM lifecycle_copy_operation o),
 'processing_output_intents',(SELECT jsonb_agg(to_jsonb(o)) FROM lifecycle_processing_output_intent o),
 'processing_output_ready',(SELECT jsonb_agg(to_jsonb(o)) FROM lifecycle_processing_output_ready o)
);

ROLLBACK;

BEGIN ISOLATION LEVEL REPEATABLE READ READ ONLY;
SELECT jsonb_build_object('provider_observations',(SELECT jsonb_agg(to_jsonb(o) ORDER BY o.attempt_id) FROM lifecycle_provider_observation o),'dispatch_releases',(SELECT jsonb_agg(to_jsonb(x) ORDER BY x.dispatch_id) FROM lifecycle_dispatch_release x),'requirements',(SELECT jsonb_agg(to_jsonb(q) ORDER BY q.id) FROM lifecycle_fixture_reference_requirement q),'reference_releases',(SELECT jsonb_agg(to_jsonb(r) ORDER BY r.requirement_id) FROM lifecycle_fixture_reference_release r),'unreleased_requirements',(SELECT count(*) FROM lifecycle_fixture_reference_requirement q WHERE NOT EXISTS(SELECT 1 FROM lifecycle_fixture_reference_release x WHERE x.workspace_id=q.workspace_id AND x.requirement_id=q.id)))::text;
ROLLBACK;

BEGIN ISOLATION LEVEL REPEATABLE READ READ ONLY;
SELECT coalesce(jsonb_agg(to_jsonb(x) ORDER BY x.schema,x.table,x.column),'[]')::text FROM (SELECT DISTINCT ns.nspname AS schema,t.relname AS table,a.attname AS column FROM pg_constraint f JOIN pg_class t ON t.oid=f.conrelid JOIN pg_namespace ns ON ns.oid=t.relnamespace JOIN LATERAL unnest(f.conkey,f.confkey) AS k(local_col,foreign_col) ON true JOIN pg_attribute a ON a.attrelid=t.oid AND a.attnum=k.local_col JOIN pg_attribute b ON b.attrelid=f.confrelid AND b.attnum=k.foreign_col WHERE f.contype='f' AND f.confrelid='dataset_version'::regclass AND b.attname='id' AND NOT (ns.nspname='public' AND a.attname='dataset_version_id' AND t.relname IN ('lifecycle_fixture_copy','lifecycle_content_use_restriction','lifecycle_copy_policy','lifecycle_restore_checkpoint','lifecycle_fixture_reference_requirement'))) x;
ROLLBACK;

BEGIN ISOLATION LEVEL REPEATABLE READ READ ONLY;
SELECT jsonb_build_object('qualified',EXISTS(SELECT 1 FROM pg_trigger g JOIN pg_class t ON t.oid=g.tgrelid JOIN pg_namespace n ON n.oid=t.relnamespace JOIN pg_attribute a ON a.attrelid=t.oid AND a.attname='input_dataset_version_id' AND NOT a.attisdropped WHERE n.nspname='public' AND t.relname='annotation_campaign' AND g.tgname='aa_lifecycle_ref_input_dataset_version_id' AND g.tgenabled='O' AND g.tgfoid='lifecycle_reference_admission_guard()'::regprocedure AND g.tgtype=23 AND g.tgqual IS NULL AND g.tgnargs=1 AND g.tgargs=decode(encode(convert_to('input_dataset_version_id','UTF8'),'hex')||'00','hex') AND cardinality(g.tgattr)=1 AND g.tgattr[0]=a.attnum),'trigger',(SELECT pg_get_triggerdef(g.oid) FROM pg_trigger g JOIN pg_class t ON t.oid=g.tgrelid JOIN pg_namespace n ON n.oid=t.relnamespace WHERE n.nspname='public' AND t.relname='annotation_campaign' AND g.tgname='aa_lifecycle_ref_input_dataset_version_id'))::text;
ROLLBACK;

BEGIN ISOLATION LEVEL REPEATABLE READ READ ONLY;
SELECT jsonb_build_object('count',count(*),'unreleased',count(*))::text FROM "public"."annotation_campaign" c WHERE c."input_dataset_version_id"='dd7aa6eb-50ea-4c5c-a6ff-79a244bcc152';
ROLLBACK;

BEGIN ISOLATION LEVEL REPEATABLE READ READ ONLY;
SELECT jsonb_build_object('count',count(*),'unreleased',count(*))::text FROM "public"."annotation_campaign" c WHERE c."input_dataset_version_id"='35b26496-f832-4496-bb91-1f50fb56e7fa';
ROLLBACK;

BEGIN ISOLATION LEVEL REPEATABLE READ READ ONLY;
SELECT jsonb_build_object('qualified',EXISTS(SELECT 1 FROM pg_trigger g JOIN pg_class t ON t.oid=g.tgrelid JOIN pg_namespace n ON n.oid=t.relnamespace JOIN pg_attribute a ON a.attrelid=t.oid AND a.attname='dataset_version_id' AND NOT a.attisdropped WHERE n.nspname='public' AND t.relname='compliance_result' AND g.tgname='aa_lifecycle_ref_dataset_version_id' AND g.tgenabled='O' AND g.tgfoid='lifecycle_reference_admission_guard()'::regprocedure AND g.tgtype=23 AND g.tgqual IS NULL AND g.tgnargs=1 AND g.tgargs=decode(encode(convert_to('dataset_version_id','UTF8'),'hex')||'00','hex') AND cardinality(g.tgattr)=1 AND g.tgattr[0]=a.attnum),'trigger',(SELECT pg_get_triggerdef(g.oid) FROM pg_trigger g JOIN pg_class t ON t.oid=g.tgrelid JOIN pg_namespace n ON n.oid=t.relnamespace WHERE n.nspname='public' AND t.relname='compliance_result' AND g.tgname='aa_lifecycle_ref_dataset_version_id'))::text;
ROLLBACK;

BEGIN ISOLATION LEVEL REPEATABLE READ READ ONLY;
SELECT jsonb_build_object('count',count(*),'unreleased',count(*))::text FROM "public"."compliance_result" c WHERE c."dataset_version_id"='dd7aa6eb-50ea-4c5c-a6ff-79a244bcc152';
ROLLBACK;

BEGIN ISOLATION LEVEL REPEATABLE READ READ ONLY;
SELECT jsonb_build_object('count',count(*),'unreleased',count(*))::text FROM "public"."compliance_result" c WHERE c."dataset_version_id"='35b26496-f832-4496-bb91-1f50fb56e7fa';
ROLLBACK;

BEGIN ISOLATION LEVEL REPEATABLE READ READ ONLY;
SELECT jsonb_build_object('qualified',EXISTS(SELECT 1 FROM pg_trigger g JOIN pg_class t ON t.oid=g.tgrelid JOIN pg_namespace n ON n.oid=t.relnamespace JOIN pg_attribute a ON a.attrelid=t.oid AND a.attname='current_version_id' AND NOT a.attisdropped WHERE n.nspname='public' AND t.relname='dataset' AND g.tgname='aa_lifecycle_ref_current_version_id' AND g.tgenabled='O' AND g.tgfoid='lifecycle_reference_admission_guard()'::regprocedure AND g.tgtype=23 AND g.tgqual IS NULL AND g.tgnargs=1 AND g.tgargs=decode(encode(convert_to('current_version_id','UTF8'),'hex')||'00','hex') AND cardinality(g.tgattr)=1 AND g.tgattr[0]=a.attnum),'trigger',(SELECT pg_get_triggerdef(g.oid) FROM pg_trigger g JOIN pg_class t ON t.oid=g.tgrelid JOIN pg_namespace n ON n.oid=t.relnamespace WHERE n.nspname='public' AND t.relname='dataset' AND g.tgname='aa_lifecycle_ref_current_version_id'))::text;
ROLLBACK;

BEGIN ISOLATION LEVEL REPEATABLE READ READ ONLY;
SELECT jsonb_build_object('count',count(*),'unreleased',count(*))::text FROM "public"."dataset" c WHERE c."current_version_id"='dd7aa6eb-50ea-4c5c-a6ff-79a244bcc152';
ROLLBACK;

BEGIN ISOLATION LEVEL REPEATABLE READ READ ONLY;
SELECT jsonb_build_object('count',count(*),'unreleased',count(*))::text FROM "public"."dataset" c WHERE c."current_version_id"='35b26496-f832-4496-bb91-1f50fb56e7fa';
ROLLBACK;

BEGIN ISOLATION LEVEL REPEATABLE READ READ ONLY;
SELECT jsonb_build_object('qualified',EXISTS(SELECT 1 FROM pg_trigger g JOIN pg_class t ON t.oid=g.tgrelid JOIN pg_namespace n ON n.oid=t.relnamespace JOIN pg_attribute a ON a.attrelid=t.oid AND a.attname='dataset_version_id' AND NOT a.attisdropped WHERE n.nspname='public' AND t.relname='dataset_certification' AND g.tgname='aa_lifecycle_ref_dataset_version_id' AND g.tgenabled='O' AND g.tgfoid='lifecycle_reference_admission_guard()'::regprocedure AND g.tgtype=23 AND g.tgqual IS NULL AND g.tgnargs=1 AND g.tgargs=decode(encode(convert_to('dataset_version_id','UTF8'),'hex')||'00','hex') AND cardinality(g.tgattr)=1 AND g.tgattr[0]=a.attnum),'trigger',(SELECT pg_get_triggerdef(g.oid) FROM pg_trigger g JOIN pg_class t ON t.oid=g.tgrelid JOIN pg_namespace n ON n.oid=t.relnamespace WHERE n.nspname='public' AND t.relname='dataset_certification' AND g.tgname='aa_lifecycle_ref_dataset_version_id'))::text;
ROLLBACK;

BEGIN ISOLATION LEVEL REPEATABLE READ READ ONLY;
SELECT jsonb_build_object('count',count(*),'unreleased',count(*) FILTER(WHERE NOT EXISTS(SELECT 1 FROM lifecycle_fixture_reference_requirement q JOIN lifecycle_fixture_reference_release x ON x.requirement_id=q.id AND x.workspace_id=q.workspace_id WHERE q.workspace_id='89eba3fd-9763-4f66-8cec-1a779104f155' AND q.copy_id='8b1ff30c-3695-4847-b4cf-cc92cd6a399c' AND q.kind='CERTIFICATION' AND q."dataset_certification_id"=c.id)))::text FROM "public"."dataset_certification" c WHERE c."dataset_version_id"='dd7aa6eb-50ea-4c5c-a6ff-79a244bcc152';
ROLLBACK;

BEGIN ISOLATION LEVEL REPEATABLE READ READ ONLY;
SELECT jsonb_build_object('count',count(*),'unreleased',count(*) FILTER(WHERE NOT EXISTS(SELECT 1 FROM lifecycle_fixture_reference_requirement q JOIN lifecycle_fixture_reference_release x ON x.requirement_id=q.id AND x.workspace_id=q.workspace_id WHERE q.workspace_id='89eba3fd-9763-4f66-8cec-1a779104f155' AND q.copy_id='0a239dc6-9e1d-49eb-8604-e22a93e7b6a0' AND q.kind='CERTIFICATION' AND q."dataset_certification_id"=c.id)))::text FROM "public"."dataset_certification" c WHERE c."dataset_version_id"='35b26496-f832-4496-bb91-1f50fb56e7fa';
ROLLBACK;

BEGIN ISOLATION LEVEL REPEATABLE READ READ ONLY;
SELECT jsonb_build_object('qualified',EXISTS(SELECT 1 FROM pg_trigger g JOIN pg_class t ON t.oid=g.tgrelid JOIN pg_namespace n ON n.oid=t.relnamespace JOIN pg_attribute a ON a.attrelid=t.oid AND a.attname='input_version_id' AND NOT a.attisdropped WHERE n.nspname='public' AND t.relname='dataset_version_lineage' AND g.tgname='aa_lifecycle_ref_input_version_id' AND g.tgenabled='O' AND g.tgfoid='lifecycle_reference_admission_guard()'::regprocedure AND g.tgtype=23 AND g.tgqual IS NULL AND g.tgnargs=1 AND g.tgargs=decode(encode(convert_to('input_version_id','UTF8'),'hex')||'00','hex') AND cardinality(g.tgattr)=1 AND g.tgattr[0]=a.attnum),'trigger',(SELECT pg_get_triggerdef(g.oid) FROM pg_trigger g JOIN pg_class t ON t.oid=g.tgrelid JOIN pg_namespace n ON n.oid=t.relnamespace WHERE n.nspname='public' AND t.relname='dataset_version_lineage' AND g.tgname='aa_lifecycle_ref_input_version_id'))::text;
ROLLBACK;

BEGIN ISOLATION LEVEL REPEATABLE READ READ ONLY;
SELECT jsonb_build_object('count',count(*),'unreleased',count(*) FILTER(WHERE NOT EXISTS(SELECT 1 FROM lifecycle_fixture_reference_requirement q JOIN lifecycle_fixture_reference_release x ON x.requirement_id=q.id AND x.workspace_id=q.workspace_id WHERE q.workspace_id='89eba3fd-9763-4f66-8cec-1a779104f155' AND q.copy_id='8b1ff30c-3695-4847-b4cf-cc92cd6a399c' AND q.kind='LINEAGE' AND q."lineage_id"=c.id)))::text FROM "public"."dataset_version_lineage" c WHERE c."input_version_id"='dd7aa6eb-50ea-4c5c-a6ff-79a244bcc152';
ROLLBACK;

BEGIN ISOLATION LEVEL REPEATABLE READ READ ONLY;
SELECT jsonb_build_object('count',count(*),'unreleased',count(*) FILTER(WHERE NOT EXISTS(SELECT 1 FROM lifecycle_fixture_reference_requirement q JOIN lifecycle_fixture_reference_release x ON x.requirement_id=q.id AND x.workspace_id=q.workspace_id WHERE q.workspace_id='89eba3fd-9763-4f66-8cec-1a779104f155' AND q.copy_id='0a239dc6-9e1d-49eb-8604-e22a93e7b6a0' AND q.kind='LINEAGE' AND q."lineage_id"=c.id)))::text FROM "public"."dataset_version_lineage" c WHERE c."input_version_id"='35b26496-f832-4496-bb91-1f50fb56e7fa';
ROLLBACK;

BEGIN ISOLATION LEVEL REPEATABLE READ READ ONLY;
SELECT jsonb_build_object('qualified',EXISTS(SELECT 1 FROM pg_trigger g JOIN pg_class t ON t.oid=g.tgrelid JOIN pg_namespace n ON n.oid=t.relnamespace JOIN pg_attribute a ON a.attrelid=t.oid AND a.attname='output_version_id' AND NOT a.attisdropped WHERE n.nspname='public' AND t.relname='dataset_version_lineage' AND g.tgname='aa_lifecycle_ref_output_version_id' AND g.tgenabled='O' AND g.tgfoid='lifecycle_reference_admission_guard()'::regprocedure AND g.tgtype=23 AND g.tgqual IS NULL AND g.tgnargs=1 AND g.tgargs=decode(encode(convert_to('output_version_id','UTF8'),'hex')||'00','hex') AND cardinality(g.tgattr)=1 AND g.tgattr[0]=a.attnum),'trigger',(SELECT pg_get_triggerdef(g.oid) FROM pg_trigger g JOIN pg_class t ON t.oid=g.tgrelid JOIN pg_namespace n ON n.oid=t.relnamespace WHERE n.nspname='public' AND t.relname='dataset_version_lineage' AND g.tgname='aa_lifecycle_ref_output_version_id'))::text;
ROLLBACK;

BEGIN ISOLATION LEVEL REPEATABLE READ READ ONLY;
SELECT jsonb_build_object('count',count(*),'unreleased',count(*))::text FROM "public"."dataset_version_lineage" c WHERE c."output_version_id"='dd7aa6eb-50ea-4c5c-a6ff-79a244bcc152';
ROLLBACK;

BEGIN ISOLATION LEVEL REPEATABLE READ READ ONLY;
SELECT jsonb_build_object('count',count(*),'unreleased',count(*))::text FROM "public"."dataset_version_lineage" c WHERE c."output_version_id"='35b26496-f832-4496-bb91-1f50fb56e7fa';
ROLLBACK;

BEGIN ISOLATION LEVEL REPEATABLE READ READ ONLY;
SELECT jsonb_build_object('qualified',EXISTS(SELECT 1 FROM pg_trigger g JOIN pg_class t ON t.oid=g.tgrelid JOIN pg_namespace n ON n.oid=t.relnamespace JOIN pg_attribute a ON a.attrelid=t.oid AND a.attname='dataset_version_id' AND NOT a.attisdropped WHERE n.nspname='public' AND t.relname='delivery_operation' AND g.tgname='aa_lifecycle_ref_dataset_version_id' AND g.tgenabled='O' AND g.tgfoid='lifecycle_reference_admission_guard()'::regprocedure AND g.tgtype=23 AND g.tgqual IS NULL AND g.tgnargs=1 AND g.tgargs=decode(encode(convert_to('dataset_version_id','UTF8'),'hex')||'00','hex') AND cardinality(g.tgattr)=1 AND g.tgattr[0]=a.attnum),'trigger',(SELECT pg_get_triggerdef(g.oid) FROM pg_trigger g JOIN pg_class t ON t.oid=g.tgrelid JOIN pg_namespace n ON n.oid=t.relnamespace WHERE n.nspname='public' AND t.relname='delivery_operation' AND g.tgname='aa_lifecycle_ref_dataset_version_id'))::text;
ROLLBACK;

BEGIN ISOLATION LEVEL REPEATABLE READ READ ONLY;
SELECT jsonb_build_object('count',count(*),'unreleased',count(*) FILTER(WHERE NOT EXISTS(SELECT 1 FROM lifecycle_fixture_reference_requirement q JOIN lifecycle_fixture_reference_release x ON x.requirement_id=q.id AND x.workspace_id=q.workspace_id WHERE q.workspace_id='89eba3fd-9763-4f66-8cec-1a779104f155' AND q.copy_id='8b1ff30c-3695-4847-b4cf-cc92cd6a399c' AND q.kind='DELIVERY' AND q."delivery_operation_id"=c.id)))::text FROM "public"."delivery_operation" c WHERE c."dataset_version_id"='dd7aa6eb-50ea-4c5c-a6ff-79a244bcc152';
ROLLBACK;

BEGIN ISOLATION LEVEL REPEATABLE READ READ ONLY;
SELECT jsonb_build_object('count',count(*),'unreleased',count(*) FILTER(WHERE NOT EXISTS(SELECT 1 FROM lifecycle_fixture_reference_requirement q JOIN lifecycle_fixture_reference_release x ON x.requirement_id=q.id AND x.workspace_id=q.workspace_id WHERE q.workspace_id='89eba3fd-9763-4f66-8cec-1a779104f155' AND q.copy_id='0a239dc6-9e1d-49eb-8604-e22a93e7b6a0' AND q.kind='DELIVERY' AND q."delivery_operation_id"=c.id)))::text FROM "public"."delivery_operation" c WHERE c."dataset_version_id"='35b26496-f832-4496-bb91-1f50fb56e7fa';
ROLLBACK;

BEGIN ISOLATION LEVEL REPEATABLE READ READ ONLY;
SELECT jsonb_build_object('qualified',EXISTS(SELECT 1 FROM pg_trigger g JOIN pg_class t ON t.oid=g.tgrelid JOIN pg_namespace n ON n.oid=t.relnamespace JOIN pg_attribute a ON a.attrelid=t.oid AND a.attname='input_dataset_version_id' AND NOT a.attisdropped WHERE n.nspname='public' AND t.relname='effective_rights_input' AND g.tgname='aa_lifecycle_ref_input_dataset_version_id' AND g.tgenabled='O' AND g.tgfoid='lifecycle_reference_admission_guard()'::regprocedure AND g.tgtype=23 AND g.tgqual IS NULL AND g.tgnargs=1 AND g.tgargs=decode(encode(convert_to('input_dataset_version_id','UTF8'),'hex')||'00','hex') AND cardinality(g.tgattr)=1 AND g.tgattr[0]=a.attnum),'trigger',(SELECT pg_get_triggerdef(g.oid) FROM pg_trigger g JOIN pg_class t ON t.oid=g.tgrelid JOIN pg_namespace n ON n.oid=t.relnamespace WHERE n.nspname='public' AND t.relname='effective_rights_input' AND g.tgname='aa_lifecycle_ref_input_dataset_version_id'))::text;
ROLLBACK;

BEGIN ISOLATION LEVEL REPEATABLE READ READ ONLY;
SELECT jsonb_build_object('count',count(*),'unreleased',count(*))::text FROM "public"."effective_rights_input" c WHERE c."input_dataset_version_id"='dd7aa6eb-50ea-4c5c-a6ff-79a244bcc152';
ROLLBACK;

BEGIN ISOLATION LEVEL REPEATABLE READ READ ONLY;
SELECT jsonb_build_object('count',count(*),'unreleased',count(*))::text FROM "public"."effective_rights_input" c WHERE c."input_dataset_version_id"='35b26496-f832-4496-bb91-1f50fb56e7fa';
ROLLBACK;

BEGIN ISOLATION LEVEL REPEATABLE READ READ ONLY;
SELECT jsonb_build_object('qualified',EXISTS(SELECT 1 FROM pg_trigger g JOIN pg_class t ON t.oid=g.tgrelid JOIN pg_namespace n ON n.oid=t.relnamespace JOIN pg_attribute a ON a.attrelid=t.oid AND a.attname='target_dataset_version_id' AND NOT a.attisdropped WHERE n.nspname='public' AND t.relname='effective_rights_snapshot' AND g.tgname='aa_lifecycle_ref_target_dataset_version_id' AND g.tgenabled='O' AND g.tgfoid='lifecycle_reference_admission_guard()'::regprocedure AND g.tgtype=23 AND g.tgqual IS NULL AND g.tgnargs=1 AND g.tgargs=decode(encode(convert_to('target_dataset_version_id','UTF8'),'hex')||'00','hex') AND cardinality(g.tgattr)=1 AND g.tgattr[0]=a.attnum),'trigger',(SELECT pg_get_triggerdef(g.oid) FROM pg_trigger g JOIN pg_class t ON t.oid=g.tgrelid JOIN pg_namespace n ON n.oid=t.relnamespace WHERE n.nspname='public' AND t.relname='effective_rights_snapshot' AND g.tgname='aa_lifecycle_ref_target_dataset_version_id'))::text;
ROLLBACK;

BEGIN ISOLATION LEVEL REPEATABLE READ READ ONLY;
SELECT jsonb_build_object('count',count(*),'unreleased',count(*))::text FROM "public"."effective_rights_snapshot" c WHERE c."target_dataset_version_id"='dd7aa6eb-50ea-4c5c-a6ff-79a244bcc152';
ROLLBACK;

BEGIN ISOLATION LEVEL REPEATABLE READ READ ONLY;
SELECT jsonb_build_object('count',count(*),'unreleased',count(*))::text FROM "public"."effective_rights_snapshot" c WHERE c."target_dataset_version_id"='35b26496-f832-4496-bb91-1f50fb56e7fa';
ROLLBACK;

BEGIN ISOLATION LEVEL REPEATABLE READ READ ONLY;
SELECT jsonb_build_object('qualified',EXISTS(SELECT 1 FROM pg_trigger g JOIN pg_class t ON t.oid=g.tgrelid JOIN pg_namespace n ON n.oid=t.relnamespace JOIN pg_attribute a ON a.attrelid=t.oid AND a.attname='input_dataset_version_id' AND NOT a.attisdropped WHERE n.nspname='public' AND t.relname='entity_match_job' AND g.tgname='aa_lifecycle_ref_input_dataset_version_id' AND g.tgenabled='O' AND g.tgfoid='lifecycle_reference_admission_guard()'::regprocedure AND g.tgtype=23 AND g.tgqual IS NULL AND g.tgnargs=1 AND g.tgargs=decode(encode(convert_to('input_dataset_version_id','UTF8'),'hex')||'00','hex') AND cardinality(g.tgattr)=1 AND g.tgattr[0]=a.attnum),'trigger',(SELECT pg_get_triggerdef(g.oid) FROM pg_trigger g JOIN pg_class t ON t.oid=g.tgrelid JOIN pg_namespace n ON n.oid=t.relnamespace WHERE n.nspname='public' AND t.relname='entity_match_job' AND g.tgname='aa_lifecycle_ref_input_dataset_version_id'))::text;
ROLLBACK;

BEGIN ISOLATION LEVEL REPEATABLE READ READ ONLY;
SELECT jsonb_build_object('count',count(*),'unreleased',count(*) FILTER(WHERE NOT EXISTS(SELECT 1 FROM lifecycle_fixture_reference_requirement q JOIN lifecycle_fixture_reference_release x ON x.requirement_id=q.id AND x.workspace_id=q.workspace_id WHERE q.workspace_id='89eba3fd-9763-4f66-8cec-1a779104f155' AND q.copy_id='8b1ff30c-3695-4847-b4cf-cc92cd6a399c' AND q.kind='ENTITY_MATCH' AND q."entity_match_job_id"=c.id)))::text FROM "public"."entity_match_job" c WHERE c."input_dataset_version_id"='dd7aa6eb-50ea-4c5c-a6ff-79a244bcc152';
ROLLBACK;

BEGIN ISOLATION LEVEL REPEATABLE READ READ ONLY;
SELECT jsonb_build_object('count',count(*),'unreleased',count(*) FILTER(WHERE NOT EXISTS(SELECT 1 FROM lifecycle_fixture_reference_requirement q JOIN lifecycle_fixture_reference_release x ON x.requirement_id=q.id AND x.workspace_id=q.workspace_id WHERE q.workspace_id='89eba3fd-9763-4f66-8cec-1a779104f155' AND q.copy_id='0a239dc6-9e1d-49eb-8604-e22a93e7b6a0' AND q.kind='ENTITY_MATCH' AND q."entity_match_job_id"=c.id)))::text FROM "public"."entity_match_job" c WHERE c."input_dataset_version_id"='35b26496-f832-4496-bb91-1f50fb56e7fa';
ROLLBACK;

BEGIN ISOLATION LEVEL REPEATABLE READ READ ONLY;
SELECT jsonb_build_object('qualified',EXISTS(SELECT 1 FROM pg_trigger g JOIN pg_class t ON t.oid=g.tgrelid JOIN pg_namespace n ON n.oid=t.relnamespace JOIN pg_attribute a ON a.attrelid=t.oid AND a.attname='output_dataset_version_id' AND NOT a.attisdropped WHERE n.nspname='public' AND t.relname='entity_match_job' AND g.tgname='aa_lifecycle_ref_output_dataset_version_id' AND g.tgenabled='O' AND g.tgfoid='lifecycle_reference_admission_guard()'::regprocedure AND g.tgtype=23 AND g.tgqual IS NULL AND g.tgnargs=1 AND g.tgargs=decode(encode(convert_to('output_dataset_version_id','UTF8'),'hex')||'00','hex') AND cardinality(g.tgattr)=1 AND g.tgattr[0]=a.attnum),'trigger',(SELECT pg_get_triggerdef(g.oid) FROM pg_trigger g JOIN pg_class t ON t.oid=g.tgrelid JOIN pg_namespace n ON n.oid=t.relnamespace WHERE n.nspname='public' AND t.relname='entity_match_job' AND g.tgname='aa_lifecycle_ref_output_dataset_version_id'))::text;
ROLLBACK;

BEGIN ISOLATION LEVEL REPEATABLE READ READ ONLY;
SELECT jsonb_build_object('count',count(*),'unreleased',count(*))::text FROM "public"."entity_match_job" c WHERE c."output_dataset_version_id"='dd7aa6eb-50ea-4c5c-a6ff-79a244bcc152';
ROLLBACK;

BEGIN ISOLATION LEVEL REPEATABLE READ READ ONLY;
SELECT jsonb_build_object('count',count(*),'unreleased',count(*))::text FROM "public"."entity_match_job" c WHERE c."output_dataset_version_id"='35b26496-f832-4496-bb91-1f50fb56e7fa';
ROLLBACK;

BEGIN ISOLATION LEVEL REPEATABLE READ READ ONLY;
SELECT jsonb_build_object('qualified',EXISTS(SELECT 1 FROM pg_trigger g JOIN pg_class t ON t.oid=g.tgrelid JOIN pg_namespace n ON n.oid=t.relnamespace JOIN pg_attribute a ON a.attrelid=t.oid AND a.attname='output_dataset_version_id' AND NOT a.attisdropped WHERE n.nspname='public' AND t.relname='entity_resolution_output_decision' AND g.tgname='aa_lifecycle_ref_output_dataset_version_id' AND g.tgenabled='O' AND g.tgfoid='lifecycle_reference_admission_guard()'::regprocedure AND g.tgtype=23 AND g.tgqual IS NULL AND g.tgnargs=1 AND g.tgargs=decode(encode(convert_to('output_dataset_version_id','UTF8'),'hex')||'00','hex') AND cardinality(g.tgattr)=1 AND g.tgattr[0]=a.attnum),'trigger',(SELECT pg_get_triggerdef(g.oid) FROM pg_trigger g JOIN pg_class t ON t.oid=g.tgrelid JOIN pg_namespace n ON n.oid=t.relnamespace WHERE n.nspname='public' AND t.relname='entity_resolution_output_decision' AND g.tgname='aa_lifecycle_ref_output_dataset_version_id'))::text;
ROLLBACK;

BEGIN ISOLATION LEVEL REPEATABLE READ READ ONLY;
SELECT jsonb_build_object('count',count(*),'unreleased',count(*))::text FROM "public"."entity_resolution_output_decision" c WHERE c."output_dataset_version_id"='dd7aa6eb-50ea-4c5c-a6ff-79a244bcc152';
ROLLBACK;

BEGIN ISOLATION LEVEL REPEATABLE READ READ ONLY;
SELECT jsonb_build_object('count',count(*),'unreleased',count(*))::text FROM "public"."entity_resolution_output_decision" c WHERE c."output_dataset_version_id"='35b26496-f832-4496-bb91-1f50fb56e7fa';
ROLLBACK;

BEGIN ISOLATION LEVEL REPEATABLE READ READ ONLY;
SELECT jsonb_build_object('qualified',EXISTS(SELECT 1 FROM pg_trigger g JOIN pg_class t ON t.oid=g.tgrelid JOIN pg_namespace n ON n.oid=t.relnamespace JOIN pg_attribute a ON a.attrelid=t.oid AND a.attname='output_dataset_version_id' AND NOT a.attisdropped WHERE n.nspname='public' AND t.relname='execution' AND g.tgname='aa_lifecycle_ref_output_dataset_version_id' AND g.tgenabled='O' AND g.tgfoid='lifecycle_reference_admission_guard()'::regprocedure AND g.tgtype=23 AND g.tgqual IS NULL AND g.tgnargs=1 AND g.tgargs=decode(encode(convert_to('output_dataset_version_id','UTF8'),'hex')||'00','hex') AND cardinality(g.tgattr)=1 AND g.tgattr[0]=a.attnum),'trigger',(SELECT pg_get_triggerdef(g.oid) FROM pg_trigger g JOIN pg_class t ON t.oid=g.tgrelid JOIN pg_namespace n ON n.oid=t.relnamespace WHERE n.nspname='public' AND t.relname='execution' AND g.tgname='aa_lifecycle_ref_output_dataset_version_id'))::text;
ROLLBACK;

BEGIN ISOLATION LEVEL REPEATABLE READ READ ONLY;
SELECT jsonb_build_object('count',count(*),'unreleased',count(*))::text FROM "public"."execution" c WHERE c."output_dataset_version_id"='dd7aa6eb-50ea-4c5c-a6ff-79a244bcc152';
ROLLBACK;

BEGIN ISOLATION LEVEL REPEATABLE READ READ ONLY;
SELECT jsonb_build_object('count',count(*),'unreleased',count(*))::text FROM "public"."execution" c WHERE c."output_dataset_version_id"='35b26496-f832-4496-bb91-1f50fb56e7fa';
ROLLBACK;

BEGIN ISOLATION LEVEL REPEATABLE READ READ ONLY;
SELECT jsonb_build_object('qualified',EXISTS(SELECT 1 FROM pg_trigger g JOIN pg_class t ON t.oid=g.tgrelid JOIN pg_namespace n ON n.oid=t.relnamespace JOIN pg_attribute a ON a.attrelid=t.oid AND a.attname='dataset_version_id' AND NOT a.attisdropped WHERE n.nspname='public' AND t.relname='execution_dependency_binding' AND g.tgname='aa_lifecycle_ref_dataset_version_id' AND g.tgenabled='O' AND g.tgfoid='lifecycle_reference_admission_guard()'::regprocedure AND g.tgtype=23 AND g.tgqual IS NULL AND g.tgnargs=1 AND g.tgargs=decode(encode(convert_to('dataset_version_id','UTF8'),'hex')||'00','hex') AND cardinality(g.tgattr)=1 AND g.tgattr[0]=a.attnum),'trigger',(SELECT pg_get_triggerdef(g.oid) FROM pg_trigger g JOIN pg_class t ON t.oid=g.tgrelid JOIN pg_namespace n ON n.oid=t.relnamespace WHERE n.nspname='public' AND t.relname='execution_dependency_binding' AND g.tgname='aa_lifecycle_ref_dataset_version_id'))::text;
ROLLBACK;

BEGIN ISOLATION LEVEL REPEATABLE READ READ ONLY;
SELECT jsonb_build_object('count',count(*),'unreleased',count(*))::text FROM "public"."execution_dependency_binding" c WHERE c."dataset_version_id"='dd7aa6eb-50ea-4c5c-a6ff-79a244bcc152';
ROLLBACK;

BEGIN ISOLATION LEVEL REPEATABLE READ READ ONLY;
SELECT jsonb_build_object('count',count(*),'unreleased',count(*))::text FROM "public"."execution_dependency_binding" c WHERE c."dataset_version_id"='35b26496-f832-4496-bb91-1f50fb56e7fa';
ROLLBACK;

BEGIN ISOLATION LEVEL REPEATABLE READ READ ONLY;
SELECT jsonb_build_object('qualified',EXISTS(SELECT 1 FROM pg_trigger g JOIN pg_class t ON t.oid=g.tgrelid JOIN pg_namespace n ON n.oid=t.relnamespace JOIN pg_attribute a ON a.attrelid=t.oid AND a.attname='dataset_version_id' AND NOT a.attisdropped WHERE n.nspname='public' AND t.relname='execution_input' AND g.tgname='aa_lifecycle_ref_dataset_version_id' AND g.tgenabled='O' AND g.tgfoid='lifecycle_reference_admission_guard()'::regprocedure AND g.tgtype=23 AND g.tgqual IS NULL AND g.tgnargs=1 AND g.tgargs=decode(encode(convert_to('dataset_version_id','UTF8'),'hex')||'00','hex') AND cardinality(g.tgattr)=1 AND g.tgattr[0]=a.attnum),'trigger',(SELECT pg_get_triggerdef(g.oid) FROM pg_trigger g JOIN pg_class t ON t.oid=g.tgrelid JOIN pg_namespace n ON n.oid=t.relnamespace WHERE n.nspname='public' AND t.relname='execution_input' AND g.tgname='aa_lifecycle_ref_dataset_version_id'))::text;
ROLLBACK;

BEGIN ISOLATION LEVEL REPEATABLE READ READ ONLY;
SELECT jsonb_build_object('count',count(*),'unreleased',count(*) FILTER(WHERE NOT EXISTS(SELECT 1 FROM lifecycle_fixture_reference_requirement q JOIN lifecycle_fixture_reference_release x ON x.requirement_id=q.id AND x.workspace_id=q.workspace_id WHERE q.workspace_id='89eba3fd-9763-4f66-8cec-1a779104f155' AND q.copy_id='8b1ff30c-3695-4847-b4cf-cc92cd6a399c' AND q.kind='EXECUTION_INPUT' AND q.execution_id=c.execution_id AND q.input_name=c.input_name)))::text FROM "public"."execution_input" c WHERE c."dataset_version_id"='dd7aa6eb-50ea-4c5c-a6ff-79a244bcc152';
ROLLBACK;

BEGIN ISOLATION LEVEL REPEATABLE READ READ ONLY;
SELECT jsonb_build_object('count',count(*),'unreleased',count(*) FILTER(WHERE NOT EXISTS(SELECT 1 FROM lifecycle_fixture_reference_requirement q JOIN lifecycle_fixture_reference_release x ON x.requirement_id=q.id AND x.workspace_id=q.workspace_id WHERE q.workspace_id='89eba3fd-9763-4f66-8cec-1a779104f155' AND q.copy_id='0a239dc6-9e1d-49eb-8604-e22a93e7b6a0' AND q.kind='EXECUTION_INPUT' AND q.execution_id=c.execution_id AND q.input_name=c.input_name)))::text FROM "public"."execution_input" c WHERE c."dataset_version_id"='35b26496-f832-4496-bb91-1f50fb56e7fa';
ROLLBACK;

BEGIN ISOLATION LEVEL REPEATABLE READ READ ONLY;
SELECT jsonb_build_object('qualified',EXISTS(SELECT 1 FROM pg_trigger g JOIN pg_class t ON t.oid=g.tgrelid JOIN pg_namespace n ON n.oid=t.relnamespace JOIN pg_attribute a ON a.attrelid=t.oid AND a.attname='input_dataset_version_id' AND NOT a.attisdropped WHERE n.nspname='public' AND t.relname='execution_mapping_usage' AND g.tgname='aa_lifecycle_ref_input_dataset_version_id' AND g.tgenabled='O' AND g.tgfoid='lifecycle_reference_admission_guard()'::regprocedure AND g.tgtype=23 AND g.tgqual IS NULL AND g.tgnargs=1 AND g.tgargs=decode(encode(convert_to('input_dataset_version_id','UTF8'),'hex')||'00','hex') AND cardinality(g.tgattr)=1 AND g.tgattr[0]=a.attnum),'trigger',(SELECT pg_get_triggerdef(g.oid) FROM pg_trigger g JOIN pg_class t ON t.oid=g.tgrelid JOIN pg_namespace n ON n.oid=t.relnamespace WHERE n.nspname='public' AND t.relname='execution_mapping_usage' AND g.tgname='aa_lifecycle_ref_input_dataset_version_id'))::text;
ROLLBACK;

BEGIN ISOLATION LEVEL REPEATABLE READ READ ONLY;
SELECT jsonb_build_object('count',count(*),'unreleased',count(*) FILTER(WHERE NOT EXISTS(SELECT 1 FROM lifecycle_fixture_reference_requirement q JOIN lifecycle_fixture_reference_release x ON x.requirement_id=q.id AND x.workspace_id=q.workspace_id WHERE q.workspace_id='89eba3fd-9763-4f66-8cec-1a779104f155' AND q.copy_id='8b1ff30c-3695-4847-b4cf-cc92cd6a399c' AND q.kind='EXECUTION_MAPPING' AND q."execution_mapping_usage_id"=c.id)))::text FROM "public"."execution_mapping_usage" c WHERE c."input_dataset_version_id"='dd7aa6eb-50ea-4c5c-a6ff-79a244bcc152';
ROLLBACK;

BEGIN ISOLATION LEVEL REPEATABLE READ READ ONLY;
SELECT jsonb_build_object('count',count(*),'unreleased',count(*) FILTER(WHERE NOT EXISTS(SELECT 1 FROM lifecycle_fixture_reference_requirement q JOIN lifecycle_fixture_reference_release x ON x.requirement_id=q.id AND x.workspace_id=q.workspace_id WHERE q.workspace_id='89eba3fd-9763-4f66-8cec-1a779104f155' AND q.copy_id='0a239dc6-9e1d-49eb-8604-e22a93e7b6a0' AND q.kind='EXECUTION_MAPPING' AND q."execution_mapping_usage_id"=c.id)))::text FROM "public"."execution_mapping_usage" c WHERE c."input_dataset_version_id"='35b26496-f832-4496-bb91-1f50fb56e7fa';
ROLLBACK;

BEGIN ISOLATION LEVEL REPEATABLE READ READ ONLY;
SELECT jsonb_build_object('qualified',EXISTS(SELECT 1 FROM pg_trigger g JOIN pg_class t ON t.oid=g.tgrelid JOIN pg_namespace n ON n.oid=t.relnamespace JOIN pg_attribute a ON a.attrelid=t.oid AND a.attname='resolution_dataset_version_id' AND NOT a.attisdropped WHERE n.nspname='public' AND t.relname='execution_mapping_usage' AND g.tgname='aa_lifecycle_ref_resolution_dataset_version_id' AND g.tgenabled='O' AND g.tgfoid='lifecycle_reference_admission_guard()'::regprocedure AND g.tgtype=23 AND g.tgqual IS NULL AND g.tgnargs=1 AND g.tgargs=decode(encode(convert_to('resolution_dataset_version_id','UTF8'),'hex')||'00','hex') AND cardinality(g.tgattr)=1 AND g.tgattr[0]=a.attnum),'trigger',(SELECT pg_get_triggerdef(g.oid) FROM pg_trigger g JOIN pg_class t ON t.oid=g.tgrelid JOIN pg_namespace n ON n.oid=t.relnamespace WHERE n.nspname='public' AND t.relname='execution_mapping_usage' AND g.tgname='aa_lifecycle_ref_resolution_dataset_version_id'))::text;
ROLLBACK;

BEGIN ISOLATION LEVEL REPEATABLE READ READ ONLY;
SELECT jsonb_build_object('count',count(*),'unreleased',count(*))::text FROM "public"."execution_mapping_usage" c WHERE c."resolution_dataset_version_id"='dd7aa6eb-50ea-4c5c-a6ff-79a244bcc152';
ROLLBACK;

BEGIN ISOLATION LEVEL REPEATABLE READ READ ONLY;
SELECT jsonb_build_object('count',count(*),'unreleased',count(*))::text FROM "public"."execution_mapping_usage" c WHERE c."resolution_dataset_version_id"='35b26496-f832-4496-bb91-1f50fb56e7fa';
ROLLBACK;

BEGIN ISOLATION LEVEL REPEATABLE READ READ ONLY;
SELECT jsonb_build_object('qualified',EXISTS(SELECT 1 FROM pg_trigger g JOIN pg_class t ON t.oid=g.tgrelid JOIN pg_namespace n ON n.oid=t.relnamespace JOIN pg_attribute a ON a.attrelid=t.oid AND a.attname='input_dataset_version_id' AND NOT a.attisdropped WHERE n.nspname='public' AND t.relname='gold_build_request' AND g.tgname='aa_lifecycle_ref_input_dataset_version_id' AND g.tgenabled='O' AND g.tgfoid='lifecycle_reference_admission_guard()'::regprocedure AND g.tgtype=23 AND g.tgqual IS NULL AND g.tgnargs=1 AND g.tgargs=decode(encode(convert_to('input_dataset_version_id','UTF8'),'hex')||'00','hex') AND cardinality(g.tgattr)=1 AND g.tgattr[0]=a.attnum),'trigger',(SELECT pg_get_triggerdef(g.oid) FROM pg_trigger g JOIN pg_class t ON t.oid=g.tgrelid JOIN pg_namespace n ON n.oid=t.relnamespace WHERE n.nspname='public' AND t.relname='gold_build_request' AND g.tgname='aa_lifecycle_ref_input_dataset_version_id'))::text;
ROLLBACK;

BEGIN ISOLATION LEVEL REPEATABLE READ READ ONLY;
SELECT jsonb_build_object('count',count(*),'unreleased',count(*))::text FROM "public"."gold_build_request" c WHERE c."input_dataset_version_id"='dd7aa6eb-50ea-4c5c-a6ff-79a244bcc152';
ROLLBACK;

BEGIN ISOLATION LEVEL REPEATABLE READ READ ONLY;
SELECT jsonb_build_object('count',count(*),'unreleased',count(*))::text FROM "public"."gold_build_request" c WHERE c."input_dataset_version_id"='35b26496-f832-4496-bb91-1f50fb56e7fa';
ROLLBACK;

BEGIN ISOLATION LEVEL REPEATABLE READ READ ONLY;
SELECT jsonb_build_object('qualified',EXISTS(SELECT 1 FROM pg_trigger g JOIN pg_class t ON t.oid=g.tgrelid JOIN pg_namespace n ON n.oid=t.relnamespace JOIN pg_attribute a ON a.attrelid=t.oid AND a.attname='input_dataset_version_id' AND NOT a.attisdropped WHERE n.nspname='public' AND t.relname='gold_production_binding' AND g.tgname='aa_lifecycle_ref_input_dataset_version_id' AND g.tgenabled='O' AND g.tgfoid='lifecycle_reference_admission_guard()'::regprocedure AND g.tgtype=23 AND g.tgqual IS NULL AND g.tgnargs=1 AND g.tgargs=decode(encode(convert_to('input_dataset_version_id','UTF8'),'hex')||'00','hex') AND cardinality(g.tgattr)=1 AND g.tgattr[0]=a.attnum),'trigger',(SELECT pg_get_triggerdef(g.oid) FROM pg_trigger g JOIN pg_class t ON t.oid=g.tgrelid JOIN pg_namespace n ON n.oid=t.relnamespace WHERE n.nspname='public' AND t.relname='gold_production_binding' AND g.tgname='aa_lifecycle_ref_input_dataset_version_id'))::text;
ROLLBACK;

BEGIN ISOLATION LEVEL REPEATABLE READ READ ONLY;
SELECT jsonb_build_object('count',count(*),'unreleased',count(*))::text FROM "public"."gold_production_binding" c WHERE c."input_dataset_version_id"='dd7aa6eb-50ea-4c5c-a6ff-79a244bcc152';
ROLLBACK;

BEGIN ISOLATION LEVEL REPEATABLE READ READ ONLY;
SELECT jsonb_build_object('count',count(*),'unreleased',count(*))::text FROM "public"."gold_production_binding" c WHERE c."input_dataset_version_id"='35b26496-f832-4496-bb91-1f50fb56e7fa';
ROLLBACK;

BEGIN ISOLATION LEVEL REPEATABLE READ READ ONLY;
SELECT jsonb_build_object('qualified',EXISTS(SELECT 1 FROM pg_trigger g JOIN pg_class t ON t.oid=g.tgrelid JOIN pg_namespace n ON n.oid=t.relnamespace JOIN pg_attribute a ON a.attrelid=t.oid AND a.attname='output_dataset_version_id' AND NOT a.attisdropped WHERE n.nspname='public' AND t.relname='gold_production_binding' AND g.tgname='aa_lifecycle_ref_output_dataset_version_id' AND g.tgenabled='O' AND g.tgfoid='lifecycle_reference_admission_guard()'::regprocedure AND g.tgtype=23 AND g.tgqual IS NULL AND g.tgnargs=1 AND g.tgargs=decode(encode(convert_to('output_dataset_version_id','UTF8'),'hex')||'00','hex') AND cardinality(g.tgattr)=1 AND g.tgattr[0]=a.attnum),'trigger',(SELECT pg_get_triggerdef(g.oid) FROM pg_trigger g JOIN pg_class t ON t.oid=g.tgrelid JOIN pg_namespace n ON n.oid=t.relnamespace WHERE n.nspname='public' AND t.relname='gold_production_binding' AND g.tgname='aa_lifecycle_ref_output_dataset_version_id'))::text;
ROLLBACK;

BEGIN ISOLATION LEVEL REPEATABLE READ READ ONLY;
SELECT jsonb_build_object('count',count(*),'unreleased',count(*))::text FROM "public"."gold_production_binding" c WHERE c."output_dataset_version_id"='dd7aa6eb-50ea-4c5c-a6ff-79a244bcc152';
ROLLBACK;

BEGIN ISOLATION LEVEL REPEATABLE READ READ ONLY;
SELECT jsonb_build_object('count',count(*),'unreleased',count(*))::text FROM "public"."gold_production_binding" c WHERE c."output_dataset_version_id"='35b26496-f832-4496-bb91-1f50fb56e7fa';
ROLLBACK;

BEGIN ISOLATION LEVEL REPEATABLE READ READ ONLY;
SELECT jsonb_build_object('qualified',EXISTS(SELECT 1 FROM pg_trigger g JOIN pg_class t ON t.oid=g.tgrelid JOIN pg_namespace n ON n.oid=t.relnamespace JOIN pg_attribute a ON a.attrelid=t.oid AND a.attname='version_id' AND NOT a.attisdropped WHERE n.nspname='public' AND t.relname='ingestion_receipt_slot' AND g.tgname='aa_lifecycle_ref_version_id' AND g.tgenabled='O' AND g.tgfoid='lifecycle_reference_admission_guard()'::regprocedure AND g.tgtype=23 AND g.tgqual IS NULL AND g.tgnargs=1 AND g.tgargs=decode(encode(convert_to('version_id','UTF8'),'hex')||'00','hex') AND cardinality(g.tgattr)=1 AND g.tgattr[0]=a.attnum),'trigger',(SELECT pg_get_triggerdef(g.oid) FROM pg_trigger g JOIN pg_class t ON t.oid=g.tgrelid JOIN pg_namespace n ON n.oid=t.relnamespace WHERE n.nspname='public' AND t.relname='ingestion_receipt_slot' AND g.tgname='aa_lifecycle_ref_version_id'))::text;
ROLLBACK;

BEGIN ISOLATION LEVEL REPEATABLE READ READ ONLY;
SELECT jsonb_build_object('count',count(*),'unreleased',count(*))::text FROM "public"."ingestion_receipt_slot" c WHERE c."version_id"='dd7aa6eb-50ea-4c5c-a6ff-79a244bcc152';
ROLLBACK;

BEGIN ISOLATION LEVEL REPEATABLE READ READ ONLY;
SELECT jsonb_build_object('count',count(*),'unreleased',count(*))::text FROM "public"."ingestion_receipt_slot" c WHERE c."version_id"='35b26496-f832-4496-bb91-1f50fb56e7fa';
ROLLBACK;

BEGIN ISOLATION LEVEL REPEATABLE READ READ ONLY;
SELECT jsonb_build_object('qualified',EXISTS(SELECT 1 FROM pg_trigger g JOIN pg_class t ON t.oid=g.tgrelid JOIN pg_namespace n ON n.oid=t.relnamespace JOIN pg_attribute a ON a.attrelid=t.oid AND a.attname='dataset_version_id' AND NOT a.attisdropped WHERE n.nspname='public' AND t.relname='lifecycle_processing_output_intent' AND g.tgname='aa_lifecycle_ref_dataset_version_id' AND g.tgenabled='O' AND g.tgfoid='lifecycle_reference_admission_guard()'::regprocedure AND g.tgtype=23 AND g.tgqual IS NULL AND g.tgnargs=1 AND g.tgargs=decode(encode(convert_to('dataset_version_id','UTF8'),'hex')||'00','hex') AND cardinality(g.tgattr)=1 AND g.tgattr[0]=a.attnum),'trigger',(SELECT pg_get_triggerdef(g.oid) FROM pg_trigger g JOIN pg_class t ON t.oid=g.tgrelid JOIN pg_namespace n ON n.oid=t.relnamespace WHERE n.nspname='public' AND t.relname='lifecycle_processing_output_intent' AND g.tgname='aa_lifecycle_ref_dataset_version_id'))::text;
ROLLBACK;

BEGIN ISOLATION LEVEL REPEATABLE READ READ ONLY;
SELECT jsonb_build_object('count',count(*),'unreleased',count(*))::text FROM "public"."lifecycle_processing_output_intent" c WHERE c."dataset_version_id"='dd7aa6eb-50ea-4c5c-a6ff-79a244bcc152';
ROLLBACK;

BEGIN ISOLATION LEVEL REPEATABLE READ READ ONLY;
SELECT jsonb_build_object('count',count(*),'unreleased',count(*))::text FROM "public"."lifecycle_processing_output_intent" c WHERE c."dataset_version_id"='35b26496-f832-4496-bb91-1f50fb56e7fa';
ROLLBACK;

BEGIN ISOLATION LEVEL REPEATABLE READ READ ONLY;
SELECT jsonb_build_object('qualified',EXISTS(SELECT 1 FROM pg_trigger g JOIN pg_class t ON t.oid=g.tgrelid JOIN pg_namespace n ON n.oid=t.relnamespace JOIN pg_attribute a ON a.attrelid=t.oid AND a.attname='dataset_version_id' AND NOT a.attisdropped WHERE n.nspname='public' AND t.relname='product_release_dataset' AND g.tgname='aa_lifecycle_ref_dataset_version_id' AND g.tgenabled='O' AND g.tgfoid='lifecycle_reference_admission_guard()'::regprocedure AND g.tgtype=23 AND g.tgqual IS NULL AND g.tgnargs=1 AND g.tgargs=decode(encode(convert_to('dataset_version_id','UTF8'),'hex')||'00','hex') AND cardinality(g.tgattr)=1 AND g.tgattr[0]=a.attnum),'trigger',(SELECT pg_get_triggerdef(g.oid) FROM pg_trigger g JOIN pg_class t ON t.oid=g.tgrelid JOIN pg_namespace n ON n.oid=t.relnamespace WHERE n.nspname='public' AND t.relname='product_release_dataset' AND g.tgname='aa_lifecycle_ref_dataset_version_id'))::text;
ROLLBACK;

BEGIN ISOLATION LEVEL REPEATABLE READ READ ONLY;
SELECT jsonb_build_object('count',count(*),'unreleased',count(*))::text FROM "public"."product_release_dataset" c WHERE c."dataset_version_id"='dd7aa6eb-50ea-4c5c-a6ff-79a244bcc152';
ROLLBACK;

BEGIN ISOLATION LEVEL REPEATABLE READ READ ONLY;
SELECT jsonb_build_object('count',count(*),'unreleased',count(*))::text FROM "public"."product_release_dataset" c WHERE c."dataset_version_id"='35b26496-f832-4496-bb91-1f50fb56e7fa';
ROLLBACK;

BEGIN ISOLATION LEVEL REPEATABLE READ READ ONLY;
SELECT jsonb_build_object('qualified',EXISTS(SELECT 1 FROM pg_trigger g JOIN pg_class t ON t.oid=g.tgrelid JOIN pg_namespace n ON n.oid=t.relnamespace JOIN pg_attribute a ON a.attrelid=t.oid AND a.attname='dataset_version_id' AND NOT a.attisdropped WHERE n.nspname='public' AND t.relname='quality_assessment_attempt' AND g.tgname='aa_lifecycle_ref_dataset_version_id' AND g.tgenabled='O' AND g.tgfoid='lifecycle_reference_admission_guard()'::regprocedure AND g.tgtype=23 AND g.tgqual IS NULL AND g.tgnargs=1 AND g.tgargs=decode(encode(convert_to('dataset_version_id','UTF8'),'hex')||'00','hex') AND cardinality(g.tgattr)=1 AND g.tgattr[0]=a.attnum),'trigger',(SELECT pg_get_triggerdef(g.oid) FROM pg_trigger g JOIN pg_class t ON t.oid=g.tgrelid JOIN pg_namespace n ON n.oid=t.relnamespace WHERE n.nspname='public' AND t.relname='quality_assessment_attempt' AND g.tgname='aa_lifecycle_ref_dataset_version_id'))::text;
ROLLBACK;

BEGIN ISOLATION LEVEL REPEATABLE READ READ ONLY;
SELECT jsonb_build_object('count',count(*),'unreleased',count(*) FILTER(WHERE NOT EXISTS(SELECT 1 FROM lifecycle_fixture_reference_requirement q JOIN lifecycle_fixture_reference_release x ON x.requirement_id=q.id AND x.workspace_id=q.workspace_id WHERE q.workspace_id='89eba3fd-9763-4f66-8cec-1a779104f155' AND q.copy_id='8b1ff30c-3695-4847-b4cf-cc92cd6a399c' AND q.kind='QUALITY_ATTEMPT' AND q."quality_assessment_attempt_id"=c.id)))::text FROM "public"."quality_assessment_attempt" c WHERE c."dataset_version_id"='dd7aa6eb-50ea-4c5c-a6ff-79a244bcc152';
ROLLBACK;

BEGIN ISOLATION LEVEL REPEATABLE READ READ ONLY;
SELECT jsonb_build_object('count',count(*),'unreleased',count(*) FILTER(WHERE NOT EXISTS(SELECT 1 FROM lifecycle_fixture_reference_requirement q JOIN lifecycle_fixture_reference_release x ON x.requirement_id=q.id AND x.workspace_id=q.workspace_id WHERE q.workspace_id='89eba3fd-9763-4f66-8cec-1a779104f155' AND q.copy_id='0a239dc6-9e1d-49eb-8604-e22a93e7b6a0' AND q.kind='QUALITY_ATTEMPT' AND q."quality_assessment_attempt_id"=c.id)))::text FROM "public"."quality_assessment_attempt" c WHERE c."dataset_version_id"='35b26496-f832-4496-bb91-1f50fb56e7fa';
ROLLBACK;

BEGIN ISOLATION LEVEL REPEATABLE READ READ ONLY;
SELECT jsonb_build_object('qualified',EXISTS(SELECT 1 FROM pg_trigger g JOIN pg_class t ON t.oid=g.tgrelid JOIN pg_namespace n ON n.oid=t.relnamespace JOIN pg_attribute a ON a.attrelid=t.oid AND a.attname='version_id' AND NOT a.attisdropped WHERE n.nspname='public' AND t.relname='quality_remediation_case' AND g.tgname='aa_lifecycle_ref_version_id' AND g.tgenabled='O' AND g.tgfoid='lifecycle_reference_admission_guard()'::regprocedure AND g.tgtype=23 AND g.tgqual IS NULL AND g.tgnargs=1 AND g.tgargs=decode(encode(convert_to('version_id','UTF8'),'hex')||'00','hex') AND cardinality(g.tgattr)=1 AND g.tgattr[0]=a.attnum),'trigger',(SELECT pg_get_triggerdef(g.oid) FROM pg_trigger g JOIN pg_class t ON t.oid=g.tgrelid JOIN pg_namespace n ON n.oid=t.relnamespace WHERE n.nspname='public' AND t.relname='quality_remediation_case' AND g.tgname='aa_lifecycle_ref_version_id'))::text;
ROLLBACK;

BEGIN ISOLATION LEVEL REPEATABLE READ READ ONLY;
SELECT jsonb_build_object('count',count(*),'unreleased',count(*))::text FROM "public"."quality_remediation_case" c WHERE c."version_id"='dd7aa6eb-50ea-4c5c-a6ff-79a244bcc152';
ROLLBACK;

BEGIN ISOLATION LEVEL REPEATABLE READ READ ONLY;
SELECT jsonb_build_object('count',count(*),'unreleased',count(*))::text FROM "public"."quality_remediation_case" c WHERE c."version_id"='35b26496-f832-4496-bb91-1f50fb56e7fa';
ROLLBACK;

BEGIN ISOLATION LEVEL REPEATABLE READ READ ONLY;
SELECT jsonb_build_object('qualified',EXISTS(SELECT 1 FROM pg_trigger g JOIN pg_class t ON t.oid=g.tgrelid JOIN pg_namespace n ON n.oid=t.relnamespace JOIN pg_attribute a ON a.attrelid=t.oid AND a.attname='version_id' AND NOT a.attisdropped WHERE n.nspname='public' AND t.relname='quality_remediation_detection' AND g.tgname='aa_lifecycle_ref_version_id' AND g.tgenabled='O' AND g.tgfoid='lifecycle_reference_admission_guard()'::regprocedure AND g.tgtype=23 AND g.tgqual IS NULL AND g.tgnargs=1 AND g.tgargs=decode(encode(convert_to('version_id','UTF8'),'hex')||'00','hex') AND cardinality(g.tgattr)=1 AND g.tgattr[0]=a.attnum),'trigger',(SELECT pg_get_triggerdef(g.oid) FROM pg_trigger g JOIN pg_class t ON t.oid=g.tgrelid JOIN pg_namespace n ON n.oid=t.relnamespace WHERE n.nspname='public' AND t.relname='quality_remediation_detection' AND g.tgname='aa_lifecycle_ref_version_id'))::text;
ROLLBACK;

BEGIN ISOLATION LEVEL REPEATABLE READ READ ONLY;
SELECT jsonb_build_object('count',count(*),'unreleased',count(*))::text FROM "public"."quality_remediation_detection" c WHERE c."version_id"='dd7aa6eb-50ea-4c5c-a6ff-79a244bcc152';
ROLLBACK;

BEGIN ISOLATION LEVEL REPEATABLE READ READ ONLY;
SELECT jsonb_build_object('count',count(*),'unreleased',count(*))::text FROM "public"."quality_remediation_detection" c WHERE c."version_id"='35b26496-f832-4496-bb91-1f50fb56e7fa';
ROLLBACK;

BEGIN ISOLATION LEVEL REPEATABLE READ READ ONLY;
SELECT jsonb_build_object('qualified',EXISTS(SELECT 1 FROM pg_trigger g JOIN pg_class t ON t.oid=g.tgrelid JOIN pg_namespace n ON n.oid=t.relnamespace JOIN pg_attribute a ON a.attrelid=t.oid AND a.attname='dataset_version_id' AND NOT a.attisdropped WHERE n.nspname='public' AND t.relname='quality_result' AND g.tgname='aa_lifecycle_ref_dataset_version_id' AND g.tgenabled='O' AND g.tgfoid='lifecycle_reference_admission_guard()'::regprocedure AND g.tgtype=23 AND g.tgqual IS NULL AND g.tgnargs=1 AND g.tgargs=decode(encode(convert_to('dataset_version_id','UTF8'),'hex')||'00','hex') AND cardinality(g.tgattr)=1 AND g.tgattr[0]=a.attnum),'trigger',(SELECT pg_get_triggerdef(g.oid) FROM pg_trigger g JOIN pg_class t ON t.oid=g.tgrelid JOIN pg_namespace n ON n.oid=t.relnamespace WHERE n.nspname='public' AND t.relname='quality_result' AND g.tgname='aa_lifecycle_ref_dataset_version_id'))::text;
ROLLBACK;

BEGIN ISOLATION LEVEL REPEATABLE READ READ ONLY;
SELECT jsonb_build_object('count',count(*),'unreleased',count(*) FILTER(WHERE NOT EXISTS(SELECT 1 FROM lifecycle_fixture_reference_requirement q JOIN lifecycle_fixture_reference_release x ON x.requirement_id=q.id AND x.workspace_id=q.workspace_id WHERE q.workspace_id='89eba3fd-9763-4f66-8cec-1a779104f155' AND q.copy_id='8b1ff30c-3695-4847-b4cf-cc92cd6a399c' AND q.kind='QUALITY_RESULT' AND q."quality_result_id"=c.id)))::text FROM "public"."quality_result" c WHERE c."dataset_version_id"='dd7aa6eb-50ea-4c5c-a6ff-79a244bcc152';
ROLLBACK;

BEGIN ISOLATION LEVEL REPEATABLE READ READ ONLY;
SELECT jsonb_build_object('count',count(*),'unreleased',count(*) FILTER(WHERE NOT EXISTS(SELECT 1 FROM lifecycle_fixture_reference_requirement q JOIN lifecycle_fixture_reference_release x ON x.requirement_id=q.id AND x.workspace_id=q.workspace_id WHERE q.workspace_id='89eba3fd-9763-4f66-8cec-1a779104f155' AND q.copy_id='0a239dc6-9e1d-49eb-8604-e22a93e7b6a0' AND q.kind='QUALITY_RESULT' AND q."quality_result_id"=c.id)))::text FROM "public"."quality_result" c WHERE c."dataset_version_id"='35b26496-f832-4496-bb91-1f50fb56e7fa';
ROLLBACK;