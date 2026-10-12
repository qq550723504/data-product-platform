-- Durable recovery requests reuse the platform outbox and Asynq runtime.
-- A completed recovery request never implies that an UNKNOWN delete stopped.
-- The same obligation also recovers COPY identity through read-only discovery;
-- it never fabricates an ACK on the original physical call or permits a resend.
ALTER TABLE lifecycle_reader_pin DROP CONSTRAINT lifecycle_reader_pin_kind_check;
ALTER TABLE lifecycle_reader_pin ADD COLUMN execution_id uuid REFERENCES execution(id);
ALTER TABLE lifecycle_reader_pin ADD COLUMN delivery_operation_id uuid REFERENCES delivery_operation(id);
ALTER TABLE lifecycle_reader_pin ADD CHECK(
 (kind='FIXTURE_READER' AND execution_id IS NULL AND delivery_operation_id IS NULL)
 OR(kind='FIXTURE_PROCESSING' AND execution_id IS NOT NULL AND execution_id=reader_id AND delivery_operation_id IS NULL)
 OR(kind='FIXTURE_DELIVERY' AND delivery_operation_id IS NOT NULL AND delivery_operation_id=reader_id AND execution_id IS NULL));
CREATE UNIQUE INDEX lifecycle_processing_one_pin ON lifecycle_reader_pin(execution_id,copy_id) WHERE execution_id IS NOT NULL;
CREATE UNIQUE INDEX lifecycle_delivery_one_pin ON lifecycle_reader_pin(delivery_operation_id) WHERE delivery_operation_id IS NOT NULL;
CREATE FUNCTION lifecycle_reader_subject_guard() RETURNS trigger AS $$
DECLARE dv uuid; e execution; d delivery_operation;
BEGIN
 SELECT dataset_version_id INTO STRICT dv FROM lifecycle_fixture_copy WHERE workspace_id=NEW.workspace_id AND id=NEW.copy_id;
 IF NEW.execution_id IS NOT NULL THEN
  SELECT * INTO STRICT e FROM execution WHERE id=NEW.execution_id FOR KEY SHARE;
  IF e.workspace_id<>NEW.workspace_id OR e.status<>'RUNNING' OR e.created_by IS DISTINCT FROM NEW.actor_id OR NOT EXISTS(SELECT 1 FROM execution_input WHERE execution_id=e.id AND dataset_version_id=dv) THEN RAISE EXCEPTION 'processing reader has no current exact execution input'; END IF;
 ELSIF NEW.delivery_operation_id IS NOT NULL THEN
  SELECT * INTO STRICT d FROM delivery_operation WHERE id=NEW.delivery_operation_id FOR KEY SHARE;
  IF d.workspace_id<>NEW.workspace_id OR d.status<>'ISSUED' OR d.dataset_version_id<>dv OR d.principal_ref<>NEW.principal_ref OR d.provider_name<>'PLATFORM_DIRECT_DATA' OR d.delivery_mode<>'DIRECT_DATA' OR d.fresh_cap_expires_at IS NULL OR NEW.use_until IS NULL OR NEW.use_until>d.fresh_cap_expires_at THEN RAISE EXCEPTION 'transfer pin must belong to fenced issued direct delivery'; END IF;
 END IF;
 RETURN NEW;
END $$ LANGUAGE plpgsql;
CREATE TRIGGER lifecycle_reader_subject_scope BEFORE INSERT ON lifecycle_reader_pin FOR EACH ROW EXECUTE FUNCTION lifecycle_reader_subject_guard();
CREATE TABLE lifecycle_transfer_observation (
 id uuid PRIMARY KEY,workspace_id uuid NOT NULL,pin_id uuid NOT NULL UNIQUE,delivery_operation_id uuid NOT NULL REFERENCES delivery_operation(id),termination_id uuid NOT NULL UNIQUE REFERENCES lifecycle_reader_termination(id),
 bytes_written bigint NOT NULL CHECK(bytes_written>=0),completed boolean NOT NULL,observed_at timestamptz NOT NULL,
 FOREIGN KEY(workspace_id,pin_id) REFERENCES lifecycle_reader_pin(workspace_id,id));
CREATE FUNCTION lifecycle_transfer_observation_guard() RETURNS trigger AS $$
DECLARE p lifecycle_reader_pin;t lifecycle_reader_termination;size bigint;
BEGIN
 SELECT * INTO STRICT p FROM lifecycle_reader_pin WHERE id=NEW.pin_id FOR UPDATE;
 SELECT * INTO STRICT t FROM lifecycle_reader_termination WHERE id=NEW.termination_id;
 SELECT byte_size INTO STRICT size FROM lifecycle_fixture_copy WHERE id=p.copy_id;
 IF p.kind<>'FIXTURE_DELIVERY' OR p.workspace_id<>NEW.workspace_id OR p.delivery_operation_id<>NEW.delivery_operation_id OR t.pin_id<>p.id OR NEW.observed_at<>t.observed_at OR NEW.bytes_written>size OR NEW.completed<>(t.outcome='COMPLETED') OR (NEW.completed AND NEW.bytes_written<>size) THEN RAISE EXCEPTION 'unbacked transfer stop observation'; END IF;
 RETURN NEW;
END $$ LANGUAGE plpgsql;
CREATE TRIGGER lifecycle_transfer_observation_scope BEFORE INSERT ON lifecycle_transfer_observation FOR EACH ROW EXECUTE FUNCTION lifecycle_transfer_observation_guard();
CREATE TRIGGER lifecycle_transfer_observation_immutable BEFORE UPDATE OR DELETE ON lifecycle_transfer_observation FOR EACH ROW EXECUTE FUNCTION lifecycle_fixture_identity_immutable();
CREATE TRIGGER lifecycle_transfer_observation_no_truncate BEFORE TRUNCATE ON lifecycle_transfer_observation FOR EACH STATEMENT EXECUTE FUNCTION lifecycle_fixture_identity_immutable();
ALTER TABLE lifecycle_storage_call ADD COLUMN reader_pin_id uuid;
CREATE TABLE lifecycle_processing_output_intent (
 id uuid PRIMARY KEY,workspace_id uuid NOT NULL,source_copy_id uuid NOT NULL,dataset_version_id uuid NOT NULL UNIQUE REFERENCES dataset_version(id),namespace_id uuid NOT NULL,object_key varchar(1024) NOT NULL,content_sha256 char(64) NOT NULL CHECK(content_sha256~'^[a-f0-9]{64}$'),byte_size bigint NOT NULL CHECK(byte_size>0 AND byte_size<=131072),
 execution_id uuid REFERENCES execution(id),entity_match_job_id uuid REFERENCES entity_match_job(id),actor_id uuid NOT NULL,principal_ref varchar(255) NOT NULL,created_at timestamptz NOT NULL,
 CHECK((execution_id IS NULL)<>(entity_match_job_id IS NULL)),UNIQUE(workspace_id,id),UNIQUE(namespace_id,object_key),
 FOREIGN KEY(workspace_id,source_copy_id) REFERENCES lifecycle_fixture_copy(workspace_id,id),FOREIGN KEY(workspace_id,namespace_id) REFERENCES lifecycle_fixture_namespace(workspace_id,id));
CREATE FUNCTION lifecycle_processing_output_scope_guard() RETURNS trigger AS $$
DECLARE v dataset_version;w uuid;source lifecycle_fixture_copy;n lifecycle_fixture_namespace;sn lifecycle_fixture_namespace;
BEGIN
 SELECT * INTO STRICT v FROM dataset_version WHERE id=NEW.dataset_version_id FOR KEY SHARE;
 SELECT workspace_id INTO STRICT w FROM dataset WHERE id=v.dataset_id;
 SELECT * INTO STRICT source FROM lifecycle_fixture_copy WHERE id=NEW.source_copy_id;
 SELECT * INTO STRICT sn FROM lifecycle_fixture_namespace WHERE id=source.namespace_id;
 SELECT * INTO STRICT n FROM lifecycle_fixture_namespace WHERE id=NEW.namespace_id;
 IF w<>NEW.workspace_id OR source.workspace_id<>w OR n.workspace_id<>w OR n.provider_instance_id<>sn.provider_instance_id OR n.incarnation_id<>sn.incarnation_id OR v.generated_by_execution_id IS DISTINCT FROM NEW.execution_id OR v.generated_by_entity_match_job_id IS DISTINCT FROM NEW.entity_match_job_id OR v.status NOT IN('CREATED','PROCESSING') THEN RAISE EXCEPTION 'processing output does not own allocated same-workspace target'; END IF;
 RETURN NEW;
END $$ LANGUAGE plpgsql;
CREATE TRIGGER lifecycle_processing_output_scope BEFORE INSERT ON lifecycle_processing_output_intent FOR EACH ROW EXECUTE FUNCTION lifecycle_processing_output_scope_guard();
CREATE TRIGGER lifecycle_processing_output_immutable BEFORE UPDATE OR DELETE ON lifecycle_processing_output_intent FOR EACH ROW EXECUTE FUNCTION lifecycle_fixture_identity_immutable();
CREATE TRIGGER lifecycle_processing_output_no_truncate BEFORE TRUNCATE ON lifecycle_processing_output_intent FOR EACH STATEMENT EXECUTE FUNCTION lifecycle_fixture_identity_immutable();
ALTER TABLE lifecycle_storage_call ADD COLUMN processing_output_intent_id uuid;
ALTER TABLE lifecycle_storage_call ADD FOREIGN KEY(workspace_id,processing_output_intent_id) REFERENCES lifecycle_processing_output_intent(workspace_id,id);
ALTER TABLE lifecycle_storage_call ADD FOREIGN KEY(workspace_id,reader_pin_id) REFERENCES lifecycle_reader_pin(workspace_id,id);
ALTER TABLE lifecycle_storage_call DROP CONSTRAINT lifecycle_storage_call_check;
ALTER TABLE lifecycle_storage_call ADD CHECK(
 (operation IN('STAT_EXACT','INSPECT_RETENTION','INSPECT_LEGAL_HOLD','LIST_NAMESPACE_VERSIONS') AND copy_operation_id IS NULL AND reader_pin_id IS NULL AND processing_output_intent_id IS NULL)
 OR(operation IN('ARCHIVE_COPY','RESTORE_COPY','VERIFY_COPY_BYTES','RECOVER_COPY_VERSIONS','RECOVER_COPY_EXACT') AND copy_operation_id IS NOT NULL AND reader_pin_id IS NULL AND processing_output_intent_id IS NULL)
 OR(operation='READ_PINNED' AND copy_operation_id IS NULL AND reader_pin_id IS NOT NULL AND processing_output_intent_id IS NULL)
 OR(operation='PROCESSING_PUT' AND copy_operation_id IS NULL AND reader_pin_id IS NULL AND processing_output_intent_id IS NOT NULL)
);
CREATE UNIQUE INDEX lifecycle_processing_output_single_call ON lifecycle_storage_call(processing_output_intent_id) WHERE processing_output_intent_id IS NOT NULL;
CREATE TABLE lifecycle_processing_output_ready (
 intent_id uuid PRIMARY KEY REFERENCES lifecycle_processing_output_intent(id),workspace_id uuid NOT NULL,copy_id uuid NOT NULL UNIQUE,call_id uuid NOT NULL UNIQUE REFERENCES lifecycle_storage_call(id),observed_at timestamptz NOT NULL,
 FOREIGN KEY(workspace_id,copy_id) REFERENCES lifecycle_fixture_copy(workspace_id,id));
CREATE FUNCTION lifecycle_processing_ready_guard() RETURNS trigger AS $$
DECLARE i lifecycle_processing_output_intent;c lifecycle_fixture_copy;
BEGIN
 SELECT * INTO STRICT i FROM lifecycle_processing_output_intent WHERE id=NEW.intent_id FOR UPDATE;
 SELECT * INTO STRICT c FROM lifecycle_fixture_copy WHERE id=NEW.copy_id;
 IF i.workspace_id<>NEW.workspace_id OR c.workspace_id<>i.workspace_id OR c.dataset_version_id<>i.dataset_version_id OR c.namespace_id<>i.namespace_id OR c.object_key<>i.object_key OR c.content_sha256<>i.content_sha256 OR c.byte_size<>i.byte_size OR NOT EXISTS(SELECT 1 FROM lifecycle_storage_call s JOIN lifecycle_storage_observation x ON x.call_id=s.id WHERE s.id=NEW.call_id AND s.processing_output_intent_id=i.id AND s.operation='PROCESSING_PUT' AND x.http_status=200 AND x.failure='' AND x.invoked_at IS NOT NULL AND x.copy_result_version=c.provider_version AND x.observed_at<=NEW.observed_at) THEN RAISE EXCEPTION 'unbacked processing output ready'; END IF;
 RETURN NEW;
END $$ LANGUAGE plpgsql;
CREATE TRIGGER lifecycle_processing_ready_scope BEFORE INSERT ON lifecycle_processing_output_ready FOR EACH ROW EXECUTE FUNCTION lifecycle_processing_ready_guard();
CREATE TRIGGER lifecycle_processing_ready_immutable BEFORE UPDATE OR DELETE ON lifecycle_processing_output_ready FOR EACH ROW EXECUTE FUNCTION lifecycle_fixture_identity_immutable();
CREATE TRIGGER lifecycle_processing_ready_no_truncate BEFORE TRUNCATE ON lifecycle_processing_output_ready FOR EACH STATEMENT EXECUTE FUNCTION lifecycle_fixture_identity_immutable();
CREATE FUNCTION lifecycle_pinned_read_guard() RETURNS trigger AS $$
DECLARE pin lifecycle_reader_pin;
BEGIN
 IF NEW.reader_pin_id IS NULL THEN RETURN NEW; END IF;
 SELECT * INTO STRICT pin FROM lifecycle_reader_pin WHERE workspace_id=NEW.workspace_id AND id=NEW.reader_pin_id FOR UPDATE;
 IF NEW.copy_id<>pin.copy_id OR NEW.actor_id<>pin.actor_id OR NEW.principal_ref<>pin.principal_ref OR NEW.started_at<pin.started_at OR (pin.use_until IS NOT NULL AND NEW.started_at>=pin.use_until) OR EXISTS(SELECT 1 FROM lifecycle_reader_termination WHERE pin_id=pin.id) THEN RAISE EXCEPTION 'pinned read owner/generation is not active'; END IF;
 RETURN NEW;
END $$ LANGUAGE plpgsql;
CREATE TRIGGER lifecycle_pinned_read_scope BEFORE INSERT ON lifecycle_storage_call FOR EACH ROW EXECUTE FUNCTION lifecycle_pinned_read_guard();
CREATE UNIQUE INDEX lifecycle_reader_single_native_read ON lifecycle_storage_call(reader_pin_id) WHERE reader_pin_id IS NOT NULL;
ALTER TABLE lifecycle_storage_observation ADD COLUMN copy_operation_tag uuid;
ALTER TABLE lifecycle_storage_observation ADD COLUMN copy_result_bytes bigint CHECK(copy_result_bytes>=0);
CREATE TABLE lifecycle_copy_discovery (
 id uuid PRIMARY KEY,workspace_id uuid NOT NULL,operation_id uuid NOT NULL,original_call_id uuid NOT NULL,
 inventory_call_id uuid NOT NULL,stat_call_id uuid NOT NULL,provider_version varchar(1024) NOT NULL CHECK(provider_version NOT IN('','null')),
 observed_at timestamptz NOT NULL,
 FOREIGN KEY(workspace_id,operation_id) REFERENCES lifecycle_copy_operation(workspace_id,id),
 FOREIGN KEY(workspace_id,original_call_id) REFERENCES lifecycle_storage_call(workspace_id,id),
 FOREIGN KEY(workspace_id,inventory_call_id) REFERENCES lifecycle_storage_call(workspace_id,id),
 FOREIGN KEY(workspace_id,stat_call_id) REFERENCES lifecycle_storage_call(workspace_id,id),
 UNIQUE(workspace_id,id),UNIQUE(operation_id)
);
ALTER TABLE lifecycle_copy_materialization ADD COLUMN discovery_id uuid;
ALTER TABLE lifecycle_copy_materialization ADD FOREIGN KEY(workspace_id,discovery_id) REFERENCES lifecycle_copy_discovery(workspace_id,id);
ALTER TABLE lifecycle_copy_decision ADD UNIQUE(workspace_id,id);
CREATE FUNCTION lifecycle_copy_discovery_guard() RETURNS trigger AS $$
DECLARE op lifecycle_copy_operation; original lifecycle_storage_call; src lifecycle_fixture_copy;
BEGIN
 SELECT * INTO STRICT op FROM lifecycle_copy_operation WHERE id=NEW.operation_id AND workspace_id=NEW.workspace_id FOR UPDATE;
 SELECT * INTO STRICT src FROM lifecycle_fixture_copy WHERE id=op.source_copy_id;
 SELECT * INTO STRICT original FROM lifecycle_storage_call WHERE id=NEW.original_call_id AND workspace_id=NEW.workspace_id AND copy_operation_id=op.id AND copy_id=src.id AND operation=op.action||'_COPY';
 IF NOT EXISTS(SELECT 1 FROM lifecycle_storage_call c JOIN lifecycle_storage_observation x ON x.call_id=c.id WHERE c.id=NEW.inventory_call_id AND c.workspace_id=NEW.workspace_id AND c.copy_operation_id=op.id AND c.copy_id=src.id AND c.operation='RECOVER_COPY_VERSIONS' AND c.started_at>=original.started_at AND x.http_status=200 AND x.failure='' AND x.invoked_at IS NOT NULL AND x.response_sha256 IS NOT NULL AND x.observed_at<=NEW.observed_at AND NEW.observed_at-x.observed_at<=interval '1 minute') THEN RAISE EXCEPTION 'unbacked copy recovery inventory'; END IF;
 IF NOT EXISTS(SELECT 1 FROM lifecycle_storage_call c JOIN lifecycle_storage_observation x ON x.call_id=c.id WHERE c.id=NEW.stat_call_id AND c.workspace_id=NEW.workspace_id AND c.copy_operation_id=op.id AND c.copy_id=src.id AND c.operation='RECOVER_COPY_EXACT' AND c.started_at>=original.started_at AND x.http_status=200 AND x.failure='' AND x.invoked_at IS NOT NULL AND x.response_sha256 IS NOT NULL AND x.copy_result_version=NEW.provider_version AND x.copy_operation_tag=op.id AND x.copy_result_bytes=src.byte_size AND x.observed_at<=NEW.observed_at AND NEW.observed_at-x.observed_at<=interval '1 minute') THEN RAISE EXCEPTION 'unbacked tagged exact copy recovery'; END IF;
 RETURN NEW;
END $$ LANGUAGE plpgsql;
CREATE TRIGGER lifecycle_copy_discovery_scope BEFORE INSERT ON lifecycle_copy_discovery FOR EACH ROW EXECUTE FUNCTION lifecycle_copy_discovery_guard();
CREATE TRIGGER lifecycle_copy_discovery_immutable BEFORE UPDATE OR DELETE ON lifecycle_copy_discovery FOR EACH ROW EXECUTE FUNCTION lifecycle_fixture_identity_immutable();
CREATE TRIGGER lifecycle_copy_discovery_no_truncate BEFORE TRUNCATE ON lifecycle_copy_discovery FOR EACH STATEMENT EXECUTE FUNCTION lifecycle_fixture_identity_immutable();
CREATE TABLE lifecycle_reconciliation_work (
 id uuid PRIMARY KEY, workspace_id uuid NOT NULL, attempt_id uuid, copy_call_id uuid,
 basis_ref varchar(255) NOT NULL CHECK(length(basis_ref)>0),
 requested_at timestamptz NOT NULL, actor_id uuid NOT NULL, principal_ref varchar(255) NOT NULL,
 dependency_revision bigint NOT NULL CHECK(dependency_revision>=0),
 FOREIGN KEY(workspace_id,attempt_id) REFERENCES lifecycle_provider_attempt(workspace_id,id),
 FOREIGN KEY(workspace_id,copy_call_id) REFERENCES lifecycle_storage_call(workspace_id,id),
 CHECK((attempt_id IS NULL)<>(copy_call_id IS NULL)),
 UNIQUE(workspace_id,id)
);
CREATE INDEX lifecycle_reconciliation_work_attempt ON lifecycle_reconciliation_work(workspace_id,attempt_id);
CREATE INDEX lifecycle_reconciliation_work_copy ON lifecycle_reconciliation_work(workspace_id,copy_call_id);
CREATE TABLE lifecycle_reconciliation_run (
 id uuid PRIMARY KEY, workspace_id uuid NOT NULL, work_id uuid NOT NULL,
 started_at timestamptz NOT NULL, actor_id uuid NOT NULL, principal_ref varchar(255) NOT NULL,
 dependency_revision bigint NOT NULL CHECK(dependency_revision>=0),
 FOREIGN KEY(workspace_id,work_id) REFERENCES lifecycle_reconciliation_work(workspace_id,id),
 UNIQUE(workspace_id,id,work_id)
);
CREATE TABLE lifecycle_reconciliation_result (
 id uuid PRIMARY KEY, workspace_id uuid NOT NULL, work_id uuid NOT NULL, run_id uuid NOT NULL,
 outcome varchar(32) NOT NULL CHECK(outcome IN('UNKNOWN_PRESENT','UNKNOWN_PROVIDER','BLOCKED_AUTHORITY','ABSENT_PARTIAL','ABSENT_VERIFIED','ALREADY_RESOLVED','UNKNOWN_COPY_TARGET','UNKNOWN_COPY_LEDGER','COPY_ARCHIVED','RESTORE_ONLINE','BLOCKED_COPY')),
 terminal boolean NOT NULL,
 inventory_call_id uuid, stat_call_id uuid,
 absence_receipt_id uuid REFERENCES lifecycle_copy_absence_receipt(id),
 completion_id uuid REFERENCES lifecycle_plan_completion(id),
 copy_discovery_id uuid,copy_decision_id uuid,
 observed_at timestamptz NOT NULL, dependency_revision bigint NOT NULL CHECK(dependency_revision>=0),
 FOREIGN KEY(workspace_id,run_id,work_id) REFERENCES lifecycle_reconciliation_run(workspace_id,id,work_id),
 FOREIGN KEY(workspace_id,inventory_call_id) REFERENCES lifecycle_storage_call(workspace_id,id),
 FOREIGN KEY(workspace_id,stat_call_id) REFERENCES lifecycle_storage_call(workspace_id,id),
 FOREIGN KEY(workspace_id,copy_discovery_id) REFERENCES lifecycle_copy_discovery(workspace_id,id),
 FOREIGN KEY(workspace_id,copy_decision_id) REFERENCES lifecycle_copy_decision(workspace_id,id),
 CHECK(outcome NOT IN('UNKNOWN_COPY_LEDGER','COPY_ARCHIVED','RESTORE_ONLINE','BLOCKED_COPY') OR copy_decision_id IS NOT NULL),
 CHECK(terminal=(outcome<>'UNKNOWN_PROVIDER')),
 CHECK(outcome<>'UNKNOWN_PRESENT' OR (inventory_call_id IS NOT NULL AND stat_call_id IS NOT NULL)),
 CHECK(outcome NOT IN('ABSENT_PARTIAL','ABSENT_VERIFIED') OR absence_receipt_id IS NOT NULL),
 CHECK(outcome<>'ABSENT_VERIFIED' OR completion_id IS NOT NULL)
);
CREATE UNIQUE INDEX lifecycle_reconciliation_one_terminal ON lifecycle_reconciliation_result(work_id) WHERE terminal;
CREATE FUNCTION lifecycle_reconciliation_scope_guard() RETURNS trigger AS $$
DECLARE a lifecycle_provider_attempt; work lifecycle_reconciliation_work; call lifecycle_storage_call;
BEGIN
 IF TG_TABLE_NAME='lifecycle_reconciliation_work' THEN
  IF NEW.copy_call_id IS NOT NULL AND NOT EXISTS(SELECT 1 FROM lifecycle_storage_call c JOIN lifecycle_copy_operation op ON op.id=c.copy_operation_id WHERE c.id=NEW.copy_call_id AND c.workspace_id=NEW.workspace_id AND c.copy_id=op.source_copy_id AND c.operation=op.action||'_COPY' AND c.started_at<=NEW.requested_at) THEN RAISE EXCEPTION 'copy recovery requires original physical custody call'; END IF;
 ELSIF TG_TABLE_NAME='lifecycle_reconciliation_run' THEN
  PERFORM id FROM lifecycle_reconciliation_work WHERE id=NEW.work_id AND workspace_id=NEW.workspace_id AND requested_at<=NEW.started_at FOR UPDATE;
  IF NOT FOUND THEN RAISE EXCEPTION 'recovery run requires scoped durable request'; END IF;
 ELSE
  SELECT * INTO STRICT work FROM lifecycle_reconciliation_work WHERE id=NEW.work_id AND workspace_id=NEW.workspace_id;
  IF work.copy_call_id IS NOT NULL THEN
   SELECT * INTO STRICT call FROM lifecycle_storage_call WHERE id=work.copy_call_id AND workspace_id=NEW.workspace_id AND operation IN('ARCHIVE_COPY','RESTORE_COPY');
   IF NEW.outcome IN('UNKNOWN_PRESENT','ABSENT_PARTIAL','ABSENT_VERIFIED') OR NEW.absence_receipt_id IS NOT NULL OR NEW.completion_id IS NOT NULL THEN RAISE EXCEPTION 'copy recovery cannot imply delete completion'; END IF;
   IF NEW.inventory_call_id IS NOT NULL AND NOT EXISTS(SELECT 1 FROM lifecycle_storage_call c JOIN lifecycle_storage_observation x ON x.call_id=c.id WHERE c.id=NEW.inventory_call_id AND c.copy_operation_id=call.copy_operation_id AND c.copy_id=call.copy_id AND c.operation='RECOVER_COPY_VERSIONS' AND x.http_status=200 AND x.failure='' AND x.invoked_at IS NOT NULL AND x.observed_at<=NEW.observed_at) THEN RAISE EXCEPTION 'unbacked copy recovery inventory'; END IF;
   IF NEW.stat_call_id IS NOT NULL AND NOT EXISTS(SELECT 1 FROM lifecycle_storage_call c JOIN lifecycle_storage_observation x ON x.call_id=c.id WHERE c.id=NEW.stat_call_id AND c.copy_operation_id=call.copy_operation_id AND c.copy_id=call.copy_id AND c.operation='RECOVER_COPY_EXACT' AND x.http_status=200 AND x.failure='' AND x.invoked_at IS NOT NULL AND x.observed_at<=NEW.observed_at) THEN RAISE EXCEPTION 'unbacked copy recovery presence'; END IF;
   IF NEW.copy_discovery_id IS NOT NULL AND NOT EXISTS(SELECT 1 FROM lifecycle_copy_discovery WHERE id=NEW.copy_discovery_id AND operation_id=call.copy_operation_id AND original_call_id=call.id) THEN RAISE EXCEPTION 'copy recovery discovery scope mismatch'; END IF;
   IF NEW.copy_decision_id IS NOT NULL AND NOT EXISTS(SELECT 1 FROM lifecycle_copy_decision WHERE id=NEW.copy_decision_id AND operation_id=call.copy_operation_id AND occurred_at<=NEW.observed_at AND (NEW.outcome NOT IN('COPY_ARCHIVED','RESTORE_ONLINE','BLOCKED_COPY','UNKNOWN_COPY_LEDGER') OR decision=CASE NEW.outcome WHEN 'COPY_ARCHIVED' THEN 'ARCHIVED' WHEN 'RESTORE_ONLINE' THEN 'ONLINE' WHEN 'BLOCKED_COPY' THEN 'BLOCKED' ELSE 'UNKNOWN' END)) THEN RAISE EXCEPTION 'copy recovery decision scope mismatch'; END IF;
   IF NOT EXISTS(SELECT 1 FROM lifecycle_reconciliation_run WHERE id=NEW.run_id AND started_at<=NEW.observed_at) THEN RAISE EXCEPTION 'recovery observation precedes run'; END IF;
   RETURN NEW;
  END IF;
  IF NEW.copy_discovery_id IS NOT NULL OR NEW.copy_decision_id IS NOT NULL OR NEW.outcome IN('UNKNOWN_COPY_TARGET','UNKNOWN_COPY_LEDGER','COPY_ARCHIVED','RESTORE_ONLINE','BLOCKED_COPY') THEN RAISE EXCEPTION 'delete recovery cannot imply copy admission'; END IF;
  SELECT p.* INTO STRICT a FROM lifecycle_reconciliation_work w JOIN lifecycle_provider_attempt p ON p.id=w.attempt_id WHERE w.id=NEW.work_id AND w.workspace_id=NEW.workspace_id;
  IF NEW.inventory_call_id IS NOT NULL AND NOT EXISTS(SELECT 1 FROM lifecycle_storage_call c JOIN lifecycle_storage_observation o ON o.call_id=c.id WHERE c.id=NEW.inventory_call_id AND c.workspace_id=NEW.workspace_id AND c.copy_id=a.copy_id AND c.operation='LIST_NAMESPACE_VERSIONS' AND o.http_status=200 AND o.failure='' AND o.invoked_at IS NOT NULL AND o.observed_at<=NEW.observed_at) THEN RAISE EXCEPTION 'unbacked recovery inventory'; END IF;
  IF NEW.stat_call_id IS NOT NULL AND NOT EXISTS(SELECT 1 FROM lifecycle_storage_call c JOIN lifecycle_storage_observation o ON o.call_id=c.id WHERE c.id=NEW.stat_call_id AND c.workspace_id=NEW.workspace_id AND c.copy_id=a.copy_id AND c.operation='STAT_EXACT' AND o.http_status=200 AND o.failure='' AND o.invoked_at IS NOT NULL AND o.observed_at<=NEW.observed_at) THEN RAISE EXCEPTION 'unbacked recovery presence'; END IF;
  IF NEW.absence_receipt_id IS NOT NULL AND NOT EXISTS(SELECT 1 FROM lifecycle_copy_absence_receipt WHERE id=NEW.absence_receipt_id AND workspace_id=NEW.workspace_id AND dispatch_id=a.dispatch_id) THEN RAISE EXCEPTION 'recovery receipt scope mismatch'; END IF;
  IF NEW.completion_id IS NOT NULL AND NOT EXISTS(SELECT 1 FROM lifecycle_plan_completion WHERE id=NEW.completion_id AND workspace_id=NEW.workspace_id AND plan_id=a.plan_id) THEN RAISE EXCEPTION 'recovery completion scope mismatch'; END IF;
  IF NOT EXISTS(SELECT 1 FROM lifecycle_reconciliation_run WHERE id=NEW.run_id AND started_at<=NEW.observed_at) THEN RAISE EXCEPTION 'recovery observation precedes run'; END IF;
 END IF;
 RETURN NEW;
END $$ LANGUAGE plpgsql;
CREATE TRIGGER lifecycle_reconciliation_run_scope BEFORE INSERT ON lifecycle_reconciliation_run FOR EACH ROW EXECUTE FUNCTION lifecycle_reconciliation_scope_guard();
CREATE TRIGGER lifecycle_reconciliation_work_scope BEFORE INSERT ON lifecycle_reconciliation_work FOR EACH ROW EXECUTE FUNCTION lifecycle_reconciliation_scope_guard();
CREATE TRIGGER lifecycle_reconciliation_result_scope BEFORE INSERT ON lifecycle_reconciliation_result FOR EACH ROW EXECUTE FUNCTION lifecycle_reconciliation_scope_guard();
DO $$ DECLARE n text; BEGIN
 FOREACH n IN ARRAY ARRAY['lifecycle_reconciliation_work','lifecycle_reconciliation_run','lifecycle_reconciliation_result'] LOOP
  EXECUTE format('CREATE TRIGGER %I BEFORE UPDATE OR DELETE ON %I FOR EACH ROW EXECUTE FUNCTION lifecycle_fixture_identity_immutable()',n||'_immutable',n);
  EXECUTE format('CREATE TRIGGER %I BEFORE TRUNCATE ON %I FOR EACH STATEMENT EXECUTE FUNCTION lifecycle_fixture_identity_immutable()',n||'_no_truncate',n);
 END LOOP;
END $$;

-- Preserve the strict ACK path; recovery adds a separate immutable discovery proof.
CREATE OR REPLACE FUNCTION lifecycle_copy_fact_guard() RETURNS trigger AS $$
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
  IF NEW.discovery_id IS NOT NULL AND NOT EXISTS(SELECT 1 FROM lifecycle_copy_discovery d WHERE d.id=NEW.discovery_id AND d.workspace_id=NEW.workspace_id AND d.operation_id=op.id AND d.original_call_id=NEW.copy_call_id AND d.provider_version=dst.provider_version AND d.observed_at<=NEW.observed_at) THEN RAISE EXCEPTION 'copy materialization discovery scope mismatch'; END IF;
  IF NOT EXISTS(SELECT 1 FROM lifecycle_storage_call c JOIN lifecycle_storage_observation x ON x.call_id=c.id WHERE c.id=NEW.copy_call_id AND c.copy_operation_id=op.id AND c.copy_id=src.id AND c.operation=op.action||'_COPY' AND x.http_status=200 AND x.failure='' AND x.invoked_at IS NOT NULL AND x.copy_result_version=dst.provider_version AND x.response_sha256 IS NOT NULL AND x.observed_at<=NEW.observed_at) AND NOT EXISTS(SELECT 1 FROM lifecycle_copy_discovery d WHERE d.id=NEW.discovery_id AND d.workspace_id=NEW.workspace_id AND d.operation_id=op.id AND d.original_call_id=NEW.copy_call_id AND d.provider_version=dst.provider_version AND d.observed_at<=NEW.observed_at) THEN RAISE EXCEPTION 'unbacked copy materialization'; END IF;
 ELSIF TG_TABLE_NAME='lifecycle_copy_verification' THEN
  IF NEW.content_sha256<>dst.content_sha256 OR NEW.byte_size<>dst.byte_size OR NEW.protection_checked_at>NEW.observed_at OR NOT EXISTS(SELECT 1 FROM lifecycle_storage_call c JOIN lifecycle_storage_observation x ON x.call_id=c.id WHERE c.id=NEW.verify_call_id AND c.copy_operation_id=op.id AND c.copy_id=dst.id AND c.operation='VERIFY_COPY_BYTES' AND x.http_status=200 AND x.failure='' AND x.response_sha256=NEW.content_sha256 AND x.observed_at<=NEW.observed_at) THEN RAISE EXCEPTION 'unbacked exact bytes verification'; END IF;
 ELSIF TG_TABLE_NAME='lifecycle_copy_decision' AND NEW.decision IN('ARCHIVED','ONLINE') THEN
  IF NOT EXISTS(SELECT 1 FROM lifecycle_copy_verification WHERE operation_id=op.id) OR (NEW.decision='ARCHIVED')<>(op.action='ARCHIVE') OR (NEW.decision='ONLINE' AND NEW.checkpoint_id IS NULL) THEN RAISE EXCEPTION 'incomplete copy cannot open'; END IF;
 END IF;
 RETURN NEW;
END;
$$ LANGUAGE plpgsql;
