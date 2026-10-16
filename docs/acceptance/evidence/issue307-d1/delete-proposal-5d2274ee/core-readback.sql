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
