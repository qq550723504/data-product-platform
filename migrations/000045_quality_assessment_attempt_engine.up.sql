ALTER TABLE quality_assessment_attempt
    ADD COLUMN engine_name varchar(128),
    ADD COLUMN engine_version varchar(64);

UPDATE quality_assessment_attempt
SET engine_name='gold-quality',
    engine_version='1'
WHERE rule_set_ref='gold/quality/annotation-v1';

UPDATE quality_assessment_attempt
SET engine_name='native-quality',
    engine_version='2'
WHERE engine_name IS NULL;

ALTER TABLE quality_assessment_attempt
    ALTER COLUMN engine_name SET NOT NULL,
    ALTER COLUMN engine_name SET DEFAULT 'native-quality',
    ALTER COLUMN engine_version SET NOT NULL,
    ALTER COLUMN engine_version SET DEFAULT '2';
