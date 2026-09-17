-- Scope EntityMapping to a workspace and separate the mutable "current mapping"
-- projection from the immutable decision history that produced it.
--
-- Before this migration the only uniqueness rule was global:
--   UNIQUE(source_type, source_ref, source_key)
-- so two workspaces could not both map the same external source key, and a
-- lookup by source could resolve to another workspace's mapping.
--
-- This migration is forward-only with respect to data. When it cannot prove a
-- row's workspace, or when scoping would expose a conflict, it aborts and asks
-- an operator to resolve it. It never repairs, deduplicates or deletes rows.

-- Referenced by the composite workspace-consistency foreign keys below.
ALTER TABLE entity
    ADD CONSTRAINT uq_entity_workspace_id UNIQUE (workspace_id, id);

ALTER TABLE entity_mapping
    ADD COLUMN workspace_id uuid;

UPDATE entity_mapping em
SET workspace_id = e.workspace_id
FROM entity e
WHERE e.id = em.entity_id;

-- Block, do not repair: every mapping must resolve to exactly one workspace.
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM entity_mapping WHERE workspace_id IS NULL) THEN
        RAISE EXCEPTION 'entity_mapping backfill left rows without a workspace; resolve the orphaned entity references before applying 000013';
    END IF;
END $$;

ALTER TABLE entity_mapping
    ALTER COLUMN workspace_id SET NOT NULL;

-- Block, do not deduplicate: the old global key makes a conflict impossible
-- today, but if one exists an operator must decide which mapping wins.
DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM entity_mapping
        GROUP BY workspace_id, source_type, source_ref, source_key
        HAVING count(*) > 1
    ) THEN
        RAISE EXCEPTION 'entity_mapping contains duplicate source triples inside one workspace; resolve them before applying 000013';
    END IF;
END $$;

ALTER TABLE entity_mapping
    DROP CONSTRAINT uq_entity_mapping_source,
    ADD CONSTRAINT uq_entity_mapping_workspace_source UNIQUE (workspace_id, source_type, source_ref, source_key),
    -- Referenced by entity_mapping_decision so a decision can never point at a
    -- mapping from another workspace.
    ADD CONSTRAINT uq_entity_mapping_workspace_id UNIQUE (workspace_id, id),
    -- Database-level workspace consistency: the mapping and the entity it points
    -- at must live in the same workspace.
    ADD CONSTRAINT fk_entity_mapping_entity FOREIGN KEY (workspace_id, entity_id) REFERENCES entity(workspace_id, id);

CREATE INDEX idx_entity_mapping_workspace_entity ON entity_mapping(workspace_id, entity_id);

-- Current mapping vs immutable decision history. entity_mapping stays the
-- updatable projection used by lookups; entity_mapping_decision appends one
-- immutable row for every accepted mapping decision.
CREATE TABLE entity_mapping_decision (
    id                    uuid PRIMARY KEY,
    -- Monotonic insertion order. decided_at is caller-supplied and can tie, so
    -- the sequence is the authoritative tie-breaker for history reads.
    decided_seq           bigint GENERATED ALWAYS AS IDENTITY,
    workspace_id          uuid NOT NULL,
    mapping_id            uuid NOT NULL,
    entity_id             uuid NOT NULL,
    source_type           varchar(64) NOT NULL,
    source_ref            varchar(512) NOT NULL,
    source_key            varchar(512) NOT NULL,
    source_name           varchar(512),
    match_method          varchar(64) NOT NULL,
    match_rule_id         varchar(128),
    match_policy_version  varchar(64) NOT NULL,
    match_engine_name     varchar(64) NOT NULL,
    match_engine_version  varchar(64) NOT NULL,
    match_model_version   varchar(128),
    confidence            numeric(6,5),
    status                varchar(32) NOT NULL,
    reviewed_by           uuid,
    reviewed_at           timestamptz,
    reviewer_reason       text,
    evidence_id           uuid REFERENCES evidence(id),
    decided_at            timestamptz NOT NULL DEFAULT now(),
    decided_by            uuid,
    CONSTRAINT ck_entity_mapping_decision_status CHECK(status IN ('AUTO_MATCHED', 'CONFIRMED', 'REJECTED', 'CONFLICT')),
    CONSTRAINT fk_entity_mapping_decision_mapping FOREIGN KEY (workspace_id, mapping_id) REFERENCES entity_mapping(workspace_id, id),
    CONSTRAINT fk_entity_mapping_decision_entity FOREIGN KEY (workspace_id, entity_id) REFERENCES entity(workspace_id, id)
);

CREATE INDEX idx_entity_mapping_decision_mapping ON entity_mapping_decision(mapping_id, decided_seq);
CREATE INDEX idx_entity_mapping_decision_source ON entity_mapping_decision(workspace_id, source_type, source_ref, source_key);

-- Backfill one decision per existing current mapping. This records the row as it
-- exists at migration time; it does not re-derive or invent provenance.
INSERT INTO entity_mapping_decision (
    id, workspace_id, mapping_id, entity_id, source_type, source_ref, source_key,
    source_name, match_method, match_rule_id, match_policy_version,
    match_engine_name, match_engine_version, match_model_version, confidence,
    status, reviewed_by, reviewed_at, reviewer_reason, evidence_id, decided_at, decided_by
)
SELECT gen_random_uuid(), em.workspace_id, em.id, em.entity_id, em.source_type, em.source_ref, em.source_key,
       em.source_name, em.match_method, em.match_rule_id, em.match_policy_version,
       em.match_engine_name, em.match_engine_version, em.match_model_version, em.confidence,
       em.status, em.reviewed_by, em.reviewed_at, em.reviewer_reason, em.evidence_id,
       em.created_at, em.reviewed_by
FROM entity_mapping em
ORDER BY em.created_at, em.id;

-- Decision history is append-only: AGENTS.md §3 forbids overwriting history.
CREATE OR REPLACE FUNCTION prevent_entity_mapping_decision_mutation() RETURNS trigger AS $$
BEGIN
    RAISE EXCEPTION 'entity_mapping_decision is immutable; append a new decision instead';
END $$ LANGUAGE plpgsql;

CREATE TRIGGER prevent_entity_mapping_decision_mutation
    BEFORE UPDATE OR DELETE ON entity_mapping_decision
    FOR EACH ROW EXECUTE FUNCTION prevent_entity_mapping_decision_mutation();
