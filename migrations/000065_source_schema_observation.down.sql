LOCK TABLE source_schema_request,source_schema_attempt,source_schema_observation,source_schema_handling,cost_allocation IN ACCESS EXCLUSIVE MODE;
DO $$
DECLARE definition text;
BEGIN
 IF EXISTS(SELECT 1 FROM source_schema_request) OR EXISTS(SELECT 1 FROM cost_allocation WHERE source_schema_attempt_id IS NOT NULL) THEN RAISE EXCEPTION 'cannot downgrade immutable source schema history'; END IF;
 SELECT pg_get_functiondef('guard_cost_allocation_workspace()'::regprocedure) INTO definition;
 EXECUTE regexp_replace(definition,E'\n-- source-schema guard begin.*?-- source-schema guard end\n','','s');
 SELECT pg_get_constraintdef(oid) INTO definition FROM pg_constraint WHERE conrelid='cost_allocation'::regclass AND conname='ck_cost_allocation_single_subject';
 ALTER TABLE cost_allocation DROP CONSTRAINT ck_cost_allocation_single_subject;
 ALTER TABLE cost_allocation DROP COLUMN source_schema_attempt_id;
 EXECUTE 'ALTER TABLE cost_allocation ADD CONSTRAINT ck_cost_allocation_single_subject ' || replace(definition,'source_schema_attempt_id, ','');
END; $$;
DROP TABLE source_schema_handling,source_schema_observation,source_schema_attempt,source_schema_request;
DROP FUNCTION source_schema_fact_guard();
