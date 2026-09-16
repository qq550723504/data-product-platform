DROP TRIGGER IF EXISTS trg_audit_event_immutable ON audit_event;
DROP FUNCTION IF EXISTS prevent_audit_history_mutation();

DROP TRIGGER IF EXISTS trg_cost_event_immutable ON cost_event;
DROP FUNCTION IF EXISTS prevent_cost_history_mutation();

DROP TRIGGER IF EXISTS trg_evidence_relation_immutable ON evidence_relation;
DROP TRIGGER IF EXISTS trg_evidence_immutable ON evidence;
DROP FUNCTION IF EXISTS prevent_evidence_history_mutation();
