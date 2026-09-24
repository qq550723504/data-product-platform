-- Gold production bindings are immutable proof. Refuse destructive rollback once facts exist.
LOCK TABLE gold_build_request, gold_production_binding, gold_production_member IN ACCESS EXCLUSIVE MODE;

DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM gold_build_request LIMIT 1)
       OR EXISTS (SELECT 1 FROM gold_production_binding LIMIT 1)
       OR EXISTS (SELECT 1 FROM gold_production_member LIMIT 1) THEN
        RAISE EXCEPTION 'refusing destructive rollback of Gold production binding history';
    END IF;
END
$$;

DROP TRIGGER IF EXISTS trg_gold_binding_require_finalized ON gold_production_binding;
DROP FUNCTION IF EXISTS require_gold_binding_finalized_on_commit();
DROP TRIGGER IF EXISTS trg_gold_binding_delete ON gold_production_binding;
DROP FUNCTION IF EXISTS prevent_gold_binding_delete();
DROP TRIGGER IF EXISTS trg_gold_binding_update ON gold_production_binding;
DROP FUNCTION IF EXISTS guard_gold_binding_update();
DROP TRIGGER IF EXISTS trg_gold_member_delete ON gold_production_member;
DROP TRIGGER IF EXISTS trg_gold_member_update ON gold_production_member;
DROP FUNCTION IF EXISTS prevent_gold_member_mutation();
DROP TRIGGER IF EXISTS trg_gold_member_insert ON gold_production_member;
DROP FUNCTION IF EXISTS guard_gold_member_insert();
DROP TRIGGER IF EXISTS trg_gold_binding_insert ON gold_production_binding;
DROP FUNCTION IF EXISTS guard_gold_binding_insert();
DROP TABLE IF EXISTS gold_production_member;
DROP TABLE IF EXISTS gold_production_binding;
DROP TRIGGER IF EXISTS trg_gold_build_request_immutable ON gold_build_request;
DROP FUNCTION IF EXISTS prevent_gold_build_request_mutation();
DROP TRIGGER IF EXISTS trg_gold_build_request_insert ON gold_build_request;
DROP FUNCTION IF EXISTS guard_gold_build_request_insert();
DROP TABLE IF EXISTS gold_build_request;
