DROP TRIGGER IF EXISTS trg_compliance_result_immutable_update ON compliance_result;
DROP FUNCTION IF EXISTS prevent_compliance_result_mutation();
DROP TRIGGER IF EXISTS trg_quality_result_immutable_update ON quality_result;
DROP FUNCTION IF EXISTS prevent_quality_result_mutation();

ALTER TABLE IF EXISTS product_release DROP CONSTRAINT IF EXISTS fk_product_release_compliance_result;
ALTER TABLE IF EXISTS product_release DROP CONSTRAINT IF EXISTS fk_product_release_quality_result;

DROP TABLE IF EXISTS compliance_finding;
DROP TABLE IF EXISTS compliance_result;
DROP TABLE IF EXISTS quality_finding;
DROP TABLE IF EXISTS quality_result;
