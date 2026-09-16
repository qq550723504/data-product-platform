CREATE TABLE evidence_snapshot (
    id              uuid PRIMARY KEY,
    workspace_id    uuid NOT NULL,
    object_type     varchar(64) NOT NULL,
    object_id       uuid NOT NULL,
    manifest        jsonb NOT NULL,
    root_hash       varchar(64) NOT NULL,
    created_at      timestamptz NOT NULL DEFAULT now(),
    created_by      uuid
);

CREATE INDEX idx_evidence_snapshot_object
    ON evidence_snapshot(object_type, object_id, created_at DESC);

CREATE TABLE evidence_snapshot_item (
    snapshot_id     uuid NOT NULL REFERENCES evidence_snapshot(id),
    evidence_id     uuid NOT NULL REFERENCES evidence(id),
    category        varchar(64) NOT NULL,
    PRIMARY KEY(snapshot_id, evidence_id, category)
);

CREATE TABLE command_idempotency (
    id               uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id     uuid NOT NULL,
    command_type     varchar(128) NOT NULL,
    idempotency_key  varchar(255) NOT NULL,
    object_id        uuid NOT NULL,
    result_ref       uuid,
    created_at       timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT uq_command_idempotency UNIQUE(workspace_id, command_type, idempotency_key)
);

CREATE INDEX idx_command_idempotency_object
    ON command_idempotency(object_id, command_type);

ALTER TABLE product_release
    ADD CONSTRAINT fk_product_release_evidence_snapshot
    FOREIGN KEY (evidence_snapshot_id) REFERENCES evidence_snapshot(id);

CREATE OR REPLACE FUNCTION prevent_evidence_snapshot_mutation()
RETURNS trigger AS $$
BEGIN
    RAISE EXCEPTION 'evidence_snapshot is immutable';
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_evidence_snapshot_immutable_update
BEFORE UPDATE OR DELETE ON evidence_snapshot
FOR EACH ROW EXECUTE FUNCTION prevent_evidence_snapshot_mutation();
