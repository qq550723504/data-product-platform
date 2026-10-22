BEGIN READ ONLY;
SELECT json_build_object(
 'copies',(SELECT count(*) FROM lifecycle_fixture_copy),
 'pins',(SELECT count(*) FROM lifecycle_reader_pin),
 'terminations',(SELECT count(*) FROM lifecycle_reader_termination),
 'active_pins',(SELECT count(*) FROM lifecycle_reader_pin p WHERE NOT EXISTS(SELECT 1 FROM lifecycle_reader_termination t WHERE t.pin_id=p.id)),
 'calls',(SELECT count(*) FROM lifecycle_storage_call),
 'observations',(SELECT count(*) FROM lifecycle_storage_observation),
 'storage_costs',(SELECT count(*) FROM cost_allocation WHERE lifecycle_storage_call_id IS NOT NULL),
 'unaccounted_storage',(SELECT count(*) FROM lifecycle_storage_call c WHERE (SELECT count(*) FROM cost_allocation a JOIN cost_event e ON e.id=a.cost_event_id WHERE a.lifecycle_storage_call_id=c.id AND e.activity_id=c.id AND e.quantity=1 AND e.unit='invocation')<>1),
 'provider_attempts',(SELECT count(*) FROM delivery_provider_attempt),
 'provider_observations',(SELECT count(*) FROM delivery_provider_observation),
 'provider_costs',(SELECT count(*) FROM cost_allocation a JOIN cost_event e ON e.id=a.cost_event_id WHERE e.cost_type='DELIVERY_PROVIDER_INVOCATION'),
 'unaccounted_provider',(SELECT count(*) FROM delivery_provider_attempt p WHERE (SELECT count(*) FROM cost_allocation a JOIN cost_event e ON e.id=a.cost_event_id WHERE a.delivery_operation_id=p.delivery_operation_id AND e.activity_id=p.id AND e.quantity=1 AND e.unit='invocation')<>1),
 'native_deliveries',(SELECT coalesce(jsonb_agg(jsonb_build_object('id',id,'status',status,'key',idempotency_key) ORDER BY id),'[]') FROM delivery_operation WHERE provider_name='LC13_MINIO_REDEMPTION'),
 'unreleased_credential_demands',(SELECT count(*) FROM lifecycle_fixture_reference_requirement r JOIN delivery_operation d ON d.id=r.delivery_operation_id WHERE d.provider_name='LC13_MINIO_REDEMPTION' AND NOT EXISTS(SELECT 1 FROM lifecycle_fixture_reference_release x WHERE x.requirement_id=r.id)),
 'replay_decisions',(SELECT coalesce(jsonb_agg(jsonb_build_object('operation',delivery_operation_id,'decision',decision,'at',created_at) ORDER BY created_at,id),'[]') FROM delivery_credential_replay_decision),
 'native_delete',(SELECT count(*) FROM lifecycle_storage_call WHERE operation='DELETE_EXACT'),
 'send_claims',(SELECT count(*) FROM lifecycle_provider_send_claim),
 'absence',(SELECT count(*) FROM lifecycle_copy_absence_receipt),
 'completion',(SELECT count(*) FROM lifecycle_plan_completion),
 'executions',(SELECT coalesce(jsonb_agg(jsonb_build_object('id',id,'status',status,'retry_of',retry_of_execution_id,'output',output_dataset_version_id) ORDER BY created_at,id),'[]') FROM execution));
ROLLBACK;
