LOCK TABLE delivery_operation, delivery_gate_evaluation, delivery_transition,
    delivery_provider_attempt, delivery_provider_observation,
    delivery_credential_replay_decision, delivery_containment,
    delivery_containment_transition, cost_allocation, cost_event IN ACCESS EXCLUSIVE MODE;

DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM delivery_operation LIMIT 1)
       OR EXISTS (SELECT 1 FROM delivery_gate_evaluation LIMIT 1)
       OR EXISTS (SELECT 1 FROM delivery_transition LIMIT 1)
       OR EXISTS (SELECT 1 FROM delivery_provider_attempt LIMIT 1)
       OR EXISTS (SELECT 1 FROM delivery_provider_observation LIMIT 1)
       OR EXISTS (SELECT 1 FROM delivery_credential_replay_decision LIMIT 1)
       OR EXISTS (SELECT 1 FROM delivery_containment LIMIT 1)
       OR EXISTS (SELECT 1 FROM delivery_containment_transition LIMIT 1)
       OR EXISTS (SELECT 1 FROM cost_allocation LIMIT 1)
       OR EXISTS (SELECT 1 FROM cost_event WHERE activity_id IS NOT NULL LIMIT 1) THEN
        RAISE EXCEPTION 'refusing destructive rollback of delivery history';
    END IF;
END
$$;

DROP TRIGGER IF EXISTS trg_delivery_operation_guard ON delivery_operation;
DROP FUNCTION IF EXISTS guard_delivery_operation_mutation();
DROP TRIGGER IF EXISTS trg_delivery_replay_decision_immutable ON delivery_credential_replay_decision;
DROP TRIGGER IF EXISTS trg_delivery_containment_transition_immutable ON delivery_containment_transition;
DROP TRIGGER IF EXISTS trg_delivery_provider_observation_immutable ON delivery_provider_observation;
DROP TRIGGER IF EXISTS trg_delivery_provider_attempt_immutable ON delivery_provider_attempt;
DROP TRIGGER IF EXISTS trg_delivery_transition_immutable ON delivery_transition;
DROP TRIGGER IF EXISTS trg_delivery_gate_evaluation_immutable ON delivery_gate_evaluation;
DROP FUNCTION IF EXISTS prevent_delivery_history_mutation();

DROP TABLE cost_allocation;
DROP TABLE delivery_credential_replay_decision;
DROP TABLE delivery_containment_transition;
DROP TABLE delivery_containment;
DROP TABLE delivery_provider_observation;
DROP TABLE delivery_provider_attempt;
DROP TABLE delivery_transition;
DROP TABLE delivery_gate_evaluation;
DROP TABLE delivery_operation;
DROP TABLE delivery_authorization_fence;

DROP INDEX uq_cost_event_activity_component;
ALTER TABLE cost_event DROP COLUMN activity_id;
