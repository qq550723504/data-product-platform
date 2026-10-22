-- A separate producer contract; no Annotation Result/Review/Gold tables.
CREATE TABLE source_admission_connection (
 id uuid PRIMARY KEY, workspace_id uuid NOT NULL, namespace_id uuid NOT NULL,
 project_id text NOT NULL CHECK(project_id ~ '^[1-9][0-9]*$'),
 organization_id text NOT NULL CHECK(organization_id ~ '^[1-9][0-9]*$'),
 binding_sha256 text NOT NULL CHECK(binding_sha256 ~ '^[0-9a-f]{64}$'),
 -- A source project may have multiple immutable operator-configured bindings.
 -- Source record uniqueness is enforced separately by namespace + source_id.
 mode text NOT NULL CHECK(mode='fork_approved'), enabled boolean NOT NULL DEFAULT true
);
CREATE TABLE source_admission_capture (
 id uuid PRIMARY KEY, workspace_id uuid NOT NULL, connection_id uuid NOT NULL REFERENCES source_admission_connection(id),
 dataset_id uuid NOT NULL REFERENCES dataset(id), version_id uuid NOT NULL UNIQUE REFERENCES dataset_version(id),
 release_id uuid NOT NULL, snapshot_id uuid NOT NULL,
 manifest_sha256 text NOT NULL CHECK(manifest_sha256 ~ '^[0-9a-f]{64}$'),
 content_digest text NOT NULL CHECK(content_digest ~ '^[0-9a-f]{64}$'),
 member_count integer NOT NULL CHECK(member_count BETWEEN 1 AND 10000),
 state text NOT NULL CHECK(state IN('PREPARED','ACCEPTED_SOURCE')),
 created_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(dataset_id,connection_id,snapshot_id)
);
-- Every typed DatasetVersion reference must participate in the existing
-- first-copy/reference fence, even while this relation is empty. Accepted
-- source history pins its evidence; release/disposal is outside this slice.
CREATE TRIGGER aa_lifecycle_ref_version_id BEFORE INSERT OR UPDATE OF version_id
 ON source_admission_capture FOR EACH ROW EXECUTE FUNCTION lifecycle_reference_admission_guard('version_id');

CREATE TABLE source_admission_operation (
 id uuid PRIMARY KEY, workspace_id uuid NOT NULL, actor_id uuid NOT NULL, principal_ref text NOT NULL,
 idempotency_key text NOT NULL CHECK(length(idempotency_key) BETWEEN 1 AND 255),
 connection_id uuid NOT NULL REFERENCES source_admission_connection(id), dataset_id uuid NOT NULL REFERENCES dataset(id),
 release_id uuid NOT NULL, snapshot_id uuid NOT NULL, manifest_sha256 text NOT NULL CHECK(manifest_sha256 ~ '^[0-9a-f]{64}$'),
 capture_id uuid REFERENCES source_admission_capture(id),
 state text NOT NULL CHECK(state IN('PREPARED','ACCEPTED_SOURCE','FAILED_FINAL')), error_code text,
 created_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(workspace_id,actor_id,idempotency_key),
 CONSTRAINT ck_source_admission_operation_outcome CHECK (
  (state='PREPARED' AND capture_id IS NULL AND error_code IS NULL) OR
  (state='ACCEPTED_SOURCE' AND capture_id IS NOT NULL AND error_code IS NULL) OR
  (state='FAILED_FINAL' AND capture_id IS NULL AND error_code IS NOT NULL AND
   error_code IN('SOURCE_INTEGRITY_FAILED','SOURCE_PROFILE_UNSUPPORTED','SOURCE_IDEMPOTENCY_CONFLICT','SOURCE_PUBLICATION_CONFLICT'))
 )
);
CREATE TABLE source_admission_asset (
 capture_id uuid NOT NULL REFERENCES source_admission_capture(id), name text NOT NULL CHECK(name IN('manifest.json','records.json','records.jsonl')),
 sha256 text NOT NULL CHECK(sha256 ~ '^[0-9a-f]{64}$'), byte_size bigint NOT NULL CHECK(byte_size BETWEEN 1 AND 33554432),
 storage_uri text, PRIMARY KEY(capture_id,name)
);
CREATE TABLE source_admission_identity (
 connection_id uuid NOT NULL REFERENCES source_admission_connection(id), namespace_id uuid NOT NULL,
 source_id text NOT NULL CHECK(source_id ~ '^[1-9][0-9]*$'),
 record_sha256 text NOT NULL CHECK(record_sha256 ~ '^[0-9a-f]{64}$'), PRIMARY KEY(namespace_id,source_id)
);
CREATE TABLE source_admission_member (
 capture_id uuid NOT NULL REFERENCES source_admission_capture(id), connection_id uuid NOT NULL,
 namespace_id uuid NOT NULL, source_id text NOT NULL, assignment_id text NOT NULL CHECK(assignment_id ~ '^[1-9][0-9]*$'),
 task_id text NOT NULL CHECK(task_id ~ '^[1-9][0-9]*$'), revision text NOT NULL CHECK(revision ~ '^[1-9][0-9]*$'),
 result_sha256 text NOT NULL CHECK(result_sha256 ~ '^[0-9a-f]{64}$'), record_sha256 text NOT NULL,
 PRIMARY KEY(capture_id,source_id), UNIQUE(capture_id,assignment_id),
 FOREIGN KEY(namespace_id,source_id) REFERENCES source_admission_identity(namespace_id,source_id)
);
CREATE TABLE source_admission_call (
 id uuid PRIMARY KEY, operation_id uuid NOT NULL REFERENCES source_admission_operation(id), workspace_id uuid NOT NULL,
 asset_name text NOT NULL, started_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE source_admission_observation (
 call_id uuid PRIMARY KEY REFERENCES source_admission_call(id), outcome text NOT NULL CHECK(outcome IN('RECEIVED','DENIED','UNKNOWN','INVALID')),
 status_code integer NOT NULL, occurred_at timestamptz NOT NULL DEFAULT now()
);

CREATE FUNCTION source_admission_guard() RETURNS trigger AS $$
DECLARE c source_admission_capture; op source_admission_operation; conn source_admission_connection; v dataset_version;
BEGIN
 IF TG_OP='DELETE' OR TG_OP='TRUNCATE' THEN RAISE EXCEPTION 'source admission history retained'; END IF;
 IF TG_TABLE_NAME='source_admission_connection' THEN
  IF TG_OP='UPDATE' AND (to_jsonb(NEW)-'enabled') IS DISTINCT FROM (to_jsonb(OLD)-'enabled') THEN RAISE EXCEPTION 'connection identity immutable'; END IF;
  RETURN NEW;
 END IF;
 IF TG_TABLE_NAME='source_admission_capture' THEN
  SELECT * INTO STRICT conn FROM source_admission_connection WHERE id=NEW.connection_id FOR SHARE;
  SELECT * INTO STRICT v FROM dataset_version WHERE id=NEW.version_id;
  IF conn.workspace_id<>NEW.workspace_id OR v.dataset_id<>NEW.dataset_id OR NOT EXISTS(SELECT 1 FROM dataset WHERE id=NEW.dataset_id AND workspace_id=NEW.workspace_id AND dataset_type='RAW') THEN RAISE EXCEPTION 'capture scope mismatch'; END IF;
  IF TG_OP='INSERT' AND (NEW.state<>'PREPARED' OR v.status<>'CREATED') THEN RAISE EXCEPTION 'direct accepted capture forbidden'; END IF;
  IF TG_OP='UPDATE' THEN
   IF OLD.state<>'PREPARED' OR NEW.state<>'ACCEPTED_SOURCE' OR (to_jsonb(NEW)-'state') IS DISTINCT FROM (to_jsonb(OLD)-'state') THEN RAISE EXCEPTION 'frozen capture'; END IF;
   IF v.status IS DISTINCT FROM 'READY' OR v.row_count IS DISTINCT FROM NEW.member_count OR
    v.content_type IS DISTINCT FROM 'application/x-ndjson' OR v.checksum_algorithm IS DISTINCT FROM 'SHA256' OR
    v.storage_type IS DISTINCT FROM 'OBJECT_STORAGE' OR
    (SELECT count(*) FROM source_admission_member WHERE capture_id=NEW.id)<>NEW.member_count OR
    (SELECT count(*) FROM source_admission_asset WHERE capture_id=NEW.id AND storage_uri IS NOT NULL)<>3 OR
    NOT EXISTS(SELECT 1 FROM source_admission_asset WHERE capture_id=NEW.id AND name='records.jsonl' AND storage_uri=v.storage_uri AND sha256=v.checksum_value AND byte_size=v.byte_size) OR
    NOT EXISTS(SELECT 1 FROM source_admission_asset WHERE capture_id=NEW.id AND name='manifest.json' AND sha256=NEW.manifest_sha256) THEN RAISE EXCEPTION 'incomplete source closure'; END IF;
  END IF;
  RETURN NEW;
 END IF;
 IF TG_TABLE_NAME='source_admission_operation' THEN
  SELECT * INTO STRICT conn FROM source_admission_connection WHERE id=NEW.connection_id;
  IF conn.workspace_id<>NEW.workspace_id OR NOT EXISTS(SELECT 1 FROM dataset WHERE id=NEW.dataset_id AND workspace_id=NEW.workspace_id AND dataset_type='RAW') THEN RAISE EXCEPTION 'operation scope mismatch'; END IF;
  IF TG_OP='INSERT' AND (NEW.state<>'PREPARED' OR NEW.capture_id IS NOT NULL) THEN RAISE EXCEPTION 'direct receipt forbidden'; END IF;
  IF TG_OP='UPDATE' THEN
   IF OLD.state<>'PREPARED' OR NEW.state NOT IN('ACCEPTED_SOURCE','FAILED_FINAL') OR (to_jsonb(NEW)-ARRAY['state','capture_id','error_code']) IS DISTINCT FROM (to_jsonb(OLD)-ARRAY['state','capture_id','error_code']) THEN RAISE EXCEPTION 'frozen operation'; END IF;
   IF NEW.state='FAILED_FINAL' THEN
    IF NEW.capture_id IS NOT NULL OR NEW.error_code IS NULL OR NEW.error_code NOT IN('SOURCE_INTEGRITY_FAILED','SOURCE_PROFILE_UNSUPPORTED','SOURCE_IDEMPOTENCY_CONFLICT','SOURCE_PUBLICATION_CONFLICT') THEN RAISE EXCEPTION 'invalid terminal rejection'; END IF;
    RETURN NEW;
   END IF;
   IF NEW.error_code IS NOT NULL THEN RAISE EXCEPTION 'accepted error code forbidden'; END IF;
   SELECT * INTO STRICT c FROM source_admission_capture WHERE id=NEW.capture_id;
   IF c.state<>'ACCEPTED_SOURCE' OR c.workspace_id<>NEW.workspace_id OR c.dataset_id<>NEW.dataset_id OR c.connection_id<>NEW.connection_id OR c.release_id<>NEW.release_id OR c.snapshot_id<>NEW.snapshot_id OR c.manifest_sha256<>NEW.manifest_sha256 THEN RAISE EXCEPTION 'receipt binding mismatch'; END IF;
  END IF; RETURN NEW;
 END IF;
 IF TG_TABLE_NAME IN('source_admission_asset','source_admission_member') THEN
  SELECT * INTO STRICT c FROM source_admission_capture WHERE id=NEW.capture_id FOR UPDATE;
  IF c.state<>'PREPARED' THEN RAISE EXCEPTION 'sealed source membership'; END IF;
  IF TG_TABLE_NAME='source_admission_asset' THEN
   IF TG_OP='UPDATE' AND (OLD.storage_uri IS NOT NULL OR NEW.storage_uri IS NULL OR (to_jsonb(NEW)-'storage_uri') IS DISTINCT FROM (to_jsonb(OLD)-'storage_uri')) THEN RAISE EXCEPTION 'immutable asset binding'; END IF;
   IF TG_OP='INSERT' AND NEW.storage_uri IS NOT NULL THEN RAISE EXCEPTION 'unverified asset binding'; END IF;
  ELSE
   IF TG_OP<>'INSERT' OR NEW.connection_id<>c.connection_id OR NOT EXISTS(SELECT 1 FROM source_admission_connection WHERE id=c.connection_id AND namespace_id=NEW.namespace_id) OR NOT EXISTS(SELECT 1 FROM source_admission_identity WHERE namespace_id=NEW.namespace_id AND source_id=NEW.source_id AND record_sha256=NEW.record_sha256) THEN RAISE EXCEPTION 'immutable source identity mismatch'; END IF;
  END IF; RETURN NEW;
 END IF;
 IF TG_OP<>'INSERT' THEN RAISE EXCEPTION 'append only source fact'; END IF;
 IF TG_TABLE_NAME='source_admission_identity' THEN
  IF NOT EXISTS(SELECT 1 FROM source_admission_connection WHERE id=NEW.connection_id AND namespace_id=NEW.namespace_id) THEN RAISE EXCEPTION 'source namespace mismatch'; END IF;
 END IF;
 IF TG_TABLE_NAME='source_admission_call' THEN
  SELECT * INTO STRICT op FROM source_admission_operation WHERE id=NEW.operation_id;
  IF op.workspace_id<>NEW.workspace_id THEN RAISE EXCEPTION 'source call workspace mismatch'; END IF;
 END IF;
 RETURN NEW;
END; $$ LANGUAGE plpgsql;
DO $$ DECLARE n text; BEGIN
 FOREACH n IN ARRAY ARRAY['source_admission_connection','source_admission_capture','source_admission_operation','source_admission_asset','source_admission_identity','source_admission_member','source_admission_call','source_admission_observation'] LOOP
  EXECUTE format('CREATE TRIGGER source_admission_scope BEFORE INSERT OR UPDATE OR DELETE ON %I FOR EACH ROW EXECUTE FUNCTION source_admission_guard()',n);
  EXECUTE format('CREATE TRIGGER source_admission_no_truncate BEFORE TRUNCATE ON %I FOR EACH STATEMENT EXECUTE FUNCTION source_admission_guard()',n);
 END LOOP;
END; $$;

ALTER TABLE cost_allocation ADD COLUMN source_admission_call_id uuid REFERENCES source_admission_call(id);
CREATE UNIQUE INDEX source_admission_cost ON cost_allocation(source_admission_call_id) WHERE source_admission_call_id IS NOT NULL;
DO $$ DECLARE definition text; guard text; BEGIN
 SELECT pg_get_constraintdef(oid) INTO definition FROM pg_constraint WHERE conrelid='cost_allocation'::regclass AND conname='ck_cost_allocation_single_subject';
 IF definition NOT LIKE '%num_nonnulls(%' THEN RAISE EXCEPTION 'unsupported cost allocation contract'; END IF;
 ALTER TABLE cost_allocation DROP CONSTRAINT ck_cost_allocation_single_subject;
 EXECUTE 'ALTER TABLE cost_allocation ADD CONSTRAINT ck_cost_allocation_single_subject '||replace(definition,'num_nonnulls(','num_nonnulls(source_admission_call_id,');
 SELECT pg_get_functiondef('guard_cost_allocation_workspace()'::regprocedure) INTO definition;
 definition:=replace(definition,E'\r\n',E'\n');
 guard:=E'\n-- source-admission cost begin\n IF NEW.source_admission_call_id IS NOT NULL THEN\n  SELECT workspace_id INTO event_workspace FROM cost_event WHERE id=NEW.cost_event_id;\n  SELECT workspace_id INTO subject_workspace FROM source_admission_call WHERE id=NEW.source_admission_call_id;\n  IF event_workspace IS DISTINCT FROM subject_workspace OR NOT EXISTS(SELECT 1 FROM cost_event WHERE id=NEW.cost_event_id AND activity_id=NEW.source_admission_call_id AND cost_type=''SOURCE_ADMISSION_FETCH'' AND quantity=1 AND unit=''invocation'' AND pricing_mode=''ACTUAL'') THEN RAISE EXCEPTION ''invalid source admission call allocation''; END IF;\n  RETURN NEW;\n END IF;\n-- source-admission cost end\n';
 IF position(E'\nBEGIN\n' IN definition)=0 THEN RAISE EXCEPTION 'unsupported cost guard'; END IF;
 EXECUTE replace(definition,E'\nBEGIN\n',E'\nBEGIN\n'||guard);
END; $$;
