-- A confirmed denied send remains a real invo­cation. After a fresh exact HEAD
-- proves the original version still present, a new independently gated physical
-- attempt may be admitted. UNKNOWN/start-only attempts are never released here.
ALTER TABLE lifecycle_dispatch_release ADD COLUMN verification_call_id uuid;
ALTER TABLE lifecycle_dispatch_release ADD CONSTRAINT lifecycle_release_verification_fk FOREIGN KEY(workspace_id,verification_call_id) REFERENCES lifecycle_storage_call(workspace_id,id);
CREATE OR REPLACE FUNCTION lifecycle_dispatch_release_guard() RETURNS trigger AS $$
BEGIN
 IF EXISTS(SELECT 1 FROM lifecycle_provider_observation o JOIN lifecycle_provider_attempt a ON a.id=o.attempt_id
  WHERE o.workspace_id=NEW.workspace_id AND o.id=NEW.observation_id AND a.dispatch_id=NEW.dispatch_id
   AND o.certainty='NOT_SENT' AND NEW.verification_call_id IS NULL) THEN RETURN NEW; END IF;
 IF NEW.verification_call_id IS NOT NULL AND EXISTS(
  SELECT 1 FROM lifecycle_provider_observation o JOIN lifecycle_provider_attempt a ON a.id=o.attempt_id
  JOIN lifecycle_provider_send_claim s ON s.attempt_id=a.id
  JOIN lifecycle_storage_call c ON c.id=NEW.verification_call_id AND c.workspace_id=a.workspace_id AND c.copy_id=a.copy_id AND c.operation='STAT_EXACT'
  JOIN lifecycle_storage_observation v ON v.call_id=c.id
  WHERE o.id=NEW.observation_id AND o.workspace_id=NEW.workspace_id AND a.dispatch_id=NEW.dispatch_id
   AND o.certainty='CONFIRMED' AND o.outcome='DENIED'
   AND v.http_status=200 AND v.failure='' AND v.invoked_at IS NOT NULL AND c.started_at>=o.observed_at) THEN RETURN NEW; END IF;
 RAISE EXCEPTION 'reservation release requires authoritative NOT_SENT or confirmed denied send plus fresh exact presence';
END; $$ LANGUAGE plpgsql;
