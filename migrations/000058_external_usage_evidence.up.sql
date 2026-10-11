-- #87: provider observations extend, never replace, production Evidence.
CREATE TABLE external_usage_source (
 id uuid PRIMARY KEY, workspace_id uuid NOT NULL, binding_id uuid NOT NULL REFERENCES external_identity_binding(id),
 provider varchar(64) NOT NULL CHECK(provider ~ '^[a-z][a-z0-9._-]{0,63}$'), instance_ref varchar(128) NOT NULL CHECK(instance_ref<>''), adapter_revision varchar(64) NOT NULL CHECK(adapter_revision<>''),
 stream varchar(255) NOT NULL CHECK(stream ~ '^urn:[A-Za-z0-9][A-Za-z0-9._:-]{0,247}$'), external_subject_id varchar(255) NOT NULL,
 actor_id uuid NOT NULL, principal_ref varchar(255) NOT NULL, evidence_id uuid NOT NULL REFERENCES evidence(id), created_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(workspace_id,provider,instance_ref,stream)
);
CREATE TABLE external_usage_pull (
 id uuid PRIMARY KEY, source_id uuid NOT NULL REFERENCES external_usage_source(id), workspace_id uuid NOT NULL,
 expected_revision bigint NOT NULL CHECK(expected_revision>=0), from_cursor varchar(512) NOT NULL CHECK(from_cursor ~ '^[A-Za-z0-9._:-]*$'),
 evidence_id uuid NOT NULL REFERENCES evidence(id), created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE external_usage_call (
 id uuid PRIMARY KEY, pull_id uuid NOT NULL REFERENCES external_usage_pull(id), source_id uuid NOT NULL REFERENCES external_usage_source(id), workspace_id uuid NOT NULL,
 request_hash char(64) NOT NULL CHECK(request_hash ~ '^[0-9a-f]{64}$'), evidence_id uuid NOT NULL REFERENCES evidence(id), created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE external_usage_dispatch (call_id uuid PRIMARY KEY REFERENCES external_usage_call(id), source_id uuid NOT NULL REFERENCES external_usage_source(id), workspace_id uuid NOT NULL,evidence_id uuid NOT NULL REFERENCES evidence(id),created_at timestamptz NOT NULL DEFAULT now());
CREATE TABLE external_usage_outcome (
 call_id uuid PRIMARY KEY REFERENCES external_usage_call(id), source_id uuid NOT NULL REFERENCES external_usage_source(id), workspace_id uuid NOT NULL,
 state varchar(32) NOT NULL CHECK(state IN ('PAGE','READ_FAILED','INVALID_PAGE','DISPATCH_REJECTED')), page jsonb,
 evidence_id uuid NOT NULL REFERENCES evidence(id),created_at timestamptz NOT NULL DEFAULT now(),
 CHECK((state='PAGE')=(page IS NOT NULL)), CHECK(page IS NULL OR (jsonb_typeof(page)='object' AND octet_length(page::text)<=131072 AND jsonb_typeof(page->'events')='array' AND jsonb_array_length(page->'events')<=50))
);
CREATE TABLE external_usage_event (
 id uuid PRIMARY KEY, source_id uuid NOT NULL REFERENCES external_usage_source(id), workspace_id uuid NOT NULL, call_id uuid NOT NULL REFERENCES external_usage_call(id),page_index int NOT NULL CHECK(page_index BETWEEN 0 AND 49),
 provider varchar(64) NOT NULL, instance_ref varchar(128) NOT NULL,event_source varchar(255) NOT NULL,provider_event_id varchar(255) NOT NULL CHECK(provider_event_id<>''),
 category varchar(64) NOT NULL CHECK(category<>''),occurred_at timestamptz NOT NULL,external_subject_id varchar(255) NOT NULL,external_identifier varchar(255) NOT NULL,remote_id varchar(255) NOT NULL,
 payload_hash char(64) NOT NULL CHECK(payload_hash ~ '^[0-9a-f]{64}$'),evidence_id uuid NOT NULL REFERENCES evidence(id),created_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(workspace_id,provider,instance_ref,event_source,provider_event_id),
 CHECK(payload_hash=encode(digest(concat_ws(E'\n',provider_event_id,event_source,category,to_char(occurred_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS.US"Z"'),external_subject_id,external_identifier,remote_id),'sha256'),'hex'))
);
CREATE TABLE external_usage_decision (
 id uuid PRIMARY KEY,event_id uuid NOT NULL REFERENCES external_usage_event(id),source_id uuid NOT NULL REFERENCES external_usage_source(id),workspace_id uuid NOT NULL,
 status varchar(16) NOT NULL CHECK(status IN ('ACCEPTED','QUARANTINED')),reason varchar(32) NOT NULL CHECK(reason IN ('MATCHED','UNSUPPORTED_CATEGORY','SUBJECT_MISMATCH','MAPPING_NOT_FOUND','AMBIGUOUS_MAPPING')),
 binding_id uuid REFERENCES external_identity_binding(id),identifier_id uuid REFERENCES external_identifier(id),publication_id uuid REFERENCES external_publication(id),release_id uuid REFERENCES product_release(id),observation_id uuid REFERENCES external_provider_observation(id),
 evidence_id uuid NOT NULL REFERENCES evidence(id),created_at timestamptz NOT NULL DEFAULT now(),
 CHECK((status='ACCEPTED' AND reason='MATCHED' AND num_nonnulls(binding_id,identifier_id,publication_id,release_id,observation_id)=5) OR (status='QUARANTINED' AND reason<>'MATCHED' AND num_nonnulls(binding_id,identifier_id,publication_id,release_id,observation_id)=0))
);
CREATE UNIQUE INDEX uq_external_usage_accepted ON external_usage_decision(event_id) WHERE status='ACCEPTED';
CREATE UNIQUE INDEX uq_external_usage_quarantined ON external_usage_decision(event_id) WHERE status='QUARANTINED';
CREATE TABLE external_usage_conflict (
 id uuid PRIMARY KEY,event_id uuid NOT NULL REFERENCES external_usage_event(id),source_id uuid NOT NULL REFERENCES external_usage_source(id),workspace_id uuid NOT NULL,call_id uuid NOT NULL REFERENCES external_usage_call(id),page_index int NOT NULL CHECK(page_index BETWEEN 0 AND 49),
 payload_hash char(64) NOT NULL CHECK(payload_hash ~ '^[0-9a-f]{64}$'),evidence_id uuid NOT NULL REFERENCES evidence(id),created_at timestamptz NOT NULL DEFAULT now(),UNIQUE(event_id,payload_hash)
);
CREATE TABLE external_usage_checkpoint (
 source_id uuid NOT NULL REFERENCES external_usage_source(id),workspace_id uuid NOT NULL,revision bigint NOT NULL CHECK(revision>0),pull_id uuid NOT NULL UNIQUE REFERENCES external_usage_pull(id),call_id uuid NOT NULL REFERENCES external_usage_call(id),cursor varchar(512) NOT NULL CHECK(cursor ~ '^[A-Za-z0-9._:-]*$'),
 evidence_id uuid NOT NULL REFERENCES evidence(id),created_at timestamptz NOT NULL DEFAULT now(),PRIMARY KEY(source_id,revision)
);
CREATE TABLE external_usage_resolution (pull_id uuid PRIMARY KEY REFERENCES external_usage_pull(id),source_id uuid NOT NULL REFERENCES external_usage_source(id),workspace_id uuid NOT NULL,outcome varchar(16) NOT NULL CHECK(outcome IN ('IMPORTED','FAILED','BLOCKED','STALE')),evidence_id uuid NOT NULL REFERENCES evidence(id),created_at timestamptz NOT NULL DEFAULT now());
CREATE INDEX idx_external_usage_pending ON external_usage_pull(source_id,created_at,id);
CREATE INDEX idx_external_usage_calls ON external_usage_call(pull_id,created_at,id);
CREATE INDEX idx_external_usage_events ON external_usage_event(workspace_id,created_at,id);
CREATE INDEX idx_external_usage_conflicts ON external_usage_conflict(workspace_id,created_at,id);

CREATE FUNCTION guard_external_usage_reference() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE s external_usage_source;b external_identity_binding;p external_usage_pull;c external_usage_call;e external_usage_event;cp external_usage_checkpoint;candidate jsonb;candidate_hash text;
BEGIN
 IF TG_TABLE_NAME='external_usage_source' THEN
  SELECT * INTO b FROM external_identity_binding WHERE id=NEW.binding_id FOR UPDATE;
  IF b.workspace_id IS DISTINCT FROM NEW.workspace_id OR b.provider<>NEW.provider OR b.external_subject_id<>NEW.external_subject_id OR b.subject_id<>NEW.actor_id OR b.principal_ref<>NEW.principal_ref OR EXISTS(SELECT 1 FROM external_identity_binding_disposition WHERE binding_id=b.id) THEN RAISE EXCEPTION 'usage source binding mismatch' USING ERRCODE='23514';END IF;
 ELSE
  SELECT * INTO s FROM external_usage_source WHERE id=NEW.source_id;
  PERFORM 1 FROM external_identity_binding WHERE id=s.binding_id FOR UPDATE;
  SELECT * INTO s FROM external_usage_source WHERE id=NEW.source_id FOR UPDATE;
  IF s.id IS NULL OR s.workspace_id<>NEW.workspace_id THEN RAISE EXCEPTION 'usage workspace mismatch' USING ERRCODE='23514';END IF;
  IF TG_TABLE_NAME IN ('external_usage_call','external_usage_checkpoint','external_usage_resolution') THEN
   SELECT * INTO p FROM external_usage_pull WHERE id=NEW.pull_id;
   IF p.source_id IS DISTINCT FROM s.id OR p.workspace_id<>s.workspace_id THEN RAISE EXCEPTION 'usage pull mismatch' USING ERRCODE='23514';END IF;
  END IF;
  IF TG_TABLE_NAME IN ('external_usage_dispatch','external_usage_outcome','external_usage_event','external_usage_conflict','external_usage_checkpoint') THEN
   SELECT * INTO c FROM external_usage_call WHERE id=NEW.call_id;
   IF c.source_id IS DISTINCT FROM s.id OR c.workspace_id<>s.workspace_id THEN RAISE EXCEPTION 'usage call mismatch' USING ERRCODE='23514';END IF;
  END IF;
  IF TG_TABLE_NAME='external_usage_outcome' THEN
   IF NEW.state='PAGE' THEN
   SELECT * INTO p FROM external_usage_pull WHERE id=c.pull_id;
   IF NOT EXISTS(SELECT 1 FROM external_usage_dispatch WHERE call_id=c.id) OR NEW.page->>'requestHash' IS DISTINCT FROM c.request_hash OR NEW.page->>'instance' IS DISTINCT FROM s.instance_ref OR NEW.page->>'fromCursor' IS DISTINCT FROM p.from_cursor OR NEW.page->>'nextCursor' IS NULL OR length(NEW.page->>'nextCursor')>512 OR NEW.page->>'nextCursor' !~ '^[A-Za-z0-9._:-]*$' OR (jsonb_array_length(NEW.page->'events')>0 AND NEW.page->>'nextCursor'=p.from_cursor) THEN RAISE EXCEPTION 'usage page context mismatch' USING ERRCODE='23514';END IF;
   END IF;
  END IF;
  IF TG_TABLE_NAME='external_usage_event' THEN
   SELECT page->'events'->NEW.page_index INTO candidate FROM external_usage_outcome WHERE call_id=c.id AND state='PAGE';
   IF candidate IS NULL OR NEW.provider<>s.provider OR NEW.instance_ref<>s.instance_ref OR NEW.event_source<>s.stream OR candidate->>'id' IS DISTINCT FROM NEW.provider_event_id OR candidate->>'source' IS DISTINCT FROM NEW.event_source OR candidate->>'category' IS DISTINCT FROM NEW.category OR (candidate->>'occurredAt')::timestamptz IS DISTINCT FROM NEW.occurred_at OR candidate->>'externalSubjectId' IS DISTINCT FROM NEW.external_subject_id OR candidate->>'externalIdentifier' IS DISTINCT FROM NEW.external_identifier OR candidate->>'remoteId' IS DISTINCT FROM NEW.remote_id THEN RAISE EXCEPTION 'usage occurrence differs from received page' USING ERRCODE='23514';END IF;
  END IF;
  IF TG_TABLE_NAME IN ('external_usage_decision','external_usage_conflict') THEN
   SELECT * INTO e FROM external_usage_event WHERE id=NEW.event_id FOR UPDATE;
   IF e.source_id IS DISTINCT FROM s.id OR e.workspace_id<>s.workspace_id THEN RAISE EXCEPTION 'usage event mismatch' USING ERRCODE='23514';END IF;
   IF TG_TABLE_NAME='external_usage_decision' THEN
    IF EXISTS(SELECT 1 FROM external_usage_decision WHERE event_id=e.id AND status='ACCEPTED') THEN RAISE EXCEPTION 'accepted usage association is final' USING ERRCODE='23514';END IF;
    IF NEW.status='ACCEPTED' AND (e.category NOT IN ('USAGE','ACCESS','DELIVERY','CONNECTOR_LOG') OR e.external_subject_id<>s.external_subject_id OR NOT EXISTS(
      SELECT 1 FROM external_provider_observation o JOIN external_provider_call call ON call.id=o.call_id JOIN external_publication_attempt a ON a.id=call.attempt_id JOIN external_publication pub ON pub.id=a.publication_id JOIN external_publication_transition t ON t.publication_id=pub.id AND t.provider_observation_id=o.id AND t.to_status='PUBLISHED'
      WHERE o.id=NEW.observation_id AND o.state='PUBLISHED' AND a.workspace_id=s.workspace_id AND a.provider=s.provider AND a.instance_ref=s.instance_ref AND a.adapter_revision=s.adapter_revision AND a.binding_id=s.binding_id AND a.identifier_id=NEW.identifier_id AND a.publication_id=NEW.publication_id AND a.product_release_id=NEW.release_id AND NEW.binding_id=s.binding_id AND pub.status='PUBLISHED' AND a.external_subject_id=e.external_subject_id AND a.external_identifier=e.external_identifier AND o.remote_id=e.remote_id
    )) THEN RAISE EXCEPTION 'usage association lacks exact published receipt' USING ERRCODE='23514';END IF;
   ELSE
    SELECT page->'events'->NEW.page_index INTO candidate FROM external_usage_outcome WHERE call_id=c.id AND state='PAGE';
    IF candidate IS NULL OR candidate->>'id' IS DISTINCT FROM e.provider_event_id OR candidate->>'source' IS DISTINCT FROM e.event_source OR NEW.payload_hash=e.payload_hash OR NEW.payload_hash IS DISTINCT FROM encode(digest(concat_ws(E'\n',candidate->>'id',candidate->>'source',candidate->>'category',to_char((candidate->>'occurredAt')::timestamptz AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS.US"Z"'),candidate->>'externalSubjectId',candidate->>'externalIdentifier',candidate->>'remoteId'),'sha256'),'hex') THEN RAISE EXCEPTION 'usage conflict identity/hash mismatch' USING ERRCODE='23514';END IF;
   END IF;
  END IF;
  IF TG_TABLE_NAME='external_usage_checkpoint' THEN
   SELECT * INTO cp FROM external_usage_checkpoint WHERE source_id=s.id ORDER BY revision DESC LIMIT 1;
   IF NEW.revision<>COALESCE(cp.revision,0)+1 OR p.expected_revision<>COALESCE(cp.revision,0) OR p.from_cursor<>COALESCE(cp.cursor,'') OR c.pull_id<>p.id OR NOT EXISTS(SELECT 1 FROM external_usage_outcome WHERE call_id=c.id AND state='PAGE' AND page->>'nextCursor'=NEW.cursor) THEN RAISE EXCEPTION 'usage checkpoint is stale or not received' USING ERRCODE='23514';END IF;
   -- The cursor acknowledges the entire received page, including duplicate/conflicting entries.
   -- A durable occurrence decision (or conflict quarantine) must precede that acknowledgement.
   FOR candidate IN SELECT jsonb_array_elements(page->'events') FROM external_usage_outcome WHERE call_id=c.id AND state='PAGE' LOOP
    SELECT * INTO e FROM external_usage_event WHERE source_id=s.id AND provider_event_id=candidate->>'id' AND event_source=candidate->>'source';
    IF e.id IS NULL OR NOT EXISTS(SELECT 1 FROM external_usage_decision WHERE event_id=e.id) THEN RAISE EXCEPTION 'usage checkpoint would skip an occurrence decision' USING ERRCODE='23514';END IF;
    candidate_hash:=encode(digest(concat_ws(E'\n',candidate->>'id',candidate->>'source',candidate->>'category',to_char((candidate->>'occurredAt')::timestamptz AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS.US"Z"'),candidate->>'externalSubjectId',candidate->>'externalIdentifier',candidate->>'remoteId'),'sha256'),'hex');
    IF candidate_hash IS DISTINCT FROM e.payload_hash AND NOT EXISTS(SELECT 1 FROM external_usage_conflict WHERE event_id=e.id AND payload_hash=candidate_hash) THEN RAISE EXCEPTION 'usage checkpoint would skip conflict quarantine' USING ERRCODE='23514';END IF;
   END LOOP;
  END IF;
  IF TG_TABLE_NAME='external_usage_resolution' THEN
   IF NEW.outcome='IMPORTED' AND NOT EXISTS(SELECT 1 FROM external_usage_checkpoint WHERE pull_id=p.id) THEN RAISE EXCEPTION 'imported pull requires committed checkpoint' USING ERRCODE='23514';END IF;
  END IF;
 END IF;
 IF NOT EXISTS(SELECT 1 FROM evidence WHERE id=NEW.evidence_id AND workspace_id=NEW.workspace_id) THEN RAISE EXCEPTION 'usage evidence crosses workspace' USING ERRCODE='23514';END IF;
 RETURN NEW;
END $$;
DO $$ DECLARE t text; BEGIN
 FOREACH t IN ARRAY ARRAY['external_usage_source','external_usage_pull','external_usage_call','external_usage_dispatch','external_usage_outcome','external_usage_event','external_usage_decision','external_usage_conflict','external_usage_checkpoint','external_usage_resolution'] LOOP
  EXECUTE format('CREATE TRIGGER usage_reference BEFORE INSERT ON %I FOR EACH ROW EXECUTE FUNCTION guard_external_usage_reference()',t);
  EXECUTE format('CREATE TRIGGER usage_immutable BEFORE UPDATE OR DELETE ON %I FOR EACH ROW EXECUTE FUNCTION guard_external_immutable()',t);
 END LOOP;
END $$;

-- Every canonical occurrence is admitted together with its initial decision.
-- Deferral permits the parent-first transaction to insert the event before its child.
CREATE FUNCTION require_external_usage_decision() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NOT EXISTS(SELECT 1 FROM external_usage_decision WHERE event_id=NEW.id) THEN
  RAISE EXCEPTION 'usage occurrence requires an immutable decision' USING ERRCODE='23514';
 END IF;
 RETURN NULL;
END $$;
CREATE CONSTRAINT TRIGGER usage_requires_decision AFTER INSERT ON external_usage_event DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION require_external_usage_decision();

ALTER TABLE cost_allocation ADD COLUMN external_usage_call_id uuid REFERENCES external_usage_call(id);
ALTER TABLE cost_allocation DROP CONSTRAINT ck_cost_allocation_single_subject;
ALTER TABLE cost_allocation ADD CONSTRAINT ck_cost_allocation_single_subject CHECK(
 num_nonnulls(delivery_operation_id,quality_assessment_id,quality_assessment_attempt_id,
 rights_declaration_id,rights_declaration_verification_id,rights_declaration_disposition_id,
 authorization_provenance_binding_id,authorization_provenance_binding_disposition_id,
 effective_rights_snapshot_id,dataset_certification_id,certification_disposition_id,
 annotation_review_attempt_id,annotation_engine_attempt_id,ingestion_invocation_id,external_provider_call_id,external_usage_call_id)=1);
CREATE OR REPLACE FUNCTION guard_cost_allocation_workspace() RETURNS trigger AS $$
DECLARE event_workspace uuid; subject_workspace uuid;
BEGIN
 SELECT workspace_id INTO event_workspace FROM cost_event WHERE id=NEW.cost_event_id;
 IF NEW.external_usage_call_id IS NOT NULL THEN
  SELECT workspace_id INTO subject_workspace FROM external_usage_call WHERE id=NEW.external_usage_call_id;
  IF NOT EXISTS(SELECT 1 FROM cost_event e JOIN external_usage_dispatch d ON d.call_id=NEW.external_usage_call_id WHERE e.id=NEW.cost_event_id AND e.activity_id=NEW.external_usage_call_id AND e.cost_type='EXTERNAL_USAGE_INVOCATION' AND e.quantity=1 AND e.unit='invocation' AND e.pricing_mode='ACTUAL') THEN RAISE EXCEPTION 'usage cost requires dispatched physical call'; END IF;
 ELSIF NEW.external_provider_call_id IS NOT NULL THEN
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
