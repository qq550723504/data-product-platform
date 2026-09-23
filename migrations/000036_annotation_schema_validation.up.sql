-- #204 post-merge blocker closeout.
-- Enforce the frozen Pilot annotation schema at the database boundary so
-- repository/direct-SQL writes cannot seal schema-invalid annotation history.

CREATE OR REPLACE FUNCTION annotation_payload_matches_frozen_schema(
    schema_snapshot text,
    canonical_payload bytea
)
RETURNS boolean AS $$
DECLARE
    schema_json jsonb;
    payload_json jsonb;
    label text;
    label_count integer;
    distinct_label_count integer;
BEGIN
    schema_json := schema_snapshot::jsonb;
    payload_json := convert_from(canonical_payload, 'UTF8')::jsonb;

    IF schema_json->>'kind' IS DISTINCT FROM 'single-label-v1'
       OR jsonb_typeof(schema_json->'labels') IS DISTINCT FROM 'array'
       OR jsonb_array_length(schema_json->'labels') < 1 THEN
        RETURN false;
    END IF;

    IF EXISTS (
        SELECT 1
          FROM jsonb_array_elements(schema_json->'labels') item
         WHERE jsonb_typeof(item) IS DISTINCT FROM 'string'
    ) THEN
        RETURN false;
    END IF;

    SELECT count(*), count(DISTINCT value)
      INTO label_count, distinct_label_count
      FROM jsonb_array_elements_text(schema_json->'labels') labels(value);

    IF label_count <> distinct_label_count
       OR EXISTS (
            SELECT 1
              FROM jsonb_array_elements_text(schema_json->'labels') labels(value)
             WHERE value = '' OR value IS DISTINCT FROM btrim(value)
       ) THEN
        RETURN false;
    END IF;

    IF jsonb_typeof(payload_json) IS DISTINCT FROM 'object'
       OR (SELECT count(*) FROM jsonb_object_keys(payload_json)) <> 1
       OR NOT (payload_json ? 'label')
       OR jsonb_typeof(payload_json->'label') IS DISTINCT FROM 'string' THEN
        RETURN false;
    END IF;

    label := payload_json->>'label';
    IF label IS NULL OR label = '' OR label IS DISTINCT FROM btrim(label) THEN
        RETURN false;
    END IF;

    RETURN EXISTS (
        SELECT 1
          FROM jsonb_array_elements_text(schema_json->'labels') labels(value)
         WHERE value = label
    );
EXCEPTION
    WHEN others THEN
        RETURN false;
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE FUNCTION validate_annotation_result_schema_insert()
RETURNS trigger AS $$
DECLARE
    schema_snapshot text;
BEGIN
    SELECT schema_content_snapshot
      INTO schema_snapshot
      FROM annotation_campaign
     WHERE id=NEW.campaign_id;

    IF schema_snapshot IS NULL
       OR NOT annotation_payload_matches_frozen_schema(schema_snapshot, NEW.canonical_payload) THEN
        RAISE EXCEPTION 'annotation result payload violates frozen schema';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_annotation_result_schema_validation
BEFORE INSERT ON annotation_result
FOR EACH ROW EXECUTE FUNCTION validate_annotation_result_schema_insert();

CREATE OR REPLACE FUNCTION validate_annotation_snapshot_result_schemas()
RETURNS trigger AS $$
DECLARE
    schema_snapshot text;
BEGIN
    IF OLD.status='BUILDING' AND NEW.status='FINALIZED' THEN
        SELECT schema_content_snapshot
          INTO schema_snapshot
          FROM annotation_campaign
         WHERE id=OLD.campaign_id;

        IF schema_snapshot IS NULL OR EXISTS (
            SELECT 1
              FROM annotation_snapshot_result sr
              JOIN annotation_result r ON r.id=sr.result_id
             WHERE sr.snapshot_id=OLD.id
               AND NOT annotation_payload_matches_frozen_schema(schema_snapshot, r.canonical_payload)
        ) THEN
            RAISE EXCEPTION 'annotation snapshot contains result outside frozen schema';
        END IF;
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_annotation_snapshot_schema_validation
BEFORE UPDATE ON annotation_snapshot
FOR EACH ROW EXECUTE FUNCTION validate_annotation_snapshot_result_schemas();
