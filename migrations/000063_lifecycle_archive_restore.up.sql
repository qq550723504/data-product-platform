-- Exact synthetic custody copies and independent restore-checkpoint identities.
CREATE TABLE lifecycle_copy_policy (
 id uuid PRIMARY KEY, workspace_id uuid NOT NULL, dataset_version_id uuid NOT NULL REFERENCES dataset_version(id),
 basis_ref varchar(255) NOT NULL CHECK(basis_ref='CONTROLLED_SYNTHETIC_CUSTODY_AND_USE'),
 use_until timestamptz NOT NULL, restore_until timestamptz NOT NULL,
 actor_id uuid NOT NULL, principal_ref varchar(255) NOT NULL, created_at timestamptz NOT NULL,
 CHECK(use_until>created_at AND restore_until>created_at), UNIQUE(workspace_id,id), UNIQUE(workspace_id,dataset_version_id)
);
CREATE TABLE lifecycle_copy_operation (
 id uuid PRIMARY KEY, workspace_id uuid NOT NULL, source_copy_id uuid NOT NULL,
 target_copy_id uuid NOT NULL UNIQUE, target_namespace_id uuid NOT NULL, target_key varchar(1024) NOT NULL CHECK(target_key<>''),
 action varchar(16) NOT NULL CHECK(action IN('ARCHIVE','RESTORE')), policy_id uuid NOT NULL,
 basis_ref varchar(255) NOT NULL CHECK(basis_ref<>''), actor_id uuid NOT NULL, principal_ref varchar(255) NOT NULL,
 dependency_revision bigint NOT NULL CHECK(dependency_revision>0), created_at timestamptz NOT NULL, expires_at timestamptz NOT NULL CHECK(expires_at>created_at),
 FOREIGN KEY(workspace_id,source_copy_id) REFERENCES lifecycle_fixture_copy(workspace_id,id),
 FOREIGN KEY(workspace_id,target_namespace_id) REFERENCES lifecycle_fixture_namespace(workspace_id,id),
 FOREIGN KEY(workspace_id,policy_id) REFERENCES lifecycle_copy_policy(workspace_id,id),
 UNIQUE(workspace_id,id), UNIQUE(target_namespace_id,target_key)
);
ALTER TABLE lifecycle_storage_call ADD COLUMN copy_operation_id uuid;
ALTER TABLE lifecycle_storage_call ADD FOREIGN KEY(workspace_id,copy_operation_id) REFERENCES lifecycle_copy_operation(workspace_id,id);
ALTER TABLE lifecycle_storage_call DROP CONSTRAINT lifecycle_storage_call_operation_check;
ALTER TABLE lifecycle_storage_call ADD CHECK(
 (operation IN('STAT_EXACT','INSPECT_RETENTION','INSPECT_LEGAL_HOLD','LIST_NAMESPACE_VERSIONS') AND copy_operation_id IS NULL)
 OR(operation IN('ARCHIVE_COPY','RESTORE_COPY','VERIFY_COPY_BYTES') AND copy_operation_id IS NOT NULL)
);
CREATE UNIQUE INDEX lifecycle_copy_single_send ON lifecycle_storage_call(copy_operation_id) WHERE operation IN('ARCHIVE_COPY','RESTORE_COPY');
ALTER TABLE lifecycle_storage_observation ADD COLUMN copy_result_version varchar(1024) CHECK(copy_result_version IS NULL OR (copy_result_version NOT IN('','null') AND copy_result_version=btrim(copy_result_version)));
CREATE TABLE lifecycle_copy_materialization (
 operation_id uuid PRIMARY KEY, workspace_id uuid NOT NULL, copy_id uuid NOT NULL UNIQUE, copy_call_id uuid NOT NULL UNIQUE,
 observed_at timestamptz NOT NULL,
 FOREIGN KEY(workspace_id,operation_id) REFERENCES lifecycle_copy_operation(workspace_id,id),
 FOREIGN KEY(workspace_id,copy_id) REFERENCES lifecycle_fixture_copy(workspace_id,id),
 FOREIGN KEY(workspace_id,copy_call_id) REFERENCES lifecycle_storage_call(workspace_id,id), UNIQUE(workspace_id,operation_id)
);
CREATE TABLE lifecycle_copy_verification (
 operation_id uuid PRIMARY KEY, workspace_id uuid NOT NULL, copy_id uuid NOT NULL, verify_call_id uuid NOT NULL UNIQUE,
 content_sha256 varchar(64) NOT NULL CHECK(content_sha256 ~ '^[0-9a-f]{64}$'), byte_size bigint NOT NULL CHECK(byte_size>0),
 observed_at timestamptz NOT NULL, protection_checked_at timestamptz NOT NULL,
 FOREIGN KEY(workspace_id,operation_id) REFERENCES lifecycle_copy_materialization(workspace_id,operation_id),
 FOREIGN KEY(workspace_id,copy_id) REFERENCES lifecycle_fixture_copy(workspace_id,id),
 FOREIGN KEY(workspace_id,verify_call_id) REFERENCES lifecycle_storage_call(workspace_id,id)
);
CREATE TABLE lifecycle_restore_checkpoint (
 id uuid PRIMARY KEY,workspace_id uuid NOT NULL,dataset_version_id uuid NOT NULL REFERENCES dataset_version(id),epoch uuid NOT NULL,
 sequence bigint NOT NULL CHECK(sequence>0),dependency_revision bigint NOT NULL CHECK(dependency_revision>=0),
 state_sha256 varchar(64) NOT NULL CHECK(state_sha256 ~ '^[0-9a-f]{64}$'),
 previous_sha256 varchar(64) NOT NULL CHECK(previous_sha256 ~ '^[0-9a-f]{64}$'),content_sha256 varchar(64) NOT NULL CHECK(content_sha256 ~ '^[0-9a-f]{64}$'),
 created_at timestamptz NOT NULL,expires_at timestamptz NOT NULL CHECK(expires_at>created_at),
 UNIQUE(workspace_id,id),UNIQUE(workspace_id,dataset_version_id,epoch,sequence)
);
CREATE TABLE lifecycle_copy_decision (
 id uuid PRIMARY KEY,workspace_id uuid NOT NULL,operation_id uuid NOT NULL,checkpoint_id uuid,
 decision varchar(16) NOT NULL CHECK(decision IN('ARCHIVED','ONLINE','BLOCKED','UNKNOWN')),
 reason varchar(64) NOT NULL,actor_id uuid NOT NULL,principal_ref varchar(255) NOT NULL,
 dependency_revision bigint NOT NULL CHECK(dependency_revision>=0),occurred_at timestamptz NOT NULL,
 FOREIGN KEY(workspace_id,operation_id) REFERENCES lifecycle_copy_operation(workspace_id,id),
 FOREIGN KEY(workspace_id,checkpoint_id) REFERENCES lifecycle_restore_checkpoint(workspace_id,id)
);
CREATE UNIQUE INDEX lifecycle_copy_one_completion ON lifecycle_copy_decision(operation_id) WHERE decision IN('ARCHIVED','ONLINE');
CREATE FUNCTION lifecycle_copy_fact_guard() RETURNS trigger AS $$
DECLARE op lifecycle_copy_operation; src lifecycle_fixture_copy; dst lifecycle_fixture_copy; source_ns lifecycle_fixture_namespace; target_ns lifecycle_fixture_namespace; policy lifecycle_copy_policy;
BEGIN
 IF TG_TABLE_NAME IN('lifecycle_copy_policy','lifecycle_restore_checkpoint') THEN
  IF NOT EXISTS(SELECT 1 FROM dataset_version v JOIN dataset d ON d.id=v.dataset_id WHERE v.id=NEW.dataset_version_id AND d.workspace_id=NEW.workspace_id) THEN RAISE EXCEPTION 'copy policy workspace mismatch'; END IF;
  RETURN NEW;
 END IF;
 IF TG_TABLE_NAME='lifecycle_copy_operation' THEN
  SELECT * INTO src FROM lifecycle_fixture_copy WHERE workspace_id=NEW.workspace_id AND id=NEW.source_copy_id;
  SELECT * INTO source_ns FROM lifecycle_fixture_namespace WHERE id=src.namespace_id;
  SELECT * INTO target_ns FROM lifecycle_fixture_namespace WHERE workspace_id=NEW.workspace_id AND id=NEW.target_namespace_id;
  SELECT * INTO policy FROM lifecycle_copy_policy WHERE workspace_id=NEW.workspace_id AND id=NEW.policy_id;
  IF src.id IS NULL OR target_ns.id IS NULL OR policy.dataset_version_id IS DISTINCT FROM src.dataset_version_id OR source_ns.provider_instance_id IS DISTINCT FROM target_ns.provider_instance_id OR source_ns.incarnation_id IS DISTINCT FROM target_ns.incarnation_id OR NEW.expires_at>policy.restore_until THEN RAISE EXCEPTION 'copy intent scope/policy mismatch'; END IF;
  RETURN NEW;
 END IF;
 SELECT * INTO op FROM lifecycle_copy_operation WHERE id=NEW.operation_id AND workspace_id=NEW.workspace_id FOR UPDATE;
 IF NOT FOUND THEN RAISE EXCEPTION 'missing copy intent'; END IF;
 SELECT * INTO src FROM lifecycle_fixture_copy WHERE id=op.source_copy_id;
 IF TG_TABLE_NAME IN('lifecycle_copy_materialization','lifecycle_copy_verification') THEN
  SELECT * INTO dst FROM lifecycle_fixture_copy WHERE id=NEW.copy_id AND workspace_id=NEW.workspace_id;
  IF NOT FOUND OR dst.id<>op.target_copy_id OR dst.namespace_id<>op.target_namespace_id OR dst.object_key<>op.target_key OR dst.dataset_version_id<>src.dataset_version_id OR dst.content_sha256<>src.content_sha256 OR dst.byte_size<>src.byte_size THEN RAISE EXCEPTION 'copy content/target mismatch'; END IF;
 END IF;
 IF TG_TABLE_NAME='lifecycle_copy_materialization' THEN
  IF NOT EXISTS(SELECT 1 FROM lifecycle_storage_call c JOIN lifecycle_storage_observation x ON x.call_id=c.id WHERE c.id=NEW.copy_call_id AND c.copy_operation_id=op.id AND c.copy_id=src.id AND c.operation=op.action||'_COPY' AND x.http_status=200 AND x.failure='' AND x.invoked_at IS NOT NULL AND x.copy_result_version=dst.provider_version AND x.response_sha256 IS NOT NULL AND x.observed_at<=NEW.observed_at) THEN RAISE EXCEPTION 'unbacked copy materialization'; END IF;
 ELSIF TG_TABLE_NAME='lifecycle_copy_verification' THEN
  IF NEW.content_sha256<>dst.content_sha256 OR NEW.byte_size<>dst.byte_size OR NEW.protection_checked_at>NEW.observed_at OR NOT EXISTS(SELECT 1 FROM lifecycle_storage_call c JOIN lifecycle_storage_observation x ON x.call_id=c.id WHERE c.id=NEW.verify_call_id AND c.copy_operation_id=op.id AND c.copy_id=dst.id AND c.operation='VERIFY_COPY_BYTES' AND x.http_status=200 AND x.failure='' AND x.response_sha256=NEW.content_sha256 AND x.observed_at<=NEW.observed_at) THEN RAISE EXCEPTION 'unbacked exact bytes verification'; END IF;
 ELSIF TG_TABLE_NAME='lifecycle_copy_decision' AND NEW.decision IN('ARCHIVED','ONLINE') THEN
  IF NOT EXISTS(SELECT 1 FROM lifecycle_copy_verification WHERE operation_id=op.id) OR (NEW.decision='ARCHIVED')<>(op.action='ARCHIVE') OR (NEW.decision='ONLINE' AND NEW.checkpoint_id IS NULL) THEN RAISE EXCEPTION 'incomplete copy cannot open'; END IF;
 END IF;
 RETURN NEW;
END;
$$ LANGUAGE plpgsql;
CREATE TRIGGER lifecycle_copy_policy_scope BEFORE INSERT ON lifecycle_copy_policy FOR EACH ROW EXECUTE FUNCTION lifecycle_copy_fact_guard();
CREATE TRIGGER lifecycle_copy_operation_scope BEFORE INSERT ON lifecycle_copy_operation FOR EACH ROW EXECUTE FUNCTION lifecycle_copy_fact_guard();
CREATE TRIGGER lifecycle_restore_checkpoint_scope BEFORE INSERT ON lifecycle_restore_checkpoint FOR EACH ROW EXECUTE FUNCTION lifecycle_copy_fact_guard();
CREATE TRIGGER lifecycle_copy_materialization_guard BEFORE INSERT ON lifecycle_copy_materialization FOR EACH ROW EXECUTE FUNCTION lifecycle_copy_fact_guard();
CREATE TRIGGER lifecycle_copy_verification_guard BEFORE INSERT ON lifecycle_copy_verification FOR EACH ROW EXECUTE FUNCTION lifecycle_copy_fact_guard();
CREATE TRIGGER lifecycle_copy_decision_guard BEFORE INSERT ON lifecycle_copy_decision FOR EACH ROW EXECUTE FUNCTION lifecycle_copy_fact_guard();
DO $$ DECLARE n text; BEGIN
 FOREACH n IN ARRAY ARRAY['lifecycle_copy_policy','lifecycle_copy_operation','lifecycle_copy_materialization','lifecycle_copy_verification','lifecycle_restore_checkpoint','lifecycle_copy_decision'] LOOP
  EXECUTE format('CREATE TRIGGER %I BEFORE UPDATE OR DELETE ON %I FOR EACH ROW EXECUTE FUNCTION lifecycle_fixture_identity_immutable()',n||'_immutable',n);
  EXECUTE format('CREATE TRIGGER %I BEFORE TRUNCATE ON %I FOR EACH STATEMENT EXECUTE FUNCTION lifecycle_fixture_identity_immutable()',n||'_no_truncate',n);
 END LOOP;
END $$;
