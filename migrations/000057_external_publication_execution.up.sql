-- #86: platform-owned execution facts; no provider runtime becomes Core truth.
CREATE TABLE external_publication_attempt (
 id uuid PRIMARY KEY, publication_id uuid NOT NULL REFERENCES external_publication(id),
 workspace_id uuid NOT NULL, attempt_no bigint NOT NULL CHECK(attempt_no>0),
 product_release_id uuid NOT NULL REFERENCES product_release(id),
 binding_id uuid NOT NULL REFERENCES external_identity_binding(id),
 identifier_id uuid NOT NULL REFERENCES external_identifier(id),
 provider varchar(64) NOT NULL CHECK(provider ~ '^[a-z][a-z0-9._-]{0,63}$'),
 instance_ref varchar(128) NOT NULL CHECK(length(btrim(instance_ref))>0 AND instance_ref=btrim(instance_ref)),
 adapter_revision varchar(64) NOT NULL CHECK(length(btrim(adapter_revision))>0 AND adapter_revision=btrim(adapter_revision)),
 external_subject_id varchar(255) NOT NULL,
 external_identifier varchar(255) NOT NULL,
 request_key varchar(255) NOT NULL UNIQUE CHECK(length(btrim(request_key))>0 AND request_key=btrim(request_key)),
 request_bytes text NOT NULL, request_hash char(64) NOT NULL CHECK(request_hash ~ '^[0-9a-f]{64}$'),
 principal_ref varchar(255) NOT NULL CHECK(length(btrim(principal_ref))>0),
 actor_id uuid NOT NULL, evidence_id uuid NOT NULL REFERENCES evidence(id),
 created_at timestamptz NOT NULL DEFAULT now(), UNIQUE(publication_id,attempt_no),
 CHECK(encode(digest(request_bytes,'sha256'),'hex')=request_hash),
 CHECK(request_bytes::jsonb=jsonb_build_object('workspaceId',workspace_id,'publicationId',publication_id,
 'releaseId',product_release_id,'bindingId',binding_id,'identifierId',identifier_id,'provider',provider,
 'instance',instance_ref,'adapterRevision',adapter_revision,'externalSubjectId',external_subject_id,
 'externalIdentifier',external_identifier,'requestKey',request_key))
);
CREATE TABLE external_provider_call (
 id uuid PRIMARY KEY, attempt_id uuid NOT NULL REFERENCES external_publication_attempt(id),
 workspace_id uuid NOT NULL, kind varchar(16) NOT NULL CHECK(kind IN ('IDENTITY','IDENTIFIER','SUBMIT','POLL')),
 evidence_id uuid NOT NULL REFERENCES evidence(id), created_at timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX uq_external_single_submit ON external_provider_call(attempt_id) WHERE kind='SUBMIT';
CREATE INDEX idx_external_call_attempt ON external_provider_call(attempt_id,created_at,id);
CREATE TABLE external_provider_dispatch (
 call_id uuid PRIMARY KEY REFERENCES external_provider_call(id),
 evidence_id uuid NOT NULL REFERENCES evidence(id), created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE external_provider_observation (
 id uuid PRIMARY KEY, call_id uuid NOT NULL UNIQUE REFERENCES external_provider_call(id),
 workspace_id uuid NOT NULL, state varchar(16) NOT NULL CHECK(state IN ('VERIFIED','REGISTERED','PUBLISHED','ABSENT','FAILED','UNKNOWN')),
 error_code varchar(32) NOT NULL CHECK(error_code IN ('OK','PROVIDER_UNAVAILABLE','INVALID_PROVIDER_RESULT','DISPATCH_UNCONFIRMED')),
 instance_ref varchar(128) NOT NULL, request_key varchar(255) NOT NULL, request_hash char(64) NOT NULL,
 external_id varchar(255) NOT NULL DEFAULT '', remote_id varchar(255) NOT NULL DEFAULT '',
 evidence_id uuid NOT NULL REFERENCES evidence(id), created_at timestamptz NOT NULL DEFAULT now(),
 CHECK(state IN ('VERIFIED','REGISTERED','PUBLISHED') OR (external_id='' AND remote_id='')),
 CHECK(state NOT IN ('REGISTERED','PUBLISHED') OR (external_id<>'' AND remote_id<>'')),
 CHECK((error_code='OK') OR state='UNKNOWN')
);
CREATE TABLE external_publication_attempt_resolution (
 attempt_id uuid PRIMARY KEY REFERENCES external_publication_attempt(id),
 observation_id uuid NOT NULL REFERENCES external_provider_observation(id), workspace_id uuid NOT NULL,
 outcome varchar(16) NOT NULL CHECK(outcome IN ('FAILED','BLOCKED','PUBLISHED')),
 evidence_id uuid NOT NULL REFERENCES evidence(id), created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_external_attempt_workspace ON external_publication_attempt(workspace_id,publication_id,attempt_no);
DO $$ DECLARE t text; BEGIN
 FOREACH t IN ARRAY ARRAY['external_publication_attempt','external_provider_call','external_provider_dispatch','external_provider_observation','external_publication_attempt_resolution'] LOOP
 EXECUTE format('CREATE TRIGGER immutable_external_execution BEFORE UPDATE OR DELETE ON %I FOR EACH ROW EXECUTE FUNCTION guard_external_immutable()',t);
 END LOOP;
END $$;

CREATE FUNCTION guard_external_execution_reference() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE a external_publication_attempt; c external_provider_call; o external_provider_observation;
 p external_publication; b external_identity_binding; i external_identifier; ws uuid;
BEGIN
 IF TG_TABLE_NAME='external_publication_attempt' THEN
  -- The application has already acquired binding/release/identifier first.
  SELECT * INTO b FROM external_identity_binding WHERE id=NEW.binding_id FOR UPDATE;
  PERFORM 1 FROM product_release WHERE id=NEW.product_release_id AND status='PUBLISHED' FOR SHARE;
  IF NOT FOUND THEN RAISE EXCEPTION 'external attempt requires published release'; END IF;
  SELECT * INTO i FROM external_identifier WHERE id=NEW.identifier_id FOR UPDATE;
  SELECT * INTO p FROM external_publication WHERE id=NEW.publication_id FOR UPDATE;
  IF p.id IS NULL OR p.workspace_id<>NEW.workspace_id OR p.product_release_id<>NEW.product_release_id OR
   p.identity_binding_id<>NEW.binding_id OR p.provider<>NEW.provider OR p.status NOT IN ('SUBMITTING','REGISTERED') OR
   b.workspace_id<>NEW.workspace_id OR b.provider<>NEW.provider OR b.external_subject_id<>NEW.external_subject_id OR
   i.workspace_id<>NEW.workspace_id OR i.provider<>NEW.provider OR i.object_type<>'PRODUCT_RELEASE' OR
   i.object_id<>NEW.product_release_id OR i.external_id<>NEW.external_identifier OR
   EXISTS(SELECT 1 FROM external_identity_binding_disposition WHERE binding_id=b.id) OR
   EXISTS(SELECT 1 FROM external_identifier_disposition WHERE identifier_id=i.id) THEN
   RAISE EXCEPTION 'external attempt reference mismatch';
  END IF;
  IF NEW.attempt_no<>COALESCE((SELECT max(attempt_no) FROM external_publication_attempt WHERE publication_id=p.id),0)+1 OR
   EXISTS(SELECT 1 FROM external_publication_attempt prior LEFT JOIN external_publication_attempt_resolution r ON r.attempt_id=prior.id
   WHERE prior.publication_id=p.id AND (r.attempt_id IS NULL OR r.outcome<>'FAILED')) THEN
   RAISE EXCEPTION 'external attempt requires definitively failed predecessors';
  END IF;
  ws:=NEW.workspace_id;
 ELSIF TG_TABLE_NAME='external_provider_call' THEN
  SELECT * INTO a FROM external_publication_attempt WHERE id=NEW.attempt_id FOR UPDATE;
  IF a.id IS NULL OR a.workspace_id<>NEW.workspace_id OR EXISTS(SELECT 1 FROM external_publication_attempt_resolution WHERE attempt_id=a.id) THEN
   RAISE EXCEPTION 'external call requires current attempt';
  END IF;
  ws:=a.workspace_id;
 ELSIF TG_TABLE_NAME='external_provider_dispatch' THEN
  SELECT * INTO c FROM external_provider_call WHERE id=NEW.call_id;
  PERFORM 1 FROM external_publication_attempt WHERE id=c.attempt_id FOR UPDATE;
  IF EXISTS(SELECT 1 FROM external_publication_attempt_resolution WHERE attempt_id=c.attempt_id) THEN RAISE EXCEPTION 'external dispatch requires open attempt'; END IF;
  ws:=c.workspace_id;
 ELSIF TG_TABLE_NAME='external_provider_observation' THEN
  SELECT * INTO c FROM external_provider_call WHERE id=NEW.call_id;
  SELECT * INTO a FROM external_publication_attempt WHERE id=c.attempt_id;
  IF a.id IS NULL OR NEW.workspace_id<>a.workspace_id OR NEW.instance_ref<>a.instance_ref OR
   NEW.request_key<>a.request_key OR NEW.request_hash<>a.request_hash OR
   (NEW.state IN ('VERIFIED','REGISTERED','PUBLISHED') AND NOT EXISTS(SELECT 1 FROM external_provider_dispatch WHERE call_id=c.id)) OR
   (NEW.state='VERIFIED' AND (c.kind NOT IN ('IDENTITY','IDENTIFIER') OR NEW.external_id<>CASE c.kind WHEN 'IDENTITY' THEN a.external_subject_id ELSE a.external_identifier END OR NEW.remote_id<>'')) OR
   (NEW.state IN ('REGISTERED','PUBLISHED') AND (c.kind NOT IN ('SUBMIT','POLL') OR NEW.external_id<>a.external_identifier)) OR
   (NEW.state='ABSENT' AND c.kind<>'POLL') THEN
   RAISE EXCEPTION 'external observation context mismatch';
  END IF;
  IF NEW.state IN ('REGISTERED','PUBLISHED') AND EXISTS(
   SELECT 1 FROM external_provider_observation prior JOIN external_provider_call pc ON pc.id=prior.call_id
   JOIN external_publication_attempt pa ON pa.id=pc.attempt_id
   WHERE pa.publication_id=a.publication_id AND prior.state IN ('REGISTERED','PUBLISHED') AND prior.remote_id<>NEW.remote_id
  ) THEN RAISE EXCEPTION 'external catalog identity changed'; END IF;
  ws:=a.workspace_id;
 ELSE
  SELECT * INTO a FROM external_publication_attempt WHERE id=NEW.attempt_id FOR UPDATE;
  SELECT * INTO o FROM external_provider_observation WHERE id=NEW.observation_id;
  SELECT * INTO c FROM external_provider_call WHERE id=o.call_id;
  IF a.id IS NULL OR a.workspace_id<>NEW.workspace_id OR c.attempt_id<>a.id THEN RAISE EXCEPTION 'external resolution context mismatch'; END IF;
  IF NEW.outcome='FAILED' AND (o.state NOT IN ('FAILED','ABSENT') OR
   (EXISTS(SELECT 1 FROM external_provider_call WHERE attempt_id=a.id AND kind='SUBMIT') AND c.kind NOT IN ('SUBMIT','POLL')) OR
   EXISTS(SELECT 1 FROM external_provider_observation po JOIN external_provider_call pc ON pc.id=po.call_id WHERE pc.attempt_id=a.id AND po.state='PUBLISHED')) THEN
   RAISE EXCEPTION 'external failure requires definite inactive provider outcome';
  END IF;
  IF NEW.outcome='PUBLISHED' AND (o.state<>'PUBLISHED' OR NOT EXISTS(SELECT 1 FROM external_publication WHERE id=a.publication_id AND status='PUBLISHED')) THEN
   RAISE EXCEPTION 'external published resolution requires confirmed projection';
  END IF;
  ws:=a.workspace_id;
 END IF;
 IF ws IS NULL OR NOT EXISTS(SELECT 1 FROM evidence WHERE id=NEW.evidence_id AND workspace_id=ws) THEN RAISE EXCEPTION 'external execution evidence mismatch'; END IF;
 RETURN NEW;
END $$;
DO $$ DECLARE t text; BEGIN
 FOREACH t IN ARRAY ARRAY['external_publication_attempt','external_provider_call','external_provider_dispatch','external_provider_observation','external_publication_attempt_resolution'] LOOP
 EXECUTE format('CREATE TRIGGER external_execution_reference BEFORE INSERT ON %I FOR EACH ROW EXECUTE FUNCTION guard_external_execution_reference()',t);
 END LOOP;
END $$;

ALTER TABLE external_publication DROP CONSTRAINT external_publication_status_check;
ALTER TABLE external_publication DROP CONSTRAINT external_publication_revision_check;
ALTER TABLE external_publication ADD CONSTRAINT external_publication_status_check CHECK(status IN ('DRAFT','SUBMITTING','REGISTERED','PUBLISHED'));
ALTER TABLE external_publication ADD CONSTRAINT external_publication_revision_check CHECK(
 (status='DRAFT' AND revision=0) OR (status='SUBMITTING' AND revision=1) OR (status='REGISTERED' AND revision=2) OR (status='PUBLISHED' AND revision=3));
ALTER TABLE external_publication_transition ADD COLUMN provider_observation_id uuid REFERENCES external_provider_observation(id);
ALTER TABLE external_publication_transition DROP CONSTRAINT external_publication_transition_check;
ALTER TABLE external_publication_transition ADD CONSTRAINT external_publication_transition_check CHECK(
 (revision=0 AND from_status='' AND to_status='DRAFT' AND provider_observation_id IS NULL) OR
 (revision=1 AND from_status='DRAFT' AND to_status='SUBMITTING' AND provider_observation_id IS NULL) OR
 (revision=2 AND from_status='SUBMITTING' AND to_status='REGISTERED' AND provider_observation_id IS NOT NULL) OR
 (revision=3 AND from_status='REGISTERED' AND to_status='PUBLISHED' AND provider_observation_id IS NOT NULL));
CREATE OR REPLACE FUNCTION guard_external_publication_projection() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'external publication cannot be deleted' USING ERRCODE='23514'; END IF;
 IF TG_OP='INSERT' THEN
  IF NEW.status<>'DRAFT' OR NEW.revision<>0 THEN RAISE EXCEPTION 'external publication starts DRAFT' USING ERRCODE='23514'; END IF;
 ELSE
  IF (to_jsonb(NEW)-'status'-'revision') IS DISTINCT FROM (to_jsonb(OLD)-'status'-'revision') OR NEW.revision<>OLD.revision+1 OR NOT (
   (OLD.status='DRAFT' AND NEW.status='SUBMITTING') OR (OLD.status='SUBMITTING' AND NEW.status='REGISTERED') OR (OLD.status='REGISTERED' AND NEW.status='PUBLISHED')) THEN
   RAISE EXCEPTION 'illegal external publication transition' USING ERRCODE='23514';
  END IF;
  PERFORM 1 FROM external_identity_binding WHERE id=NEW.identity_binding_id FOR UPDATE NOWAIT;
  IF EXISTS(SELECT 1 FROM external_identity_binding_disposition WHERE binding_id=NEW.identity_binding_id) THEN RAISE EXCEPTION 'external identity invalidated' USING ERRCODE='23514'; END IF;
 END IF;
 RETURN NEW;
END $$;
CREATE OR REPLACE FUNCTION guard_external_transition() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE p external_publication; expected_event text;
BEGIN
 SELECT * INTO p FROM external_publication WHERE id=NEW.publication_id FOR UPDATE;
 IF p.id IS NULL OR p.workspace_id<>NEW.workspace_id OR p.revision<>NEW.revision OR p.status<>NEW.to_status OR
  NOT EXISTS(SELECT 1 FROM evidence WHERE id=NEW.evidence_id AND workspace_id=p.workspace_id) THEN RAISE EXCEPTION 'external transition projection/evidence mismatch' USING ERRCODE='23514'; END IF;
 expected_event:=CASE NEW.to_status WHEN 'DRAFT' THEN 'ExternalPublicationCreated' WHEN 'SUBMITTING' THEN 'ExternalPublicationRequested' WHEN 'REGISTERED' THEN 'ExternalPublicationRegistered' ELSE 'ExternalPublicationPublished' END;
 IF NEW.revision>=2 AND NOT EXISTS(
  SELECT 1 FROM external_provider_observation o JOIN external_provider_call c ON c.id=o.call_id JOIN external_publication_attempt a ON a.id=c.attempt_id
  WHERE o.id=NEW.provider_observation_id AND a.publication_id=p.id AND a.actor_id=NEW.created_by AND a.principal_ref=NEW.principal_ref AND
   ((NEW.to_status='REGISTERED' AND o.state IN ('REGISTERED','PUBLISHED')) OR (NEW.to_status='PUBLISHED' AND o.state='PUBLISHED'))
 ) THEN RAISE EXCEPTION 'external transition requires exact verified provider observation' USING ERRCODE='23514'; END IF;
 IF NOT EXISTS(SELECT 1 FROM outbox_event WHERE id=NEW.event_id AND aggregate_id=p.id AND aggregate_type='ExternalPublication' AND event_type=expected_event AND
  payload->>'workspaceId'=p.workspace_id::text AND payload->>'revision'=NEW.revision::text AND payload->>'factId'=NEW.id::text AND payload->>'actorId'=NEW.created_by::text AND payload->>'principalRef'=NEW.principal_ref AND
  (NEW.revision<2 OR payload->>'providerObservationId'=NEW.provider_observation_id::text)) THEN RAISE EXCEPTION 'external transition event mismatch' USING ERRCODE='23514'; END IF;
 RETURN NEW;
END $$;

ALTER TABLE cost_allocation ADD COLUMN external_provider_call_id uuid REFERENCES external_provider_call(id);
ALTER TABLE cost_allocation DROP CONSTRAINT ck_cost_allocation_single_subject;
ALTER TABLE cost_allocation ADD CONSTRAINT ck_cost_allocation_single_subject CHECK(
 num_nonnulls(delivery_operation_id,quality_assessment_id,quality_assessment_attempt_id,
 rights_declaration_id,rights_declaration_verification_id,rights_declaration_disposition_id,
 authorization_provenance_binding_id,authorization_provenance_binding_disposition_id,
 effective_rights_snapshot_id,dataset_certification_id,certification_disposition_id,
 annotation_review_attempt_id,annotation_engine_attempt_id,ingestion_invocation_id,external_provider_call_id)=1);
CREATE INDEX idx_cost_allocation_external ON cost_allocation(external_provider_call_id) WHERE external_provider_call_id IS NOT NULL;
-- The existing workspace guard is extended below without removing existing subjects.

CREATE OR REPLACE FUNCTION guard_cost_allocation_workspace() RETURNS trigger AS $$
DECLARE event_workspace uuid; subject_workspace uuid;
BEGIN
 SELECT workspace_id INTO event_workspace FROM cost_event WHERE id=NEW.cost_event_id;
 IF NEW.external_provider_call_id IS NOT NULL THEN
  SELECT workspace_id INTO subject_workspace FROM external_provider_call WHERE id=NEW.external_provider_call_id;
  IF NOT EXISTS(SELECT 1 FROM cost_event e JOIN external_provider_dispatch d ON d.call_id=NEW.external_provider_call_id WHERE e.id=NEW.cost_event_id AND e.activity_id=NEW.external_provider_call_id AND e.cost_type='EXTERNAL_PROVIDER_INVOCATION' AND e.quantity=1 AND e.unit='invocation' AND e.pricing_mode='ACTUAL') THEN RAISE EXCEPTION 'external cost requires dispatched physical call'; END IF;
 ELSIF NEW.delivery_operation_id IS NOT NULL THEN
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
