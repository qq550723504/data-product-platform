-- #204 closure: make provider replay aliases durable Core facts.
-- Observation aliases are append-only mappings to immutable AnnotationResult
-- identities. This prevents a successful replay alias from being claimed later
-- by another result.

CREATE TABLE annotation_result_alias (
    workspace_id                uuid NOT NULL,
    campaign_id                 uuid NOT NULL REFERENCES annotation_campaign(id),
    alias                       varchar(1024) NOT NULL,
    result_id                   uuid NOT NULL REFERENCES annotation_result(id),
    created_at                  timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (workspace_id, campaign_id, alias),
    CONSTRAINT ck_annotation_result_alias_nonempty CHECK (length(btrim(alias)) > 0)
);

CREATE INDEX idx_annotation_result_alias_result
    ON annotation_result_alias(result_id, created_at);

CREATE OR REPLACE FUNCTION validate_annotation_result_alias_insert()
RETURNS trigger AS $$
DECLARE
    result_workspace uuid;
    result_campaign uuid;
BEGIN
    SELECT workspace_id, campaign_id
      INTO result_workspace, result_campaign
      FROM annotation_result
     WHERE id=NEW.result_id;

    IF result_workspace IS DISTINCT FROM NEW.workspace_id
       OR result_campaign IS DISTINCT FROM NEW.campaign_id THEN
        RAISE EXCEPTION 'annotation result alias crosses result boundary';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_annotation_result_alias_insert
BEFORE INSERT ON annotation_result_alias
FOR EACH ROW EXECUTE FUNCTION validate_annotation_result_alias_insert();

CREATE OR REPLACE FUNCTION prevent_annotation_result_alias_mutation()
RETURNS trigger AS $$
BEGIN
    RAISE EXCEPTION 'annotation result alias history is append-only';
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_annotation_result_alias_immutable
BEFORE UPDATE OR DELETE ON annotation_result_alias
FOR EACH ROW EXECUTE FUNCTION prevent_annotation_result_alias_mutation();

-- Preserve all canonical aliases that predate this migration.
INSERT INTO annotation_result_alias(workspace_id, campaign_id, alias, result_id, created_at)
SELECT workspace_id, campaign_id, observation_key, id, created_at
  FROM annotation_result;

-- Every future Result insertion reserves its canonical alias in the same
-- transaction, including direct SQL/repository paths.
CREATE OR REPLACE FUNCTION append_annotation_result_canonical_alias()
RETURNS trigger AS $$
BEGIN
    INSERT INTO annotation_result_alias(
        workspace_id, campaign_id, alias, result_id, created_at
    ) VALUES (
        NEW.workspace_id, NEW.campaign_id, NEW.observation_key, NEW.id, NEW.created_at
    );
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_annotation_result_canonical_alias
AFTER INSERT ON annotation_result
FOR EACH ROW EXECUTE FUNCTION append_annotation_result_canonical_alias();
