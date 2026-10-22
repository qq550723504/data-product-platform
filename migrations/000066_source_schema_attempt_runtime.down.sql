LOCK TABLE source_schema_attempt,source_schema_attempt_runtime IN ACCESS EXCLUSIVE MODE;
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM source_schema_attempt_runtime) THEN
  RAISE EXCEPTION 'cannot remove recorded source schema runtime contracts';
 END IF;
END; $$;
DROP TRIGGER source_schema_attempt_runtime_required ON source_schema_attempt;
DROP FUNCTION require_source_schema_attempt_runtime();
DROP TABLE source_schema_attempt_runtime;
