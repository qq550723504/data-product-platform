LOCK TABLE acceptance_party_representation,acceptance_responsibility_binding,acceptance_standard_applicability,acceptance_binding_disposition IN ACCESS EXCLUSIVE MODE;
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM acceptance_party_representation) OR EXISTS(SELECT 1 FROM acceptance_responsibility_binding) OR EXISTS(SELECT 1 FROM acceptance_standard_applicability) OR EXISTS(SELECT 1 FROM acceptance_binding_disposition) THEN RAISE EXCEPTION 'cannot drop acceptance binding historical facts'; END IF;
END $$;
DROP TABLE acceptance_binding_disposition,acceptance_standard_applicability,acceptance_responsibility_binding,acceptance_party_representation;
DROP FUNCTION guard_acceptance_binding_fact();
