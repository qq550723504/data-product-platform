-- Serialize with all fixture/plan writers before checking for historical facts.
LOCK TABLE lifecycle_fixture_namespace, lifecycle_fixture_copy, lifecycle_disposition_plan,
           lifecycle_disposition_plan_target, lifecycle_content_use_restriction, lifecycle_content_hold,
           lifecycle_content_hold_release, lifecycle_reader_pin, lifecycle_reader_termination IN ACCESS EXCLUSIVE MODE;
DO $$ BEGIN
    IF EXISTS (SELECT 1 FROM lifecycle_fixture_namespace) OR
       EXISTS (SELECT 1 FROM lifecycle_fixture_copy) OR EXISTS (SELECT 1 FROM lifecycle_disposition_plan) THEN
        RAISE EXCEPTION 'cannot remove lifecycle fixture or disposition history';
    END IF;
END $$;
DROP TABLE lifecycle_reader_termination;
DROP TABLE lifecycle_reader_pin;
DROP TABLE lifecycle_content_hold_release;
DROP TABLE lifecycle_content_hold;
DROP TABLE lifecycle_content_use_restriction;
DROP FUNCTION lifecycle_termination_scope_guard();
DROP FUNCTION lifecycle_hold_release_scope_guard();
DROP FUNCTION lifecycle_restriction_scope_guard();
DROP TABLE lifecycle_disposition_plan_target;
DROP TABLE lifecycle_disposition_plan;
DROP TABLE lifecycle_fixture_copy;
DROP TABLE lifecycle_fixture_namespace;
DROP FUNCTION lifecycle_plan_finalize_guard();
DROP FUNCTION lifecycle_plan_scope_hash(uuid);
DROP FUNCTION lifecycle_plan_frame(text);
DROP FUNCTION lifecycle_plan_target_guard();
DROP FUNCTION lifecycle_fixture_identity_immutable();
DROP FUNCTION lifecycle_fixture_copy_workspace_guard();
