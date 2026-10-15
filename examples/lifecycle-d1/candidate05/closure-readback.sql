SELECT jsonb_build_object(
'as_of',now(),'new_basis',(SELECT count(*) FROM lifecycle_fixture_disposal_basis WHERE run_id='9ea8f3e2-c805-44b4-b133-a36f50df9d15'),'new_command_keys',(SELECT count(*) FROM command_idempotency WHERE idempotency_key LIKE '9ea8f3e2-c805-44b4-b133-a36f50df9d15:%'),'basis',(SELECT jsonb_agg(to_jsonb(x)) FROM lifecycle_fixture_disposal_basis x WHERE run_id='614cd9ad-3f9d-40be-b110-3f7cf95f0790'),
'observations',(SELECT jsonb_agg(to_jsonb(x)) FROM lifecycle_provider_observation x),
'attempts',(SELECT jsonb_agg(to_jsonb(x)) FROM lifecycle_provider_attempt x),
'claims',(SELECT jsonb_agg(to_jsonb(x)) FROM lifecycle_provider_send_claim x),
'releases',(SELECT jsonb_agg(to_jsonb(x)) FROM lifecycle_dispatch_release x),
'results',(SELECT jsonb_agg(to_jsonb(x)) FROM lifecycle_reconciliation_result x),
'absence',(SELECT count(*) FROM lifecycle_copy_absence_receipt),'completion',(SELECT count(*) FROM lifecycle_plan_completion),
'storage_calls',(SELECT count(*) FROM lifecycle_storage_call),'storage_observations',(SELECT count(*) FROM lifecycle_storage_observation),
'storage_costs',(SELECT count(*) FROM cost_allocation WHERE lifecycle_storage_call_id IS NOT NULL),
'provider_costs',(SELECT jsonb_agg(jsonb_build_object('allocation',to_jsonb(a),'cost',to_jsonb(e))) FROM cost_allocation a JOIN cost_event e ON e.id=a.cost_event_id WHERE a.lifecycle_provider_attempt_id IS NOT NULL),
'unaccounted_storage',(SELECT count(*) FROM lifecycle_storage_observation x WHERE x.invoked_at IS NOT NULL AND (SELECT count(*) FROM cost_allocation a JOIN cost_event e ON e.id=a.cost_event_id WHERE a.lifecycle_storage_call_id=x.call_id AND e.activity_id=x.call_id AND e.quantity=1 AND e.unit='invocation')<>1),
'old_outbox_states',(SELECT jsonb_agg(to_jsonb(x)) FROM (SELECT event_type,status,count(*) FROM outbox_event WHERE created_at<'2026-10-15T08:37:14Z' GROUP BY event_type,status ORDER BY event_type,status)x));
