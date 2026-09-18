-- Protect the EntityMapping decision chain introduced by 000013:
--
--   * entity_mapping.current_decision_id points at the immutable decision that
--     produced the current projection, so the mutable projection can never be
--     read without the decision that created it.
--   * entity_mapping_decision records the operation idempotency key and the
--     source association (job / candidate) that produced it.
--   * legacy decision rows that cannot prove a source are explicitly UNKNOWN.
--
-- Forward-only with respect to data: the migration blocks instead of repairing
-- when an existing mapping has no decision to point at. It never rewrites or
-- deletes the immutable decision history created by 000013.

ALTER TABLE entity_mapping_decision
    ADD COLUMN idempotency_key varchar(255),
    ADD COLUMN source_origin varchar(32) NOT NULL DEFAULT 'UNKNOWN',
    ADD COLUMN source_job_id uuid,
    ADD COLUMN source_candidate_id uuid;

ALTER TABLE entity_mapping_decision
    ADD CONSTRAINT ck_entity_mapping_decision_source_origin
        CHECK (source_origin IN ('UNKNOWN', 'MATCH_CANDIDATE', 'WORKFLOW_ALIAS')),
    -- Decisions backfilled by 000013 predate source tracking; their origin is
    -- unknown by definition and stays UNKNOWN. Nothing is re-derived.
    ADD CONSTRAINT fk_entity_mapping_decision_job
        FOREIGN KEY (source_job_id) REFERENCES entity_match_job(id),
    ADD CONSTRAINT fk_entity_mapping_decision_candidate
        FOREIGN KEY (source_candidate_id) REFERENCES entity_match_candidate(id);

-- One operation key appends at most one decision per workspace.
CREATE UNIQUE INDEX uq_entity_mapping_decision_idempotency
    ON entity_mapping_decision(workspace_id, idempotency_key)
    WHERE idempotency_key IS NOT NULL;

-- Referenced by the composite current-pointer foreign key below, so the pointer
-- can only ever name a decision that belongs to the same mapping.
ALTER TABLE entity_mapping_decision
    ADD CONSTRAINT uq_entity_mapping_decision_workspace_mapping_id
        UNIQUE (workspace_id, mapping_id, id);

ALTER TABLE entity_mapping
    ADD COLUMN current_decision_id uuid;

-- Backfill the pointer with the newest decision per mapping. decided_seq is the
-- authoritative insertion order because decided_at is caller-supplied and ties.
UPDATE entity_mapping em
SET current_decision_id = latest.id
FROM (
    SELECT DISTINCT ON (mapping_id) mapping_id, id
    FROM entity_mapping_decision
    ORDER BY mapping_id, decided_seq DESC
) latest
WHERE latest.mapping_id = em.id;

-- Block, do not invent: a mapping whose decision cannot be proven must be
-- resolved by an operator before this migration is applied.
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM entity_mapping WHERE current_decision_id IS NULL) THEN
        RAISE EXCEPTION 'entity_mapping contains a row without a decision; resolve the orphaned mapping before applying 000014';
    END IF;
END $$;

ALTER TABLE entity_mapping
    ALTER COLUMN current_decision_id SET NOT NULL;

-- The pointer must name a decision of this exact workspace and mapping. The
-- constraint is deferred so a decision can be inserted after the projection row
-- that points at it inside one transaction.
ALTER TABLE entity_mapping
    ADD CONSTRAINT fk_entity_mapping_current_decision
        FOREIGN KEY (workspace_id, id, current_decision_id)
        REFERENCES entity_mapping_decision(workspace_id, mapping_id, id)
        DEFERRABLE INITIALLY DEFERRED;
