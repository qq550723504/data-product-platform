-- Runtime is an opaque, secret-free Adapter extension; Core freezes its hash.
-- Existing terminal facts are not rewritten or given invented launch contracts.
CREATE TABLE source_schema_attempt_runtime (
 attempt_id uuid PRIMARY KEY REFERENCES source_schema_attempt(id),
 contract jsonb NOT NULL CHECK(jsonb_typeof(contract)='object' AND octet_length(contract::text)<=16384),
 contract_hash char(64) NOT NULL CHECK(contract_hash ~ '^[0-9a-f]{64}$'),
 recorded_at timestamptz NOT NULL DEFAULT clock_timestamp()
);
CREATE TRIGGER source_schema_attempt_runtime_fact BEFORE UPDATE OR DELETE ON source_schema_attempt_runtime
 FOR EACH ROW EXECUTE FUNCTION source_schema_fact_guard();
CREATE FUNCTION require_source_schema_attempt_runtime() RETURNS trigger AS $$
BEGIN
 IF NOT EXISTS(SELECT 1 FROM source_schema_attempt_runtime WHERE attempt_id=NEW.id)
 THEN RAISE EXCEPTION 'source schema attempt requires frozen runtime contract'; END IF;
 RETURN NULL;
END; $$ LANGUAGE plpgsql;
CREATE CONSTRAINT TRIGGER source_schema_attempt_runtime_required AFTER INSERT ON source_schema_attempt
 DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION require_source_schema_attempt_runtime();
