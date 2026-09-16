CREATE OR REPLACE FUNCTION prevent_evidence_history_mutation()
RETURNS trigger AS $$
BEGIN
    RAISE EXCEPTION 'evidence history is immutable';
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_evidence_immutable
BEFORE UPDATE OR DELETE ON evidence
FOR EACH ROW EXECUTE FUNCTION prevent_evidence_history_mutation();

CREATE TRIGGER trg_evidence_relation_immutable
BEFORE UPDATE OR DELETE ON evidence_relation
FOR EACH ROW EXECUTE FUNCTION prevent_evidence_history_mutation();

CREATE OR REPLACE FUNCTION prevent_cost_history_mutation()
RETURNS trigger AS $$
BEGIN
    RAISE EXCEPTION 'cost history is immutable';
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_cost_event_immutable
BEFORE UPDATE OR DELETE ON cost_event
FOR EACH ROW EXECUTE FUNCTION prevent_cost_history_mutation();

CREATE OR REPLACE FUNCTION prevent_audit_history_mutation()
RETURNS trigger AS $$
BEGIN
    RAISE EXCEPTION 'audit history is immutable';
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_audit_event_immutable
BEFORE UPDATE OR DELETE ON audit_event
FOR EACH ROW EXECUTE FUNCTION prevent_audit_history_mutation();
