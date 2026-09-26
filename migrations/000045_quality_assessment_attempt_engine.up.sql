ALTER TABLE quality_assessment_attempt
    ADD COLUMN engine_name varchar(128) NOT NULL DEFAULT 'native-quality';
