DROP INDEX idx_dataset_certification_history_page;

ALTER TABLE certification_disposition
    DROP COLUMN history_revision;

ALTER TABLE dataset_certification
    DROP COLUMN history_revision;
