LOCK TABLE source_admission_connection,source_admission_capture,source_admission_operation,source_admission_identity,source_admission_call IN ACCESS EXCLUSIVE MODE;
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM source_admission_connection) OR EXISTS(SELECT 1 FROM source_admission_operation) THEN RAISE EXCEPTION 'retained source admission history prohibits rollback'; END IF;
END; $$;
DO $$ DECLARE definition text; BEGIN
 SELECT pg_get_functiondef('guard_cost_allocation_workspace()'::regprocedure) INTO definition;
 EXECUTE regexp_replace(definition,E'\n-- source-admission cost begin.*?-- source-admission cost end\n','','s');
 SELECT pg_get_constraintdef(oid) INTO definition FROM pg_constraint WHERE conrelid='cost_allocation'::regclass AND conname='ck_cost_allocation_single_subject';
 ALTER TABLE cost_allocation DROP CONSTRAINT ck_cost_allocation_single_subject;
 EXECUTE 'ALTER TABLE cost_allocation ADD CONSTRAINT ck_cost_allocation_single_subject '||regexp_replace(definition,'source_admission_call_id, ?','');
END; $$;
ALTER TABLE cost_allocation DROP COLUMN source_admission_call_id;
DROP TABLE source_admission_observation,source_admission_call,source_admission_member,source_admission_identity,source_admission_asset,source_admission_operation,source_admission_capture,source_admission_connection;
DROP FUNCTION source_admission_guard();
