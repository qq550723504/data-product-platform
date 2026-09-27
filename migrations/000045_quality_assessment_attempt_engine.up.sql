ALTER TABLE quality_assessment_attempt
    ADD COLUMN engine_name varchar(128),
    ADD COLUMN engine_version varchar(64);

-- Migration 23 made attempt rows append-only. This migration is the one
-- controlled schema evolution that must enrich those historical rows with
-- engine identity. Keep the trigger disabled only for this bounded backfill.
ALTER TABLE quality_assessment_attempt
    DISABLE TRIGGER trg_quality_assessment_attempt_immutable;

-- Successful attempts already point at the immutable QualityAssessment that
-- records the exact evaluator identity. Recover from that authoritative fact
-- instead of guessing from the current Native evaluator version.
UPDATE quality_assessment_attempt AS attempt
SET engine_name = result.evaluator_name,
    engine_version = result.evaluator_version
FROM quality_assessment_attempt_outcome AS outcome
JOIN quality_result AS result
  ON result.id = outcome.assessment_id
WHERE outcome.attempt_id = attempt.id
  AND outcome.outcome = 'SUCCEEDED'
  AND outcome.assessment_id IS NOT NULL;

-- Gold has had a fixed evaluator identity since this attempt table existed.
-- Failed/in-progress Gold attempts have no QualityAssessment to join to, but
-- their frozen rule-set reference proves the engine identity.
UPDATE quality_assessment_attempt
SET engine_name = 'gold-quality',
    engine_version = '1'
WHERE engine_name IS NULL
  AND rule_set_ref = 'gold/quality/annotation-v1';

-- Before the provider-neutral engine port, every remaining attempt was Native,
-- but failed/in-progress legacy rows do not carry enough immutable evidence to
-- prove whether they ran Native v1 or v2. Preserve that uncertainty instead of
-- falsifying history. A replay under a concrete current version will conflict.
UPDATE quality_assessment_attempt
SET engine_name = 'native-quality',
    engine_version = 'legacy-unknown'
WHERE engine_name IS NULL;

ALTER TABLE quality_assessment_attempt
    ENABLE TRIGGER trg_quality_assessment_attempt_immutable;

DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM quality_assessment_attempt
        WHERE engine_name IS NULL OR engine_version IS NULL
    ) THEN
        RAISE EXCEPTION 'quality assessment attempt engine backfill is incomplete';
    END IF;
END;
$$;

ALTER TABLE quality_assessment_attempt
    ALTER COLUMN engine_name SET NOT NULL,
    ALTER COLUMN engine_name SET DEFAULT 'native-quality',
    ALTER COLUMN engine_version SET NOT NULL,
    ALTER COLUMN engine_version SET DEFAULT '2';
