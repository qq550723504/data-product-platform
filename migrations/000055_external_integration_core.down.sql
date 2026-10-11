-- Lock before checking so a concurrent writer cannot create history after check.
LOCK TABLE external_identity_binding,external_identity_binding_disposition,
  external_identifier,external_identifier_disposition,external_publication,
  external_publication_transition IN ACCESS EXCLUSIVE MODE;
DO $$ BEGIN
  IF EXISTS(SELECT 1 FROM external_identity_binding) OR EXISTS(SELECT 1 FROM external_identifier) OR
     EXISTS(SELECT 1 FROM external_publication) OR EXISTS(SELECT 1 FROM external_publication_transition) OR
     EXISTS(SELECT 1 FROM external_identity_binding_disposition) OR EXISTS(SELECT 1 FROM external_identifier_disposition) THEN
    RAISE EXCEPTION 'cannot drop external integration historical facts';
  END IF;
END $$;
DROP TABLE external_publication_transition,external_publication,external_identifier_disposition,
  external_identifier,external_identity_binding_disposition,external_identity_binding;
DROP FUNCTION check_external_projection_history(),guard_external_transition(),
  guard_external_publication_projection(),guard_external_reference(),guard_external_immutable();
