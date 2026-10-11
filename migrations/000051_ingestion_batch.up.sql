-- #298/B2. Core facts only; connector config and secrets remain in adapters.
CREATE TABLE ingestion_operation (
 id uuid PRIMARY KEY, workspace_id uuid NOT NULL, actor_id uuid NOT NULL,
 principal_ref varchar(255) NOT NULL, request_key varchar(128) NOT NULL,
 batch_key varchar(128) NOT NULL, connection_ref varchar(128) NOT NULL,
 instance_ref varchar(128) NOT NULL, config_hash char(64) NOT NULL,
 template_hash char(64) NOT NULL, schema_hash char(64) NOT NULL,
 request_fingerprint char(64) NOT NULL,
 state varchar(32) NOT NULL DEFAULT 'PREPARED', current_attempt_id uuid,
 remote_id varchar(255) NOT NULL DEFAULT '', manifest jsonb, manifest_hash char(64),
 created_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(workspace_id,request_key), UNIQUE(id,workspace_id),
 CHECK (state IN ('PREPARED','REGISTERING','REGISTERED','STARTING','RUNNING','UNKNOWN','SUCCEEDED','FAILED','ACCEPTED')),
 CHECK ((manifest IS NULL)=(manifest_hash IS NULL)),
 CHECK (state NOT IN ('SUCCEEDED','ACCEPTED') OR (manifest IS NOT NULL AND remote_id<>''))
);
CREATE TABLE ingestion_request_slot (
 operation_id uuid NOT NULL REFERENCES ingestion_operation(id),
 slot_no smallint NOT NULL CHECK(slot_no BETWEEN 1 AND 3),
 code varchar(128) NOT NULL, dataset_id uuid NOT NULL REFERENCES dataset(id),
 resource_id uuid NOT NULL REFERENCES data_resource(id),
 columns text[] NOT NULL CHECK(cardinality(columns)>0), row_count bigint NOT NULL CHECK(row_count>=0),
 PRIMARY KEY(operation_id,slot_no), UNIQUE(operation_id,code), UNIQUE(operation_id,dataset_id)
);
CREATE TABLE ingestion_attempt (
 id uuid PRIMARY KEY, operation_id uuid NOT NULL REFERENCES ingestion_operation(id),
 started_at timestamptz NOT NULL DEFAULT now(), UNIQUE(operation_id,id)
);
ALTER TABLE ingestion_operation ADD CONSTRAINT fk_ingestion_current_attempt
 FOREIGN KEY(id,current_attempt_id) REFERENCES ingestion_attempt(operation_id,id);
CREATE TABLE ingestion_invocation (
 id uuid PRIMARY KEY, operation_id uuid NOT NULL, workspace_id uuid NOT NULL,
 attempt_id uuid NOT NULL, kind varchar(16) NOT NULL CHECK(kind IN ('REGISTER','START','STATUS')),
 started_at timestamptz NOT NULL DEFAULT now(),
 FOREIGN KEY(operation_id,workspace_id) REFERENCES ingestion_operation(id,workspace_id),
 FOREIGN KEY(operation_id,attempt_id) REFERENCES ingestion_attempt(operation_id,id)
);
CREATE UNIQUE INDEX uq_ingestion_single_dispatch ON ingestion_invocation(attempt_id,kind)
 WHERE kind IN ('REGISTER','START');
CREATE TABLE ingestion_observation (
 id uuid PRIMARY KEY, invocation_id uuid NOT NULL REFERENCES ingestion_invocation(id),
 state varchar(32) NOT NULL CHECK(state IN ('REGISTERED','RUNNING','SUCCEEDED','FAILED','UNKNOWN')),
 remote_id varchar(255) NOT NULL DEFAULT '', observed_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE ingestion_receipt (
 id uuid PRIMARY KEY, operation_id uuid NOT NULL UNIQUE REFERENCES ingestion_operation(id),
 attempt_id uuid NOT NULL, manifest_hash char(64) NOT NULL,
 evidence_id uuid NOT NULL REFERENCES evidence(id), created_at timestamptz NOT NULL DEFAULT now(),
 FOREIGN KEY(operation_id,attempt_id) REFERENCES ingestion_attempt(operation_id,id)
);
CREATE TABLE ingestion_receipt_slot (
 receipt_id uuid NOT NULL REFERENCES ingestion_receipt(id),
 code varchar(128) NOT NULL, dataset_id uuid NOT NULL REFERENCES dataset(id),
 version_id uuid NOT NULL UNIQUE REFERENCES dataset_version(id),
 storage_uri text NOT NULL, content_hash char(64) NOT NULL,
 PRIMARY KEY(receipt_id,code), UNIQUE(receipt_id,dataset_id)
);

CREATE FUNCTION guard_ingestion_operation() RETURNS trigger AS $$
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
   (OLD.state='REGISTERING' AND NEW.state IN ('REGISTERED','UNKNOWN')) OR
   (OLD.state='REGISTERED' AND NEW.state='STARTING') OR
   (OLD.state='STARTING' AND NEW.state IN ('RUNNING','SUCCEEDED','FAILED','UNKNOWN')) OR
   (OLD.state IN ('RUNNING','UNKNOWN') AND NEW.state IN ('RUNNING','SUCCEEDED','FAILED','UNKNOWN')) OR
   (OLD.state='FAILED' AND NEW.state='PREPARED') OR
   (OLD.state='SUCCEEDED' AND NEW.state='ACCEPTED')
 ) THEN RAISE EXCEPTION 'illegal ingestion lifecycle transition'; END IF;
 IF NEW.current_attempt_id IS DISTINCT FROM OLD.current_attempt_id AND OLD.state NOT IN ('PREPARED','FAILED') THEN
    RAISE EXCEPTION 'active/unknown attempt cannot be replaced';
 END IF;
 RETURN NEW;
END; $$ LANGUAGE plpgsql;
CREATE TRIGGER trg_ingestion_operation_guard BEFORE UPDATE OR DELETE ON ingestion_operation
 FOR EACH ROW EXECUTE FUNCTION guard_ingestion_operation();

CREATE FUNCTION guard_ingestion_request_slot() RETURNS trigger AS $$
DECLARE op ingestion_operation; d dataset; r data_resource;
BEGIN
 IF TG_OP<>'INSERT' THEN RAISE EXCEPTION 'ingestion request slot is immutable'; END IF;
 SELECT * INTO op FROM ingestion_operation WHERE id=NEW.operation_id FOR UPDATE;
 IF op.current_attempt_id IS NOT NULL THEN RAISE EXCEPTION 'request slot set is frozen'; END IF;
 SELECT * INTO d FROM dataset WHERE id=NEW.dataset_id;
 SELECT * INTO r FROM data_resource WHERE id=NEW.resource_id;
 IF d.workspace_id IS DISTINCT FROM op.workspace_id OR r.workspace_id IS DISTINCT FROM op.workspace_id
    OR d.source_resource_id IS DISTINCT FROM r.id OR d.dataset_type<>'RAW' THEN
    RAISE EXCEPTION 'ingestion slot crosses workspace/source/type boundary';
 END IF;
 RETURN NEW;
END; $$ LANGUAGE plpgsql;
CREATE TRIGGER trg_ingestion_request_slot_guard BEFORE INSERT OR UPDATE OR DELETE ON ingestion_request_slot
 FOR EACH ROW EXECUTE FUNCTION guard_ingestion_request_slot();

CREATE TRIGGER trg_ingestion_attempt_immutable BEFORE UPDATE OR DELETE ON ingestion_attempt
 FOR EACH ROW EXECUTE FUNCTION prevent_delivery_history_mutation();
CREATE TRIGGER trg_ingestion_invocation_immutable BEFORE UPDATE OR DELETE ON ingestion_invocation
 FOR EACH ROW EXECUTE FUNCTION prevent_delivery_history_mutation();
CREATE TRIGGER trg_ingestion_observation_immutable BEFORE UPDATE OR DELETE ON ingestion_observation
 FOR EACH ROW EXECUTE FUNCTION prevent_delivery_history_mutation();
CREATE TRIGGER trg_ingestion_receipt_immutable BEFORE UPDATE OR DELETE ON ingestion_receipt
 FOR EACH ROW EXECUTE FUNCTION prevent_delivery_history_mutation();

CREATE FUNCTION guard_ingestion_receipt_slot() RETURNS trigger AS $$
DECLARE op ingestion_operation; h ingestion_receipt; s ingestion_request_slot; v dataset_version;
BEGIN
 IF TG_OP<>'INSERT' THEN RAISE EXCEPTION 'ingestion receipt slot is immutable'; END IF;
 SELECT * INTO h FROM ingestion_receipt WHERE id=NEW.receipt_id;
 SELECT * INTO op FROM ingestion_operation WHERE id=h.operation_id FOR UPDATE;
 IF op.state<>'SUCCEEDED' OR op.current_attempt_id<>h.attempt_id OR op.manifest_hash<>h.manifest_hash THEN
    RAISE EXCEPTION 'receipt does not bind current successful attempt';
 END IF;
 SELECT * INTO s FROM ingestion_request_slot WHERE operation_id=op.id AND code=NEW.code;
 SELECT * INTO v FROM dataset_version WHERE id=NEW.version_id;
 IF s.dataset_id IS NULL OR s.dataset_id IS DISTINCT FROM NEW.dataset_id OR v.dataset_id IS DISTINCT FROM NEW.dataset_id
    OR v.status<>'READY' OR v.storage_uri IS DISTINCT FROM NEW.storage_uri OR v.checksum_value IS DISTINCT FROM NEW.content_hash THEN
    RAISE EXCEPTION 'receipt slot does not bind its exact READY output';
 END IF;
 RETURN NEW;
END; $$ LANGUAGE plpgsql;
CREATE TRIGGER trg_ingestion_receipt_slot_guard BEFORE INSERT OR UPDATE OR DELETE ON ingestion_receipt_slot
 FOR EACH ROW EXECUTE FUNCTION guard_ingestion_receipt_slot();
CREATE FUNCTION check_ingestion_receipt_complete() RETURNS trigger AS $$
DECLARE op ingestion_operation;
BEGIN
 SELECT * INTO op FROM ingestion_operation WHERE id=NEW.operation_id FOR UPDATE;
 IF op.state<>'ACCEPTED' OR op.current_attempt_id<>NEW.attempt_id OR op.manifest_hash<>NEW.manifest_hash
    OR (SELECT count(*) FROM ingestion_receipt_slot WHERE receipt_id=NEW.id)<>3 THEN
    RAISE EXCEPTION 'partial ingestion batch cannot commit';
 END IF;
 RETURN NULL;
END; $$ LANGUAGE plpgsql;
CREATE CONSTRAINT TRIGGER trg_ingestion_receipt_complete AFTER INSERT ON ingestion_receipt
 DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION check_ingestion_receipt_complete();

CREATE FUNCTION check_ingestion_operation_complete() RETURNS trigger AS $$
DECLARE h ingestion_receipt;
BEGIN
 IF NEW.current_attempt_id IS NOT NULL AND (SELECT count(*) FROM ingestion_request_slot WHERE operation_id=NEW.id)<>3 THEN
   RAISE EXCEPTION 'attempt requires exactly three frozen request slots';
 END IF;
 IF NEW.state='ACCEPTED' THEN
  SELECT * INTO h FROM ingestion_receipt WHERE operation_id=NEW.id;
  IF h.id IS NULL OR h.attempt_id<>NEW.current_attempt_id OR h.manifest_hash<>NEW.manifest_hash
     OR (SELECT count(*) FROM ingestion_receipt_slot WHERE receipt_id=h.id)<>3 THEN
    RAISE EXCEPTION 'accepted batch requires complete receipt';
  END IF;
 END IF;
 RETURN NULL;
END; $$ LANGUAGE plpgsql;
CREATE CONSTRAINT TRIGGER trg_ingestion_operation_complete AFTER INSERT OR UPDATE ON ingestion_operation
 DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION check_ingestion_operation_complete();

ALTER TABLE cost_allocation ADD COLUMN ingestion_invocation_id uuid REFERENCES ingestion_invocation(id);
ALTER TABLE cost_allocation DROP CONSTRAINT ck_cost_allocation_single_subject;
ALTER TABLE cost_allocation ADD CONSTRAINT ck_cost_allocation_single_subject CHECK (
 num_nonnulls(delivery_operation_id,quality_assessment_id,quality_assessment_attempt_id,
 rights_declaration_id,rights_declaration_verification_id,rights_declaration_disposition_id,
 authorization_provenance_binding_id,authorization_provenance_binding_disposition_id,
 effective_rights_snapshot_id,dataset_certification_id,certification_disposition_id,
 annotation_review_attempt_id,annotation_engine_attempt_id,ingestion_invocation_id)=1
);
CREATE INDEX idx_cost_allocation_ingestion ON cost_allocation(ingestion_invocation_id) WHERE ingestion_invocation_id IS NOT NULL;
CREATE OR REPLACE FUNCTION guard_cost_allocation_workspace() RETURNS trigger AS $$
DECLARE event_workspace uuid; subject_workspace uuid;
BEGIN
 SELECT workspace_id INTO event_workspace FROM cost_event WHERE id=NEW.cost_event_id;
 IF NEW.delivery_operation_id IS NOT NULL THEN
  SELECT workspace_id INTO subject_workspace FROM delivery_operation WHERE id=NEW.delivery_operation_id;
 ELSIF NEW.quality_assessment_id IS NOT NULL THEN
  SELECT workspace_id INTO subject_workspace FROM quality_result WHERE id=NEW.quality_assessment_id;
 ELSIF NEW.quality_assessment_attempt_id IS NOT NULL THEN
  SELECT workspace_id INTO subject_workspace FROM quality_assessment_attempt WHERE id=NEW.quality_assessment_attempt_id;
 ELSIF NEW.rights_declaration_id IS NOT NULL THEN
  SELECT workspace_id INTO subject_workspace FROM rights_declaration WHERE id=NEW.rights_declaration_id;
 ELSIF NEW.rights_declaration_verification_id IS NOT NULL THEN
  SELECT d.workspace_id INTO subject_workspace FROM rights_declaration_verification v JOIN rights_declaration d ON d.id=v.declaration_id WHERE v.id=NEW.rights_declaration_verification_id;
 ELSIF NEW.rights_declaration_disposition_id IS NOT NULL THEN
  SELECT d.workspace_id INTO subject_workspace FROM rights_declaration_disposition x JOIN rights_declaration d ON d.id=x.declaration_id WHERE x.id=NEW.rights_declaration_disposition_id;
 ELSIF NEW.authorization_provenance_binding_id IS NOT NULL THEN
  SELECT workspace_id INTO subject_workspace FROM authorization_provenance_binding WHERE id=NEW.authorization_provenance_binding_id;
 ELSIF NEW.authorization_provenance_binding_disposition_id IS NOT NULL THEN
  SELECT b.workspace_id INTO subject_workspace FROM authorization_provenance_binding_disposition x JOIN authorization_provenance_binding b ON b.id=x.binding_id WHERE x.id=NEW.authorization_provenance_binding_disposition_id;
 ELSIF NEW.effective_rights_snapshot_id IS NOT NULL THEN
  SELECT workspace_id INTO subject_workspace FROM effective_rights_snapshot WHERE id=NEW.effective_rights_snapshot_id;
 ELSIF NEW.dataset_certification_id IS NOT NULL THEN
  SELECT workspace_id INTO subject_workspace FROM dataset_certification WHERE id=NEW.dataset_certification_id;
 ELSIF NEW.certification_disposition_id IS NOT NULL THEN
  SELECT workspace_id INTO subject_workspace FROM certification_disposition WHERE id=NEW.certification_disposition_id;
 ELSIF NEW.annotation_review_attempt_id IS NOT NULL THEN
  SELECT workspace_id INTO subject_workspace FROM annotation_review_attempt WHERE id=NEW.annotation_review_attempt_id;
 ELSIF NEW.annotation_engine_attempt_id IS NOT NULL THEN
  SELECT workspace_id INTO subject_workspace FROM annotation_engine_attempt WHERE id=NEW.annotation_engine_attempt_id;
 ELSE
  SELECT workspace_id INTO subject_workspace FROM ingestion_invocation WHERE id=NEW.ingestion_invocation_id;
 END IF;
 IF event_workspace IS DISTINCT FROM subject_workspace THEN RAISE EXCEPTION 'cost allocation crosses workspace boundary'; END IF;
 RETURN NEW;
END; $$ LANGUAGE plpgsql;
