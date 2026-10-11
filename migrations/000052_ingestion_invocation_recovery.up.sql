-- #303 follow-up: durable engine result separate from Core projection.
CREATE TABLE ingestion_invocation_outcome (
 invocation_id uuid PRIMARY KEY REFERENCES ingestion_invocation(id),
 provider_dispatched boolean NOT NULL,
 state varchar(32) NOT NULL CHECK(state IN ('REGISTERED','RUNNING','SUCCEEDED','FAILED','UNKNOWN')),
 remote_id varchar(255) NOT NULL DEFAULT '', manifest jsonb, manifest_hash char(64),
 observed_at timestamptz NOT NULL DEFAULT now(),
 CHECK ((manifest IS NULL)=(manifest_hash IS NULL)),
 CHECK (state<>'SUCCEEDED' OR (manifest IS NOT NULL AND remote_id<>'')),
 CHECK (state<>'REGISTERED' OR remote_id<>''),
 CHECK (provider_dispatched OR state IN ('FAILED','UNKNOWN'))
);
CREATE TRIGGER trg_ingestion_invocation_outcome_immutable BEFORE UPDATE OR DELETE ON ingestion_invocation_outcome
 FOR EACH ROW EXECUTE FUNCTION prevent_delivery_history_mutation();
-- Existing immutable observation is the once-only Core application marker.
CREATE UNIQUE INDEX uq_ingestion_observation_invocation ON ingestion_observation(invocation_id);

CREATE OR REPLACE FUNCTION guard_ingestion_operation() RETURNS trigger AS $$
BEGIN
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'ingestion operation is historical'; END IF;
 IF (to_jsonb(OLD)-ARRAY['state','current_attempt_id','remote_id','manifest','manifest_hash']) IS DISTINCT FROM
    (to_jsonb(NEW)-ARRAY['state','current_attempt_id','remote_id','manifest','manifest_hash']) THEN
    RAISE EXCEPTION 'ingestion request identity is immutable';
 END IF;
 IF OLD.remote_id<>'' AND OLD.remote_id IS DISTINCT FROM NEW.remote_id
    AND NOT (OLD.state='FAILED' AND NEW.state='PREPARED' AND NEW.current_attempt_id IS DISTINCT FROM OLD.current_attempt_id) THEN
    RAISE EXCEPTION 'engine identity cannot be replaced';
 END IF;
 IF OLD.manifest IS NOT NULL AND (OLD.manifest IS DISTINCT FROM NEW.manifest OR OLD.manifest_hash IS DISTINCT FROM NEW.manifest_hash) THEN
    RAISE EXCEPTION 'successful manifest is immutable';
 END IF;
 IF OLD.state='ACCEPTED' AND NEW IS DISTINCT FROM OLD THEN RAISE EXCEPTION 'accepted ingestion is frozen'; END IF;
 IF NEW.state<>OLD.state AND NOT (
   (OLD.state='PREPARED' AND NEW.state='REGISTERING') OR
   (OLD.state='REGISTERING' AND NEW.state IN ('REGISTERED','FAILED','UNKNOWN')) OR
   (OLD.state='REGISTERED' AND NEW.state='STARTING') OR
   (OLD.state='STARTING' AND NEW.state IN ('RUNNING','SUCCEEDED','FAILED','UNKNOWN')) OR
   (OLD.state IN ('RUNNING','UNKNOWN') AND NEW.state IN ('RUNNING','SUCCEEDED','FAILED','UNKNOWN')) OR
   (OLD.state='FAILED' AND NEW.state='PREPARED') OR
   (OLD.state='SUCCEEDED' AND NEW.state='ACCEPTED')
 ) THEN RAISE EXCEPTION 'illegal ingestion lifecycle transition'; END IF;
 IF NEW.current_attempt_id IS DISTINCT FROM OLD.current_attempt_id AND OLD.state NOT IN ('PREPARED','FAILED') THEN
    RAISE EXCEPTION 'active/unknown attempt cannot be replaced';
 END IF;
 IF OLD.state='REGISTERING' AND NEW.state='FAILED' AND NOT EXISTS(
   SELECT 1 FROM ingestion_invocation i JOIN ingestion_invocation_outcome o ON o.invocation_id=i.id
   WHERE i.operation_id=OLD.id AND i.attempt_id=OLD.current_attempt_id AND i.kind='REGISTER'
     AND NOT o.provider_dispatched AND o.state='FAILED'
 ) THEN RAISE EXCEPTION 'register failure requires definite no-dispatch observation'; END IF;
 RETURN NEW;
END; $$ LANGUAGE plpgsql;
