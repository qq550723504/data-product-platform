-- Retained synthetic fixtures need fresh, bounded authority without rewriting
-- the old policy or creating a new content version. One explicit immutable chain.
ALTER TABLE lifecycle_copy_policy DROP CONSTRAINT lifecycle_copy_policy_workspace_id_dataset_version_id_key;
ALTER TABLE lifecycle_copy_policy ADD COLUMN supersedes_policy_id uuid;
ALTER TABLE lifecycle_copy_policy ADD CONSTRAINT lifecycle_copy_policy_exact_identity UNIQUE(workspace_id,dataset_version_id,id);
ALTER TABLE lifecycle_copy_policy ADD CONSTRAINT lifecycle_copy_policy_previous_fk
 FOREIGN KEY(workspace_id,dataset_version_id,supersedes_policy_id) REFERENCES lifecycle_copy_policy(workspace_id,dataset_version_id,id);
CREATE UNIQUE INDEX lifecycle_copy_policy_one_root ON lifecycle_copy_policy(workspace_id,dataset_version_id) WHERE supersedes_policy_id IS NULL;
CREATE UNIQUE INDEX lifecycle_copy_policy_one_successor ON lifecycle_copy_policy(supersedes_policy_id) WHERE supersedes_policy_id IS NOT NULL;

CREATE FUNCTION lifecycle_copy_policy_renewal_guard() RETURNS trigger AS $$
DECLARE previous lifecycle_copy_policy;
BEGIN
 IF NEW.supersedes_policy_id IS NOT NULL THEN
  PERFORM lifecycle_reference_lock(NEW.workspace_id,NEW.dataset_version_id);
  SELECT * INTO previous FROM lifecycle_copy_policy WHERE workspace_id=NEW.workspace_id AND id=NEW.supersedes_policy_id FOR UPDATE;
  IF NOT FOUND OR previous.dataset_version_id<>NEW.dataset_version_id OR previous.created_at>=NEW.created_at
   OR NEW.use_until>NEW.created_at+interval '1 hour' OR NEW.restore_until>NEW.created_at+interval '1 hour'
  THEN RAISE EXCEPTION 'invalid immutable fixture policy successor'; END IF;
  IF EXISTS(SELECT 1 FROM lifecycle_copy_policy p WHERE p.supersedes_policy_id=previous.id)
   OR EXISTS(SELECT 1 FROM lifecycle_content_use_restriction r WHERE r.workspace_id=NEW.workspace_id AND r.dataset_version_id=NEW.dataset_version_id)
   OR EXISTS(SELECT 1 FROM lifecycle_fixture_copy c WHERE c.workspace_id=NEW.workspace_id AND c.dataset_version_id=NEW.dataset_version_id AND (
    EXISTS(SELECT 1 FROM lifecycle_reader_pin p WHERE p.copy_id=c.id AND NOT EXISTS(SELECT 1 FROM lifecycle_reader_termination t WHERE t.pin_id=p.id))
    OR EXISTS(SELECT 1 FROM lifecycle_content_hold h WHERE h.copy_id=c.id AND NOT EXISTS(SELECT 1 FROM lifecycle_content_hold_release r WHERE r.hold_id=h.id))
    OR EXISTS(SELECT 1 FROM lifecycle_disposition_dispatch d WHERE d.copy_id=c.id AND NOT EXISTS(SELECT 1 FROM lifecycle_dispatch_release r WHERE r.dispatch_id=d.id))
    OR EXISTS(SELECT 1 FROM lifecycle_copy_operation o WHERE (o.source_copy_id=c.id OR o.target_copy_id=c.id) AND NOT EXISTS(SELECT 1 FROM lifecycle_copy_verification v WHERE v.operation_id=o.id))))
   OR EXISTS(SELECT 1 FROM lifecycle_processing_output_intent i WHERE i.workspace_id=NEW.workspace_id AND NOT EXISTS(SELECT 1 FROM lifecycle_processing_output_ready r WHERE r.intent_id=i.id) AND (i.dataset_version_id=NEW.dataset_version_id OR EXISTS(SELECT 1 FROM lifecycle_fixture_copy c WHERE c.id=i.source_copy_id AND c.dataset_version_id=NEW.dataset_version_id) OR EXISTS(SELECT 1 FROM execution_input x WHERE x.execution_id=i.execution_id AND x.dataset_version_id=NEW.dataset_version_id)))
  THEN RAISE EXCEPTION 'fixture policy renewal is not quiescent/current'; END IF;
 END IF;
 RETURN NEW;
END;
$$ LANGUAGE plpgsql;
CREATE TRIGGER lifecycle_copy_policy_renewal BEFORE INSERT ON lifecycle_copy_policy FOR EACH ROW EXECUTE FUNCTION lifecycle_copy_policy_renewal_guard();

CREATE VIEW lifecycle_current_copy_policy AS
 SELECT p.* FROM lifecycle_copy_policy p WHERE NOT EXISTS(SELECT 1 FROM lifecycle_copy_policy next WHERE next.supersedes_policy_id=p.id);
