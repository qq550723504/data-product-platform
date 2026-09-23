DROP TRIGGER IF EXISTS trg_annotation_snapshot_schema_validation ON annotation_snapshot;
DROP FUNCTION IF EXISTS validate_annotation_snapshot_result_schemas();

DROP TRIGGER IF EXISTS trg_annotation_result_schema_validation ON annotation_result;
DROP FUNCTION IF EXISTS validate_annotation_result_schema_insert();

DROP FUNCTION IF EXISTS annotation_payload_matches_frozen_schema(text, bytea);
