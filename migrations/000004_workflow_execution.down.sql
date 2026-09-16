DROP TRIGGER IF EXISTS trg_workflow_version_immutable_delete ON workflow_version;
DROP TRIGGER IF EXISTS trg_workflow_version_immutable_update ON workflow_version;
DROP FUNCTION IF EXISTS prevent_workflow_version_mutation();
DROP TABLE IF EXISTS cost_event;
DROP TABLE IF EXISTS execution_quarantine_record;
DROP TABLE IF EXISTS execution_input;
DROP TABLE IF EXISTS execution;
DROP TABLE IF EXISTS workflow_version;
DROP TABLE IF EXISTS workflow;
