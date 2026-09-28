-- #161 Stable, bounded DatasetCertification history pagination.
--
-- Runtime certification/disposition writes already advance the workspace delivery
-- authorization fence before inserting immutable history. Persist that committed
-- revision on each new fact so paginated reads can freeze a membership boundary
-- that is not vulnerable to pre-commit issued_at/effective_at timestamps.
ALTER TABLE dataset_certification
    ADD COLUMN history_revision bigint NOT NULL DEFAULT 0;

ALTER TABLE certification_disposition
    ADD COLUMN history_revision bigint NOT NULL DEFAULT 0;

CREATE INDEX idx_dataset_certification_history_page
    ON dataset_certification(workspace_id, dataset_version_id, issued_at DESC, id DESC)
    INCLUDE (history_revision, certification_profile_id);
