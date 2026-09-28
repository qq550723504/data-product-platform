-- history_revision is immutable ordering metadata. Once certification history
-- exists, dropping it would make a later re-upgrade unable to reconstruct the
-- original committed pagination boundary.
DO $$
BEGIN
    LOCK TABLE dataset_certification, certification_disposition
        IN ACCESS EXCLUSIVE MODE;
    IF EXISTS (SELECT 1 FROM dataset_certification)
       OR EXISTS (SELECT 1 FROM certification_disposition) THEN
        RAISE EXCEPTION 'refusing to downgrade certification history pagination metadata';
    END IF;
END;
$$;

DROP INDEX idx_dataset_certification_history_page;

ALTER TABLE certification_disposition
    DROP COLUMN history_revision;

ALTER TABLE dataset_certification
    DROP COLUMN history_revision;
