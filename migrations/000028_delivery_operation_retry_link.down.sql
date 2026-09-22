CREATE OR REPLACE FUNCTION guard_delivery_operation_mutation()
RETURNS trigger AS $$
BEGIN
    IF TG_OP = 'DELETE' THEN
        RAISE EXCEPTION 'delivery_operation is historical and cannot be deleted';
    END IF;
    IF OLD.workspace_id IS DISTINCT FROM NEW.workspace_id OR
       OLD.dataset_version_id IS DISTINCT FROM NEW.dataset_version_id OR
       OLD.certification_ref IS DISTINCT FROM NEW.certification_ref OR
       OLD.idempotency_key IS DISTINCT FROM NEW.idempotency_key OR
       OLD.provider_name IS DISTINCT FROM NEW.provider_name OR
       OLD.provider_request_key IS DISTINCT FROM NEW.provider_request_key OR
       OLD.principal_ref IS DISTINCT FROM NEW.principal_ref OR
       OLD.effective_consumer_ref IS DISTINCT FROM NEW.effective_consumer_ref OR
       OLD.delegation_ref IS DISTINCT FROM NEW.delegation_ref OR
       OLD.purpose IS DISTINCT FROM NEW.purpose OR
       OLD.action IS DISTINCT FROM NEW.action OR
       OLD.scope_ref IS DISTINCT FROM NEW.scope_ref OR
       OLD.delivery_channel IS DISTINCT FROM NEW.delivery_channel OR
       OLD.delivery_mode IS DISTINCT FROM NEW.delivery_mode OR
       OLD.requested_expires_at IS DISTINCT FROM NEW.requested_expires_at OR
       OLD.created_at IS DISTINCT FROM NEW.created_at THEN
        RAISE EXCEPTION 'delivery_operation request identity is immutable';
    END IF;
    IF OLD.status IN ('ISSUED','BLOCKED','FAILED') AND NEW.status IS DISTINCT FROM OLD.status THEN
        RAISE EXCEPTION 'terminal delivery_operation status is immutable';
    END IF;
    IF OLD.status = 'ISSUED' AND (
       OLD.credential_ref IS DISTINCT FROM NEW.credential_ref OR
       OLD.credential_hash IS DISTINCT FROM NEW.credential_hash OR
       OLD.provider_credential_expires_at IS DISTINCT FROM NEW.provider_credential_expires_at
    ) THEN
        RAISE EXCEPTION 'issued delivery capability is immutable';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

LOCK TABLE delivery_operation, delivery_gate_evaluation IN ACCESS EXCLUSIVE MODE;

DO $rollback$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM delivery_operation
        WHERE retry_of_delivery_operation_id IS NOT NULL
    ) THEN
        RAISE EXCEPTION 'cannot rollback delivery retry link migration: immutable retry history exists';
    END IF;
    IF EXISTS (
        SELECT 1
        FROM delivery_gate_evaluation
        WHERE certification_profile_id IS NOT NULL
    ) THEN
        RAISE EXCEPTION 'cannot rollback delivery profile attribution: immutable gate history exists';
    END IF;
END;
$rollback$;

DROP INDEX IF EXISTS idx_delivery_operation_retry_of;
ALTER TABLE delivery_operation DROP COLUMN IF EXISTS retry_of_delivery_operation_id;
ALTER TABLE delivery_gate_evaluation DROP COLUMN IF EXISTS certification_profile_id;
