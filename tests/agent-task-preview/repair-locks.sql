-- Preview-only lock privileges; never a production migration.
BEGIN;
CREATE FUNCTION agent_q2_reject_owner_mutation() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF current_user='agent_q2_planner' THEN RAISE EXCEPTION 'preview owner mutation denied'; END IF;
 IF TG_OP='DELETE' THEN RETURN OLD; END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER agent_q2_no_entity_mutation BEFORE UPDATE OR DELETE ON entity_match_job FOR EACH ROW EXECUTE FUNCTION agent_q2_reject_owner_mutation();
-- PostgreSQL row locks require UPDATE on at least one column. Neither grant
-- permits a business mutation: case already immutable, job protected above.
GRANT UPDATE(id) ON quality_remediation_case,entity_match_job TO agent_q2_planner;
COMMIT;
