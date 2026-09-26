LOCK TABLE quality_assessment_attempt IN ACCESS EXCLUSIVE MODE;

DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM quality_assessment_attempt
        WHERE engine_name <> 'native-quality'
    ) THEN
        RAISE EXCEPTION 'cannot drop quality assessment engine identity while non-native attempt history exists';
    END IF;
END;
$$;

ALTER TABLE quality_assessment_attempt
    DROP COLUMN IF EXISTS engine_name;
