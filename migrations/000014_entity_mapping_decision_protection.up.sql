-- Protect the EntityMapping decision chain introduced by 000013:
--
--   * entity_mapping.current_decision_id points at the immutable decision that
--     produced the current projection, so the mutable projection can never be
--     read without the decision that created it.
--   * entity_mapping_decision records the operation idempotency key and the
--     source association (job / candidate) that produced it.
--   * every decision has an explicit current provenance and idempotency key.
--
-- This pre-production schema assumes no legacy decision rows need preserving.

ALTER TABLE entity_mapping_decision
    ADD COLUMN idempotency_key varchar(255) NOT NULL,
    ADD COLUMN source_origin varchar(32) NOT NULL,
    ADD COLUMN source_job_id uuid,
    ADD COLUMN source_candidate_id uuid;

ALTER TABLE entity_mapping_decision
    ADD CONSTRAINT ck_entity_mapping_decision_source_origin
        CHECK (source_origin IN ('MATCH_CANDIDATE', 'WORKFLOW_ALIAS')),
    ADD CONSTRAINT fk_entity_mapping_decision_job
        FOREIGN KEY (source_job_id) REFERENCES entity_match_job(id),
    ADD CONSTRAINT fk_entity_mapping_decision_candidate
        FOREIGN KEY (source_candidate_id) REFERENCES entity_match_candidate(id);

-- One operation key appends at most one decision per workspace.
CREATE UNIQUE INDEX uq_entity_mapping_decision_idempotency
    ON entity_mapping_decision(workspace_id, idempotency_key);

-- Referenced by the composite current-pointer foreign key below, so the pointer
-- can only ever name a decision that belongs to the same mapping.
ALTER TABLE entity_mapping_decision
    ADD CONSTRAINT uq_entity_mapping_decision_workspace_mapping_id
        UNIQUE (workspace_id, mapping_id, id);

ALTER TABLE entity_mapping
    ADD COLUMN current_decision_id uuid NOT NULL;

-- The pointer must name a decision of this exact workspace and mapping. The
-- constraint is deferred so a decision can be inserted after the projection row
-- that points at it inside one transaction.
ALTER TABLE entity_mapping
    ADD CONSTRAINT fk_entity_mapping_current_decision
        FOREIGN KEY (workspace_id, id, current_decision_id)
        REFERENCES entity_mapping_decision(workspace_id, mapping_id, id)
        DEFERRABLE INITIALLY DEFERRED;
