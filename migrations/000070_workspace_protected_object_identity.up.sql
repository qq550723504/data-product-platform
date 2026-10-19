-- #330: one physical inventory, with an explicit non-versioned identity.
-- Existing exact-version identities and all historical hashes remain unchanged.
ALTER TABLE lifecycle_fixture_namespace
 ADD COLUMN identity_kind varchar(32) NOT NULL DEFAULT 'PROVIDER_VERSION',
 ADD COLUMN object_prefix varchar(1024) NOT NULL DEFAULT '',
 ADD CONSTRAINT lifecycle_namespace_identity_kind CHECK (
  (identity_kind='PROVIDER_VERSION' AND object_prefix='') OR
  (identity_kind='PROTECTED_CREATE' AND object_prefix='core/'||workspace_id::text||'/')
 );
DO $$ DECLARE n text; BEGIN
 SELECT conname INTO STRICT n FROM pg_constraint
 WHERE conrelid='lifecycle_fixture_namespace'::regclass AND contype='u'
 AND pg_get_constraintdef(oid)='UNIQUE (provider_instance_id, incarnation_id, namespace)';
 EXECUTE format('ALTER TABLE lifecycle_fixture_namespace DROP CONSTRAINT %I',n);
END; $$;
ALTER TABLE lifecycle_fixture_namespace ADD UNIQUE(provider_instance_id,incarnation_id,namespace,object_prefix);
-- A full-bucket versioned namespace cannot overlap a protected workspace
-- prefix. Serialize even registrations in different workspaces before testing.
CREATE FUNCTION lifecycle_namespace_prefix_guard() RETURNS trigger AS $$
BEGIN
 INSERT INTO delivery_authorization_fence(workspace_id) VALUES(NEW.workspace_id) ON CONFLICT DO NOTHING;
 PERFORM 1 FROM delivery_authorization_fence WHERE workspace_id=NEW.workspace_id FOR UPDATE;
 PERFORM pg_advisory_xact_lock(hashtextextended(NEW.provider_instance_id::text||':'||NEW.incarnation_id::text||':'||NEW.namespace,0));
 IF EXISTS(SELECT 1 FROM lifecycle_fixture_namespace n WHERE n.provider_instance_id=NEW.provider_instance_id
  AND n.incarnation_id=NEW.incarnation_id AND n.namespace=NEW.namespace AND n.id<>NEW.id AND
  (n.object_prefix='' OR NEW.object_prefix='' OR n.object_prefix=NEW.object_prefix)) THEN
  RAISE EXCEPTION 'overlapping physical namespace ownership'; END IF;
 RETURN NEW;
END; $$ LANGUAGE plpgsql;
CREATE TRIGGER aa_lifecycle_namespace_prefix BEFORE INSERT ON lifecycle_fixture_namespace
 FOR EACH ROW EXECUTE FUNCTION lifecycle_namespace_prefix_guard();

CREATE TABLE lifecycle_protected_object_intent (
 id uuid PRIMARY KEY CHECK(id<>'00000000-0000-0000-0000-000000000000'),
 workspace_id uuid NOT NULL,
 namespace_id uuid NOT NULL,
 dataset_version_id uuid NOT NULL REFERENCES dataset_version(id),
 purpose varchar(20) NOT NULL CHECK(purpose IN('CREATE','RETAINED_COPY','SOURCE_READ')),
 object_key varchar(1024) NOT NULL CHECK(object_key<>'' AND object_key=btrim(object_key)),
 content_sha256 varchar(64) NOT NULL CHECK(content_sha256 ~ '^[0-9a-f]{64}$'),
 byte_size bigint NOT NULL CHECK(byte_size BETWEEN 1 AND 134217728),
 content_type varchar(255) NOT NULL CHECK(content_type<>''),
 actor_id uuid NOT NULL CHECK(actor_id<>'00000000-0000-0000-0000-000000000000'),
 principal_ref varchar(255) NOT NULL CHECK(principal_ref<>''),
 dependency_revision bigint NOT NULL CHECK(dependency_revision>=0),
 created_at timestamptz NOT NULL,
 FOREIGN KEY(workspace_id,namespace_id) REFERENCES lifecycle_fixture_namespace(workspace_id,id),
 UNIQUE(workspace_id,id), UNIQUE(namespace_id,object_key,purpose)
);
-- This relation participates in the same source/first-copy admission fence.
CREATE TRIGGER aa_lifecycle_ref_dataset_version_id
 BEFORE INSERT OR UPDATE OF dataset_version_id ON lifecycle_protected_object_intent
 FOR EACH ROW EXECUTE FUNCTION lifecycle_reference_admission_guard('dataset_version_id');
CREATE FUNCTION lifecycle_protected_intent_guard() RETURNS trigger AS $$
DECLARE n lifecycle_fixture_namespace; w uuid;
BEGIN
 SELECT * INTO STRICT n FROM lifecycle_fixture_namespace WHERE id=NEW.namespace_id AND workspace_id=NEW.workspace_id;
 SELECT d.workspace_id INTO STRICT w FROM dataset_version v JOIN dataset d ON d.id=v.dataset_id WHERE v.id=NEW.dataset_version_id;
 IF w<>NEW.workspace_id OR n.identity_kind<>'PROTECTED_CREATE' OR
 left(NEW.object_key,length(n.object_prefix))<>n.object_prefix OR
 NEW.object_key ~ '(^|/)[.]{1,2}(/|$)' OR NEW.object_key LIKE '%//%' OR
 position('%' in NEW.object_key)>0 OR position(chr(92) in NEW.object_key)>0 THEN RAISE EXCEPTION 'protected object scope mismatch'; END IF;
 RETURN NEW;
END; $$ LANGUAGE plpgsql;
CREATE TRIGGER lifecycle_protected_intent_scope BEFORE INSERT ON lifecycle_protected_object_intent
 FOR EACH ROW EXECUTE FUNCTION lifecycle_protected_intent_guard();

-- Reuse the existing physical HTTP start/observation/cost journal.
ALTER TABLE lifecycle_storage_call ALTER COLUMN copy_id DROP NOT NULL;
ALTER TABLE lifecycle_storage_call ADD COLUMN protected_object_intent_id uuid;
ALTER TABLE lifecycle_storage_call ADD FOREIGN KEY(workspace_id,protected_object_intent_id)
 REFERENCES lifecycle_protected_object_intent(workspace_id,id);
ALTER TABLE lifecycle_storage_call DROP CONSTRAINT lifecycle_storage_call_check;
ALTER TABLE lifecycle_storage_call ADD CONSTRAINT lifecycle_storage_call_check CHECK(
 (operation IN('STAT_EXACT','INSPECT_RETENTION','INSPECT_LEGAL_HOLD','LIST_NAMESPACE_VERSIONS','READ_CHECKPOINT_EXACT')
  AND copy_operation_id IS NULL AND reader_pin_id IS NULL AND processing_output_intent_id IS NULL)
 OR(operation IN('ARCHIVE_COPY','RESTORE_COPY','VERIFY_COPY_BYTES','RECOVER_COPY_VERSIONS','RECOVER_COPY_EXACT')
  AND copy_operation_id IS NOT NULL AND reader_pin_id IS NULL AND processing_output_intent_id IS NULL)
 OR(operation='READ_PINNED' AND reader_pin_id IS NOT NULL AND copy_operation_id IS NULL AND processing_output_intent_id IS NULL)
 OR(operation='PROCESSING_PUT' AND processing_output_intent_id IS NOT NULL AND reader_pin_id IS NULL AND copy_operation_id IS NULL)
 OR(operation IN('PROTECTED_PUT','PROTECTED_GET','PROTECTED_HEAD') AND protected_object_intent_id IS NOT NULL
  AND copy_operation_id IS NULL AND reader_pin_id IS NULL AND processing_output_intent_id IS NULL));
ALTER TABLE lifecycle_storage_call ADD CONSTRAINT lifecycle_storage_call_identity CHECK(
 (protected_object_intent_id IS NULL AND copy_id IS NOT NULL AND operation NOT LIKE 'PROTECTED_%') OR
 (protected_object_intent_id IS NOT NULL AND copy_id IS NULL AND operation IN('PROTECTED_PUT','PROTECTED_GET','PROTECTED_HEAD')
 AND copy_operation_id IS NULL AND reader_pin_id IS NULL AND processing_output_intent_id IS NULL));

CREATE TABLE lifecycle_protected_object_verification (
 id uuid PRIMARY KEY,
 workspace_id uuid NOT NULL,
 intent_id uuid NOT NULL,
 create_call_id uuid NOT NULL,
 read_call_id uuid NOT NULL,
 content_sha256 varchar(64) NOT NULL CHECK(content_sha256 ~ '^[0-9a-f]{64}$'),
 byte_size bigint NOT NULL CHECK(byte_size BETWEEN 1 AND 134217728),
 verified_at timestamptz NOT NULL,
 creation_observation varchar(16) NOT NULL CHECK(creation_observation IN('CONFIRMED','RECONCILED')),
 FOREIGN KEY(workspace_id,intent_id) REFERENCES lifecycle_protected_object_intent(workspace_id,id),
 FOREIGN KEY(workspace_id,create_call_id) REFERENCES lifecycle_storage_call(workspace_id,id),
 FOREIGN KEY(workspace_id,read_call_id) REFERENCES lifecycle_storage_call(workspace_id,id),
 UNIQUE(intent_id), UNIQUE(workspace_id,id)
);
CREATE FUNCTION lifecycle_protected_verification_guard() RETURNS trigger AS $$
DECLARE i lifecycle_protected_object_intent;
BEGIN
 SELECT * INTO STRICT i FROM lifecycle_protected_object_intent WHERE id=NEW.intent_id AND workspace_id=NEW.workspace_id;
 PERFORM lifecycle_reference_lock(i.workspace_id,i.dataset_version_id);
 IF i.purpose='SOURCE_READ' OR i.content_sha256<>NEW.content_sha256 OR i.byte_size<>NEW.byte_size THEN
  RAISE EXCEPTION 'protected verification identity mismatch'; END IF;
 IF NOT EXISTS(SELECT 1 FROM lifecycle_storage_call c WHERE c.id=NEW.create_call_id
  AND c.protected_object_intent_id=i.id AND c.operation='PROTECTED_PUT'
  AND NOT EXISTS(SELECT 1 FROM lifecycle_storage_observation o WHERE o.call_id=c.id AND o.http_status BETWEEN 400 AND 599 AND o.http_status<>412)) THEN
  RAISE EXCEPTION 'protected creation start missing'; END IF;
 IF (NEW.creation_observation='CONFIRMED') IS DISTINCT FROM EXISTS(
  SELECT 1 FROM lifecycle_storage_observation o WHERE o.call_id=NEW.create_call_id AND o.http_status=200 AND o.failure='') THEN
  RAISE EXCEPTION 'protected creation observation mismatch'; END IF;
 IF NOT EXISTS(SELECT 1 FROM lifecycle_storage_call c JOIN lifecycle_storage_observation o ON o.call_id=c.id
  WHERE c.id=NEW.read_call_id AND c.protected_object_intent_id=i.id AND c.operation='PROTECTED_GET'
  AND o.http_status=200 AND o.failure='' AND o.invoked_at IS NOT NULL
  AND o.response_sha256=i.content_sha256 AND o.observed_at<=NEW.verified_at) THEN
  RAISE EXCEPTION 'protected native readback proof missing'; END IF;
 RETURN NEW;
END; $$ LANGUAGE plpgsql;
CREATE TRIGGER lifecycle_protected_verification_scope BEFORE INSERT ON lifecycle_protected_object_verification
 FOR EACH ROW EXECUTE FUNCTION lifecycle_protected_verification_guard();

ALTER TABLE lifecycle_fixture_copy ALTER COLUMN provider_version DROP NOT NULL;
ALTER TABLE lifecycle_fixture_copy DROP CONSTRAINT lifecycle_fixture_copy_provider_version_check;
ALTER TABLE lifecycle_fixture_copy
 ADD COLUMN identity_kind varchar(32) NOT NULL DEFAULT 'PROVIDER_VERSION',
 ADD COLUMN protected_verification_id uuid,
 ADD FOREIGN KEY(workspace_id,protected_verification_id) REFERENCES lifecycle_protected_object_verification(workspace_id,id),
 ADD CONSTRAINT lifecycle_copy_identity_kind CHECK(
  (identity_kind='PROVIDER_VERSION' AND provider_version IS NOT NULL AND provider_version NOT IN('','null')
   AND provider_version=btrim(provider_version) AND protected_verification_id IS NULL) OR
  (identity_kind='PROTECTED_CREATE' AND provider_version IS NULL AND protected_verification_id IS NOT NULL));
CREATE UNIQUE INDEX lifecycle_protected_copy_key ON lifecycle_fixture_copy(namespace_id,object_key)
 WHERE identity_kind='PROTECTED_CREATE';
CREATE FUNCTION lifecycle_protected_copy_guard() RETURNS trigger AS $$
DECLARE n lifecycle_fixture_namespace; i lifecycle_protected_object_intent;
BEGIN
 SELECT * INTO STRICT n FROM lifecycle_fixture_namespace WHERE id=NEW.namespace_id AND workspace_id=NEW.workspace_id;
 IF n.identity_kind<>NEW.identity_kind THEN RAISE EXCEPTION 'namespace/copy identity mismatch'; END IF;
 IF NEW.identity_kind='PROVIDER_VERSION' THEN RETURN NEW; END IF;
 SELECT a.* INTO STRICT i FROM lifecycle_protected_object_verification v
 JOIN lifecycle_protected_object_intent a ON a.id=v.intent_id
 WHERE v.id=NEW.protected_verification_id AND v.workspace_id=NEW.workspace_id;
 IF i.dataset_version_id<>NEW.dataset_version_id OR i.namespace_id<>NEW.namespace_id OR
 i.object_key<>NEW.object_key OR i.content_sha256<>NEW.content_sha256 OR i.byte_size<>NEW.byte_size THEN
  RAISE EXCEPTION 'protected copy verification mismatch'; END IF;
 RETURN NEW;
END; $$ LANGUAGE plpgsql;
CREATE TRIGGER lifecycle_protected_copy_scope BEFORE INSERT ON lifecycle_fixture_copy
 FOR EACH ROW EXECUTE FUNCTION lifecycle_protected_copy_guard();

-- Protected locations are not exact-provider-version disposal capabilities.
CREATE FUNCTION lifecycle_versioned_plan_target_guard() RETURNS trigger AS $$
DECLARE subject uuid;
BEGIN
 IF TG_TABLE_NAME='lifecycle_copy_operation' THEN subject:=NEW.source_copy_id; ELSE subject:=NEW.copy_id; END IF;
 IF NOT EXISTS(SELECT 1 FROM lifecycle_fixture_copy WHERE id=subject AND workspace_id=NEW.workspace_id
 AND identity_kind='PROVIDER_VERSION') THEN RAISE EXCEPTION 'protected object destructive capability unsupported'; END IF;
 RETURN NEW;
END; $$ LANGUAGE plpgsql;
CREATE TRIGGER aa_lifecycle_versioned_target BEFORE INSERT ON lifecycle_disposition_plan_target
 FOR EACH ROW EXECUTE FUNCTION lifecycle_versioned_plan_target_guard();
CREATE TRIGGER aa_lifecycle_versioned_source BEFORE INSERT ON lifecycle_copy_operation
 FOR EACH ROW EXECUTE FUNCTION lifecycle_versioned_plan_target_guard();

DO $$ DECLARE n text; BEGIN
 FOREACH n IN ARRAY ARRAY['lifecycle_protected_object_intent','lifecycle_protected_object_verification'] LOOP
  EXECUTE format('CREATE TRIGGER %I BEFORE UPDATE OR DELETE ON %I FOR EACH ROW EXECUTE FUNCTION lifecycle_fixture_identity_immutable()',n||'_immutable',n);
  EXECUTE format('CREATE TRIGGER %I BEFORE TRUNCATE ON %I FOR EACH STATEMENT EXECUTE FUNCTION lifecycle_fixture_identity_immutable()',n||'_no_truncate',n);
 END LOOP;
END; $$;
