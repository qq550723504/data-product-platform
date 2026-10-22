-- #325: exact two-member operator acquisition, never a protected-copy identity.
CREATE TABLE operator_custody_acquisition (
 id uuid PRIMARY KEY, workspace_id uuid NOT NULL, actor_id uuid NOT NULL,
 principal_ref varchar(255) NOT NULL CHECK(principal_ref<>''),
 run_id varchar(128) NOT NULL CHECK(run_id<>''),
 request_key varchar(128) NOT NULL CHECK(request_key<>''),
 request_hash char(64) NOT NULL CHECK(request_hash ~ '^[0-9a-f]{64}$'),
 plan_sha256 char(64) NOT NULL CHECK(plan_sha256 ~ '^[0-9a-f]{64}$'),
 provider_witness varchar(255) NOT NULL CHECK(provider_witness<>''),
 endpoint varchar(255) NOT NULL, output_directory text NOT NULL,
 first_version_id uuid NOT NULL REFERENCES dataset_version(id),
 first_uri text NOT NULL, first_sha256 char(64) NOT NULL CHECK(first_sha256 ~ '^[0-9a-f]{64}$'),
 first_bytes bigint NOT NULL CHECK(first_bytes BETWEEN 1 AND 134217728),
 second_version_id uuid NOT NULL REFERENCES dataset_version(id),
 second_uri text NOT NULL, second_sha256 char(64) NOT NULL CHECK(second_sha256 ~ '^[0-9a-f]{64}$'),
 second_bytes bigint NOT NULL CHECK(second_bytes BETWEEN 1 AND 134217728),
 prepared_at timestamptz NOT NULL, expires_at timestamptz NOT NULL,
 dependency_revision bigint NOT NULL CHECK(dependency_revision>=0),
 CHECK(first_version_id<>second_version_id AND first_uri<>second_uri),
 CHECK(expires_at>prepared_at AND expires_at<=prepared_at+interval '1 hour'),
 UNIQUE(workspace_id,request_key), UNIQUE(workspace_id,id)
);
CREATE TRIGGER aa_lifecycle_ref_first_version_id BEFORE INSERT OR UPDATE OF first_version_id
 ON operator_custody_acquisition FOR EACH ROW EXECUTE FUNCTION lifecycle_reference_admission_guard('first_version_id');
CREATE TRIGGER aa_lifecycle_ref_second_version_id BEFORE INSERT OR UPDATE OF second_version_id
 ON operator_custody_acquisition FOR EACH ROW EXECUTE FUNCTION lifecycle_reference_admission_guard('second_version_id');

CREATE TABLE operator_custody_call (
 id uuid PRIMARY KEY, workspace_id uuid NOT NULL, acquisition_id uuid NOT NULL,
 member_no smallint NOT NULL CHECK(member_no IN (1,2)),
 started_at timestamptz NOT NULL, dependency_revision bigint NOT NULL CHECK(dependency_revision>=0),
 FOREIGN KEY(workspace_id,acquisition_id) REFERENCES operator_custody_acquisition(workspace_id,id),
 UNIQUE(acquisition_id,member_no), UNIQUE(workspace_id,id)
);
CREATE TABLE operator_custody_observation (
 id uuid PRIMARY KEY, workspace_id uuid NOT NULL, call_id uuid NOT NULL UNIQUE,
 invoked_at timestamptz NOT NULL, observed_at timestamptz NOT NULL,
 invocation_certainty text NOT NULL CHECK(invocation_certainty IN ('CONFIRMED','UNKNOWN')),
 http_status integer NOT NULL CHECK(http_status=0 OR http_status BETWEEN 100 AND 599),
 byte_size bigint NOT NULL CHECK(byte_size>=0), body_sha256 text NOT NULL,
 eof boolean NOT NULL, close_ok boolean NOT NULL,
 observation_sha256 char(64) NOT NULL CHECK(observation_sha256 ~ '^[0-9a-f]{64}$'),
 failure text NOT NULL CHECK(failure IN ('','TRANSPORT','BODY','CONTENT','HTTP')),
 FOREIGN KEY(workspace_id,call_id) REFERENCES operator_custody_call(workspace_id,id),
 CHECK(observed_at>=invoked_at), CHECK(body_sha256='' OR body_sha256 ~ '^[0-9a-f]{64}$'),
 CHECK(failure<>'' OR (http_status=200 AND eof AND close_ok AND body_sha256<>'' AND invocation_certainty='CONFIRMED'))
);
CREATE TABLE operator_custody_termination (
 id uuid PRIMARY KEY, workspace_id uuid NOT NULL, acquisition_id uuid NOT NULL UNIQUE,
 outcome text NOT NULL CHECK(outcome IN ('ACCEPTED','FAILED')),
 owner_verified_quiescent boolean NOT NULL CHECK(owner_verified_quiescent),
 manifest_sha256 text NOT NULL, occurred_at timestamptz NOT NULL,
 dependency_revision bigint NOT NULL CHECK(dependency_revision>=0),
 FOREIGN KEY(workspace_id,acquisition_id) REFERENCES operator_custody_acquisition(workspace_id,id),
 CHECK((outcome='ACCEPTED' AND manifest_sha256 ~ '^[0-9a-f]{64}$') OR (outcome='FAILED' AND manifest_sha256=''))
);

CREATE FUNCTION operator_custody_scope_guard() RETURNS trigger AS $$
DECLARE a operator_custody_acquisition; physical operator_custody_call; n integer;
BEGIN
 IF TG_TABLE_NAME='operator_custody_acquisition' THEN
  IF NOT EXISTS(SELECT 1 FROM dataset_version v JOIN dataset d ON d.id=v.dataset_id
   WHERE v.id=NEW.first_version_id AND d.workspace_id=NEW.workspace_id AND d.dataset_type='STANDARDIZED'
   AND v.storage_uri=NEW.first_uri AND v.checksum_algorithm='SHA256' AND v.checksum_value=NEW.first_sha256 AND v.byte_size=NEW.first_bytes AND v.status IN ('READY','SUPERSEDED'))
   OR NOT EXISTS(SELECT 1 FROM dataset_version v JOIN dataset d ON d.id=v.dataset_id
   WHERE v.id=NEW.second_version_id AND d.workspace_id=NEW.workspace_id AND d.dataset_type='CURATED'
   AND v.storage_uri=NEW.second_uri AND v.checksum_algorithm='SHA256' AND v.checksum_value=NEW.second_sha256 AND v.byte_size=NEW.second_bytes AND v.status IN ('READY','SUPERSEDED'))
  THEN RAISE EXCEPTION 'custody source identity/workspace mismatch'; END IF;
  RETURN NEW;
 ELSIF TG_TABLE_NAME='operator_custody_observation' THEN
  SELECT * INTO STRICT physical FROM operator_custody_call WHERE id=NEW.call_id AND workspace_id=NEW.workspace_id;
  IF NEW.invoked_at<physical.started_at THEN RAISE EXCEPTION 'custody observation predates start'; END IF;
  RETURN NEW; -- Observations survive authority expiry/restriction.
 END IF;
 SELECT * INTO STRICT a FROM operator_custody_acquisition WHERE id=NEW.acquisition_id AND workspace_id=NEW.workspace_id;
 PERFORM lifecycle_reference_lock(a.workspace_id,a.first_version_id);
 PERFORM lifecycle_reference_lock(a.workspace_id,a.second_version_id);
 PERFORM 1 FROM operator_custody_acquisition WHERE id=a.id FOR UPDATE;
 IF EXISTS(SELECT 1 FROM operator_custody_termination WHERE acquisition_id=a.id) THEN
  RAISE EXCEPTION 'custody acquisition already terminated'; END IF;
 IF TG_TABLE_NAME='operator_custody_call' THEN
  IF NEW.started_at<a.prepared_at OR NEW.started_at>=a.expires_at THEN RAISE EXCEPTION 'custody call outside authority'; END IF;
 ELSE
  IF EXISTS(SELECT 1 FROM operator_custody_call c WHERE c.acquisition_id=a.id AND NOT EXISTS(SELECT 1 FROM operator_custody_observation o WHERE o.call_id=c.id)) THEN
   RAISE EXCEPTION 'custody call outcome/owner remains unknown'; END IF;
  IF NEW.outcome='ACCEPTED' THEN
   SELECT count(*) INTO n FROM operator_custody_call c JOIN operator_custody_observation o ON o.call_id=c.id
    WHERE c.acquisition_id=a.id AND o.failure='' AND o.http_status=200 AND o.eof AND o.close_ok
    AND o.byte_size=CASE c.member_no WHEN 1 THEN a.first_bytes ELSE a.second_bytes END
    AND o.body_sha256=CASE c.member_no WHEN 1 THEN a.first_sha256 ELSE a.second_sha256 END
    AND EXISTS(SELECT 1 FROM cost_allocation ca JOIN cost_event ce ON ce.id=ca.cost_event_id
     WHERE ca.operator_custody_call_id=c.id AND ce.workspace_id=c.workspace_id AND ce.activity_id=c.id
     AND ce.cost_type='OPERATOR_CUSTODY_INVOCATION' AND ce.occurred_at=o.invoked_at);
   IF n<>2 OR NEW.occurred_at>=a.expires_at THEN RAISE EXCEPTION 'custody verified calls/authority missing'; END IF;
  END IF;
 END IF;
 RETURN NEW;
END; $$ LANGUAGE plpgsql;
DO $$ DECLARE n text; BEGIN
 FOREACH n IN ARRAY ARRAY['operator_custody_acquisition','operator_custody_call','operator_custody_observation','operator_custody_termination'] LOOP
  EXECUTE format('CREATE TRIGGER custody_scope BEFORE INSERT ON %I FOR EACH ROW EXECUTE FUNCTION operator_custody_scope_guard()',n);
  EXECUTE format('CREATE TRIGGER custody_immutable BEFORE UPDATE OR DELETE ON %I FOR EACH ROW EXECUTE FUNCTION lifecycle_fixture_identity_immutable()',n);
  EXECUTE format('CREATE TRIGGER custody_no_truncate BEFORE TRUNCATE ON %I FOR EACH STATEMENT EXECUTE FUNCTION lifecycle_fixture_identity_immutable()',n);
 END LOOP;
END; $$;

ALTER TABLE cost_allocation ADD COLUMN operator_custody_call_id uuid REFERENCES operator_custody_call(id);
CREATE UNIQUE INDEX operator_custody_cost ON cost_allocation(operator_custody_call_id) WHERE operator_custody_call_id IS NOT NULL;
DO $$
DECLARE definition text; guard text;
BEGIN
 SELECT pg_get_constraintdef(oid) INTO definition FROM pg_constraint WHERE conrelid='cost_allocation'::regclass AND conname='ck_cost_allocation_single_subject';
 IF definition NOT LIKE '%num_nonnulls(%' THEN RAISE EXCEPTION 'unsupported cost subject contract'; END IF;
 ALTER TABLE cost_allocation DROP CONSTRAINT ck_cost_allocation_single_subject;
 EXECUTE 'ALTER TABLE cost_allocation ADD CONSTRAINT ck_cost_allocation_single_subject ' || replace(definition,'num_nonnulls(','num_nonnulls(operator_custody_call_id,');
 SELECT pg_get_functiondef('guard_cost_allocation_workspace()'::regprocedure) INTO definition;
 definition:=replace(definition,E'\r\n',E'\n');
 guard := E'\n IF NEW.operator_custody_call_id IS NOT NULL THEN\n  SELECT workspace_id INTO event_workspace FROM cost_event WHERE id=NEW.cost_event_id;\n  SELECT workspace_id INTO subject_workspace FROM operator_custody_call WHERE id=NEW.operator_custody_call_id;\n  IF event_workspace IS DISTINCT FROM subject_workspace OR NOT EXISTS(SELECT 1 FROM cost_event e JOIN operator_custody_observation o ON o.call_id=e.activity_id WHERE e.id=NEW.cost_event_id AND e.activity_id=NEW.operator_custody_call_id AND e.workspace_id=o.workspace_id AND e.cost_type=''OPERATOR_CUSTODY_INVOCATION'' AND e.quantity=1 AND e.unit=''invocation'' AND e.pricing_mode=''ACTUAL'' AND e.amount IS NULL AND e.currency IS NULL AND e.occurred_at=o.invoked_at) THEN RAISE EXCEPTION ''invalid custody physical call allocation''; END IF;\n  RETURN NEW;\n END IF;\n';
 IF position(E'\nBEGIN\n' IN definition)=0 THEN RAISE EXCEPTION 'unsupported cost workspace contract'; END IF;
 EXECUTE replace(definition,E'\nBEGIN\n',E'\nBEGIN\n'||guard);
END; $$;
