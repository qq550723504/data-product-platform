-- D1 controlled fixture scope, immutable planning, use restriction, holds and
-- reader admission/termination, scoped approval, DB-only dispatch and physical
-- attempt observations. No provider connection or execution is enabled here.
CREATE TABLE lifecycle_fixture_namespace (
    id uuid PRIMARY KEY CHECK (id <> '00000000-0000-0000-0000-000000000000'),
    workspace_id uuid NOT NULL CHECK (workspace_id <> '00000000-0000-0000-0000-000000000000'),
    provider_instance_id uuid NOT NULL CHECK (provider_instance_id <> '00000000-0000-0000-0000-000000000000'),
    incarnation_id uuid NOT NULL CHECK (incarnation_id <> '00000000-0000-0000-0000-000000000000'),
    namespace varchar(255) NOT NULL CHECK (namespace <> '' AND namespace = btrim(namespace)),
    registered_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (workspace_id, id),
    UNIQUE (provider_instance_id, incarnation_id, namespace)
);

-- Only the isolated, server-owned fixture harness may register these rows.
-- Merely being present is not a receipt proving provider bytes were verified.
CREATE TABLE lifecycle_fixture_copy (
    id uuid PRIMARY KEY CHECK (id <> '00000000-0000-0000-0000-000000000000'),
    workspace_id uuid NOT NULL,
    namespace_id uuid NOT NULL,
    dataset_version_id uuid NOT NULL REFERENCES dataset_version(id),
    object_key varchar(1024) NOT NULL CHECK (object_key <> '' AND object_key = btrim(object_key)),
    provider_version varchar(1024) NOT NULL CHECK (provider_version NOT IN ('', 'null') AND provider_version = btrim(provider_version)),
    content_sha256 varchar(64) NOT NULL CHECK (content_sha256 ~ '^[0-9a-f]{64}$'),
    byte_size bigint NOT NULL CHECK (byte_size BETWEEN 1 AND 134217728),
    registered_at timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (workspace_id, namespace_id) REFERENCES lifecycle_fixture_namespace(workspace_id, id),
    UNIQUE (workspace_id, id),
    UNIQUE (namespace_id, object_key, provider_version)
);

CREATE FUNCTION lifecycle_fixture_copy_workspace_guard() RETURNS trigger AS $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM dataset_version v JOIN dataset d ON d.id=v.dataset_id
        WHERE v.id=NEW.dataset_version_id AND d.workspace_id=NEW.workspace_id
    ) THEN
        RAISE EXCEPTION 'lifecycle fixture copy workspace mismatch';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
CREATE TRIGGER lifecycle_fixture_copy_workspace BEFORE INSERT ON lifecycle_fixture_copy
FOR EACH ROW EXECUTE FUNCTION lifecycle_fixture_copy_workspace_guard();

CREATE FUNCTION lifecycle_fixture_identity_immutable() RETURNS trigger AS $$
BEGIN
    RAISE EXCEPTION 'lifecycle fixture identity is immutable';
END;
$$ LANGUAGE plpgsql;
CREATE TRIGGER lifecycle_fixture_namespace_immutable BEFORE UPDATE OR DELETE ON lifecycle_fixture_namespace
FOR EACH ROW EXECUTE FUNCTION lifecycle_fixture_identity_immutable();
CREATE TRIGGER lifecycle_fixture_copy_immutable BEFORE UPDATE OR DELETE ON lifecycle_fixture_copy
FOR EACH ROW EXECUTE FUNCTION lifecycle_fixture_identity_immutable();
CREATE TRIGGER lifecycle_fixture_namespace_no_truncate BEFORE TRUNCATE ON lifecycle_fixture_namespace
FOR EACH STATEMENT EXECUTE FUNCTION lifecycle_fixture_identity_immutable();
CREATE TRIGGER lifecycle_fixture_copy_no_truncate BEFORE TRUNCATE ON lifecycle_fixture_copy
FOR EACH STATEMENT EXECUTE FUNCTION lifecycle_fixture_identity_immutable();

CREATE TABLE lifecycle_disposition_plan (
    id uuid PRIMARY KEY CHECK (id <> '00000000-0000-0000-0000-000000000000'),
    workspace_id uuid NOT NULL CHECK (workspace_id <> '00000000-0000-0000-0000-000000000000'),
    action varchar(32) NOT NULL CHECK (action = 'DELETE_EXACT'),
    status varchar(16) NOT NULL DEFAULT 'DRAFT' CHECK (status IN ('DRAFT','FINALIZED')),
    prepared_by uuid NOT NULL CHECK (prepared_by <> '00000000-0000-0000-0000-000000000000'),
    principal_ref varchar(255) NOT NULL CHECK (principal_ref <> '' AND principal_ref = btrim(principal_ref)),
    dependency_revision bigint NOT NULL CHECK (dependency_revision >= 0),
    created_at timestamptz NOT NULL DEFAULT now(),
    finalized_at timestamptz,
    content_hash varchar(64),
    UNIQUE (workspace_id, id),
    CHECK ((status='DRAFT' AND finalized_at IS NULL AND content_hash IS NULL) OR
           (status='FINALIZED' AND finalized_at IS NOT NULL AND finalized_at >= created_at AND content_hash ~ '^[0-9a-f]{64}$'))
);

CREATE TABLE lifecycle_disposition_plan_target (
    plan_id uuid NOT NULL,
    workspace_id uuid NOT NULL,
    copy_id uuid NOT NULL,
    PRIMARY KEY (plan_id, copy_id),
    FOREIGN KEY (workspace_id, plan_id) REFERENCES lifecycle_disposition_plan(workspace_id, id),
    FOREIGN KEY (workspace_id, copy_id) REFERENCES lifecycle_fixture_copy(workspace_id, id)
);

-- Lock the plan before touching membership. Late DRAFT writers cannot commit
-- after finalization; UPDATE cannot move a row to another parent.
CREATE FUNCTION lifecycle_plan_target_guard() RETURNS trigger AS $$
DECLARE parent_id uuid; parent_status text;
BEGIN
    IF TG_OP='UPDATE' THEN RAISE EXCEPTION 'replace draft plan target via explicit remove/add'; END IF;
    IF TG_OP='DELETE' THEN parent_id := OLD.plan_id; ELSE parent_id := NEW.plan_id; END IF;
    SELECT status INTO parent_status FROM lifecycle_disposition_plan WHERE id=parent_id FOR UPDATE;
    IF parent_status IS DISTINCT FROM 'DRAFT' THEN RAISE EXCEPTION 'lifecycle plan membership is frozen'; END IF;
    IF TG_OP='DELETE' THEN RETURN OLD; ELSE RETURN NEW; END IF;
END;
$$ LANGUAGE plpgsql;
CREATE TRIGGER lifecycle_plan_target_freeze BEFORE INSERT OR UPDATE OR DELETE ON lifecycle_disposition_plan_target
FOR EACH ROW EXECUTE FUNCTION lifecycle_plan_target_guard();

CREATE FUNCTION lifecycle_plan_frame(value text) RETURNS text AS $$
    SELECT octet_length(convert_to(value,'UTF8'))::text || ':' || value;
$$ LANGUAGE sql IMMUTABLE STRICT;

CREATE FUNCTION lifecycle_plan_scope_hash(plan uuid) RETURNS text AS $$
DECLARE payload text; target record; header record;
BEGIN
    SELECT workspace_id, action INTO STRICT header FROM lifecycle_disposition_plan WHERE id=plan;
    payload := lifecycle_plan_frame('SHA256-LIFECYCLE-PLAN-V1') ||
               lifecycle_plan_frame(header.workspace_id::text) || lifecycle_plan_frame(header.action);
    FOR target IN
        SELECT c.*, n.provider_instance_id, n.incarnation_id, n.namespace
        FROM lifecycle_disposition_plan_target t JOIN lifecycle_fixture_copy c ON c.id=t.copy_id
        JOIN lifecycle_fixture_namespace n ON n.id=c.namespace_id
        WHERE t.plan_id=plan ORDER BY c.id
    LOOP
        payload := payload || lifecycle_plan_frame(target.id::text) || lifecycle_plan_frame(target.dataset_version_id::text) ||
            lifecycle_plan_frame(target.provider_instance_id::text) || lifecycle_plan_frame(target.incarnation_id::text) ||
            lifecycle_plan_frame(target.namespace) || lifecycle_plan_frame(target.object_key) ||
            lifecycle_plan_frame(target.provider_version) || lifecycle_plan_frame(target.content_sha256) ||
            lifecycle_plan_frame(target.byte_size::text);
    END LOOP;
    RETURN encode(digest(convert_to(payload,'UTF8'),'sha256'),'hex');
END;
$$ LANGUAGE plpgsql;

CREATE FUNCTION lifecycle_plan_finalize_guard() RETURNS trigger AS $$
DECLARE member_count bigint;
BEGIN
    IF TG_OP='INSERT' THEN
        IF NEW.status <> 'DRAFT' THEN RAISE EXCEPTION 'lifecycle plan starts in DRAFT'; END IF;
        RETURN NEW;
    END IF;
    IF TG_OP='DELETE' THEN RAISE EXCEPTION 'lifecycle plan history cannot be deleted'; END IF;
    IF OLD.status <> 'DRAFT' OR NEW.status <> 'FINALIZED' OR
       (to_jsonb(NEW) - ARRAY['status','finalized_at','content_hash']) IS DISTINCT FROM
       (to_jsonb(OLD) - ARRAY['status','finalized_at','content_hash']) THEN
        RAISE EXCEPTION 'only exact lifecycle plan finalization is allowed';
    END IF;
    SELECT count(*) INTO member_count FROM lifecycle_disposition_plan_target WHERE plan_id=OLD.id;
    IF member_count NOT BETWEEN 1 AND 64 OR NEW.content_hash IS DISTINCT FROM lifecycle_plan_scope_hash(OLD.id) THEN
        RAISE EXCEPTION 'lifecycle plan exact membership/hash mismatch';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
CREATE TRIGGER lifecycle_plan_finalize BEFORE INSERT OR UPDATE OR DELETE ON lifecycle_disposition_plan
FOR EACH ROW EXECUTE FUNCTION lifecycle_plan_finalize_guard();
CREATE TRIGGER lifecycle_plan_no_truncate BEFORE TRUNCATE ON lifecycle_disposition_plan
FOR EACH STATEMENT EXECUTE FUNCTION lifecycle_fixture_identity_immutable();
CREATE TRIGGER lifecycle_plan_target_no_truncate BEFORE TRUNCATE ON lifecycle_disposition_plan_target
FOR EACH STATEMENT EXECUTE FUNCTION lifecycle_fixture_identity_immutable();

CREATE TABLE lifecycle_content_use_restriction (
    id uuid PRIMARY KEY CHECK (id <> '00000000-0000-0000-0000-000000000000'),
    workspace_id uuid NOT NULL,
    dataset_version_id uuid NOT NULL REFERENCES dataset_version(id),
    basis_ref varchar(255) NOT NULL CHECK (basis_ref <> '' AND basis_ref=btrim(basis_ref)),
    actor_id uuid NOT NULL CHECK (actor_id <> '00000000-0000-0000-0000-000000000000'),
    principal_ref varchar(255) NOT NULL CHECK (principal_ref <> ''),
    dependency_revision bigint NOT NULL CHECK (dependency_revision>0),
    occurred_at timestamptz NOT NULL,
    UNIQUE (workspace_id,id)
);
CREATE INDEX lifecycle_restriction_subject ON lifecycle_content_use_restriction(workspace_id,dataset_version_id,occurred_at);
CREATE FUNCTION lifecycle_restriction_scope_guard() RETURNS trigger AS $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM lifecycle_fixture_copy c WHERE c.workspace_id=NEW.workspace_id AND c.dataset_version_id=NEW.dataset_version_id) THEN
        RAISE EXCEPTION 'lifecycle restriction requires registered fixture scope';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
CREATE TRIGGER lifecycle_restriction_scope BEFORE INSERT ON lifecycle_content_use_restriction
FOR EACH ROW EXECUTE FUNCTION lifecycle_restriction_scope_guard();

CREATE TABLE lifecycle_content_hold (
    id uuid PRIMARY KEY CHECK (id <> '00000000-0000-0000-0000-000000000000'),
    workspace_id uuid NOT NULL,
    copy_id uuid NOT NULL,
    basis_ref varchar(255) NOT NULL CHECK (basis_ref <> '' AND basis_ref=btrim(basis_ref)),
    actor_id uuid NOT NULL CHECK (actor_id <> '00000000-0000-0000-0000-000000000000'),
    principal_ref varchar(255) NOT NULL CHECK (principal_ref <> ''),
    dependency_revision bigint NOT NULL CHECK (dependency_revision>0),
    occurred_at timestamptz NOT NULL,
    FOREIGN KEY (workspace_id,copy_id) REFERENCES lifecycle_fixture_copy(workspace_id,id),
    UNIQUE (workspace_id,id)
);
CREATE TABLE lifecycle_content_hold_release (
    id uuid PRIMARY KEY CHECK (id <> '00000000-0000-0000-0000-000000000000'),
    workspace_id uuid NOT NULL,
    hold_id uuid NOT NULL UNIQUE,
    basis_ref varchar(255) NOT NULL CHECK (basis_ref <> '' AND basis_ref=btrim(basis_ref)),
    actor_id uuid NOT NULL CHECK (actor_id <> '00000000-0000-0000-0000-000000000000'),
    principal_ref varchar(255) NOT NULL CHECK (principal_ref <> ''),
    dependency_revision bigint NOT NULL CHECK (dependency_revision>0),
    occurred_at timestamptz NOT NULL,
    FOREIGN KEY (workspace_id,hold_id) REFERENCES lifecycle_content_hold(workspace_id,id)
);

CREATE TABLE lifecycle_reader_pin (
    id uuid PRIMARY KEY CHECK (id <> '00000000-0000-0000-0000-000000000000'),
    workspace_id uuid NOT NULL,
    copy_id uuid NOT NULL,
    reader_id uuid NOT NULL CHECK (reader_id <> '00000000-0000-0000-0000-000000000000'),
    kind varchar(32) NOT NULL CHECK (kind='FIXTURE_READER'),
    actor_id uuid NOT NULL CHECK (actor_id <> '00000000-0000-0000-0000-000000000000'),
    principal_ref varchar(255) NOT NULL CHECK (principal_ref <> ''),
    generation bigint NOT NULL CHECK (generation>0),
    gate_revision bigint NOT NULL CHECK (gate_revision>=0 AND generation>gate_revision),
    started_at timestamptz NOT NULL,
    window_mode varchar(32) NOT NULL CHECK (window_mode IN ('EXPLICIT_UNBOUNDED','DEADLINE')),
    use_until timestamptz,
    FOREIGN KEY (workspace_id,copy_id) REFERENCES lifecycle_fixture_copy(workspace_id,id),
    UNIQUE (workspace_id,id),
    CHECK ((window_mode='EXPLICIT_UNBOUNDED' AND use_until IS NULL) OR
           (window_mode='DEADLINE' AND use_until IS NOT NULL AND use_until>started_at))
);

CREATE FUNCTION lifecycle_hold_release_scope_guard() RETURNS trigger AS $$
DECLARE placed_at timestamptz;
BEGIN
    SELECT occurred_at INTO STRICT placed_at FROM lifecycle_content_hold WHERE workspace_id=NEW.workspace_id AND id=NEW.hold_id FOR UPDATE;
    IF NEW.occurred_at<placed_at THEN RAISE EXCEPTION 'hold release precedes placement'; END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
CREATE TRIGGER lifecycle_hold_release_scope BEFORE INSERT ON lifecycle_content_hold_release
FOR EACH ROW EXECUTE FUNCTION lifecycle_hold_release_scope_guard();
CREATE INDEX lifecycle_reader_copy ON lifecycle_reader_pin(workspace_id,copy_id);
CREATE TABLE lifecycle_reader_termination (
    id uuid PRIMARY KEY CHECK (id <> '00000000-0000-0000-0000-000000000000'),
    workspace_id uuid NOT NULL,
    pin_id uuid NOT NULL UNIQUE,
    reader_id uuid NOT NULL,
    generation bigint NOT NULL,
    outcome varchar(32) NOT NULL CHECK (outcome IN ('COMPLETED','CANCELLATION_CONFIRMED')),
    observed_at timestamptz NOT NULL,
    actor_id uuid NOT NULL CHECK (actor_id <> '00000000-0000-0000-0000-000000000000'),
    principal_ref varchar(255) NOT NULL CHECK (principal_ref <> ''),
    dependency_revision bigint NOT NULL CHECK (dependency_revision>0),
    FOREIGN KEY (workspace_id,pin_id) REFERENCES lifecycle_reader_pin(workspace_id,id)
);

CREATE FUNCTION lifecycle_termination_scope_guard() RETURNS trigger AS $$
DECLARE pin lifecycle_reader_pin;
BEGIN
    SELECT * INTO STRICT pin FROM lifecycle_reader_pin WHERE workspace_id=NEW.workspace_id AND id=NEW.pin_id FOR UPDATE;
    IF NEW.reader_id<>pin.reader_id OR NEW.generation<>pin.generation OR NEW.observed_at<pin.started_at THEN
        RAISE EXCEPTION 'reader termination does not match pin generation';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
CREATE TRIGGER lifecycle_termination_scope BEFORE INSERT ON lifecycle_reader_termination
FOR EACH ROW EXECUTE FUNCTION lifecycle_termination_scope_guard();

-- All current state is resolved from these append-only facts. No expiry-based
-- refcount or mutable ALLOWED flag can silently release historical occupancy.
DO $$ DECLARE name text; BEGIN
    FOREACH name IN ARRAY ARRAY['lifecycle_content_use_restriction','lifecycle_content_hold',
        'lifecycle_content_hold_release','lifecycle_reader_pin','lifecycle_reader_termination'] LOOP
        EXECUTE format('CREATE TRIGGER %I BEFORE UPDATE OR DELETE ON %I FOR EACH ROW EXECUTE FUNCTION lifecycle_fixture_identity_immutable()',name||'_immutable',name);
        EXECUTE format('CREATE TRIGGER %I BEFORE TRUNCATE ON %I FOR EACH STATEMENT EXECUTE FUNCTION lifecycle_fixture_identity_immutable()',name||'_no_truncate',name);
    END LOOP;
END $$;

CREATE TABLE lifecycle_disposition_approval (
 id uuid PRIMARY KEY CHECK(id <> '00000000-0000-0000-0000-000000000000'),
 workspace_id uuid NOT NULL, plan_id uuid NOT NULL, plan_hash varchar(64) NOT NULL,
 action varchar(32) NOT NULL CHECK(action='DELETE_EXACT'),
 basis_ref varchar(255) NOT NULL CHECK(basis_ref<>'' AND basis_ref=btrim(basis_ref)),
 actor_id uuid NOT NULL CHECK(actor_id <> '00000000-0000-0000-0000-000000000000'), principal_ref varchar(255) NOT NULL CHECK(principal_ref<>''),
 valid_from timestamptz NOT NULL, expires_at timestamptz NOT NULL CHECK(expires_at>valid_from),
 dependency_revision bigint NOT NULL CHECK(dependency_revision>0),
 FOREIGN KEY(workspace_id,plan_id) REFERENCES lifecycle_disposition_plan(workspace_id,id), UNIQUE(workspace_id,id)
);
CREATE FUNCTION lifecycle_approval_scope_guard() RETURNS trigger AS $$
DECLARE p lifecycle_disposition_plan;
BEGIN
 SELECT * INTO STRICT p FROM lifecycle_disposition_plan WHERE workspace_id=NEW.workspace_id AND id=NEW.plan_id FOR UPDATE;
 IF p.status<>'FINALIZED' OR NEW.plan_hash<>p.content_hash OR NEW.action<>p.action OR NEW.valid_from<p.finalized_at THEN
  RAISE EXCEPTION 'approval requires finalized exact scope';
 END IF;
 RETURN NEW;
END; $$ LANGUAGE plpgsql;
CREATE TRIGGER lifecycle_approval_scope BEFORE INSERT ON lifecycle_disposition_approval FOR EACH ROW EXECUTE FUNCTION lifecycle_approval_scope_guard();
CREATE TABLE lifecycle_approval_revocation (
 id uuid PRIMARY KEY CHECK(id <> '00000000-0000-0000-0000-000000000000'), workspace_id uuid NOT NULL,
 approval_id uuid NOT NULL UNIQUE, basis_ref varchar(255) NOT NULL CHECK(basis_ref<>'' AND basis_ref=btrim(basis_ref)),
 actor_id uuid NOT NULL CHECK(actor_id <> '00000000-0000-0000-0000-000000000000'), principal_ref varchar(255) NOT NULL CHECK(principal_ref<>''),
 occurred_at timestamptz NOT NULL, dependency_revision bigint NOT NULL CHECK(dependency_revision>0),
 FOREIGN KEY(workspace_id,approval_id) REFERENCES lifecycle_disposition_approval(workspace_id,id)
);
CREATE TABLE lifecycle_disposition_gate_evaluation (
 id uuid PRIMARY KEY CHECK(id <> '00000000-0000-0000-0000-000000000000'), workspace_id uuid NOT NULL,
 plan_id uuid NOT NULL, approval_id uuid NOT NULL, copy_id uuid NOT NULL,
 actor_id uuid NOT NULL CHECK(actor_id <> '00000000-0000-0000-0000-000000000000'), principal_ref varchar(255) NOT NULL CHECK(principal_ref<>''),
 checked_at timestamptz NOT NULL, dependency_revision bigint NOT NULL CHECK(dependency_revision>=0),
 status varchar(16) NOT NULL DEFAULT 'DRAFT' CHECK(status IN ('DRAFT','FINALIZED')),
 retention_count integer NOT NULL CHECK(retention_count>=0), content_hash varchar(64),
 CHECK((status='DRAFT' AND content_hash IS NULL) OR (status='FINALIZED' AND content_hash ~ '^[0-9a-f]{64}$')),
 eligible boolean NOT NULL, blockers text[] NOT NULL,
 CHECK((eligible AND cardinality(blockers)=0) OR (NOT eligible AND cardinality(blockers)>0)),
 FOREIGN KEY(workspace_id,plan_id) REFERENCES lifecycle_disposition_plan(workspace_id,id),
 FOREIGN KEY(workspace_id,approval_id) REFERENCES lifecycle_disposition_approval(workspace_id,id),
 FOREIGN KEY(workspace_id,copy_id) REFERENCES lifecycle_fixture_copy(workspace_id,id), UNIQUE(workspace_id,id)
);
CREATE FUNCTION lifecycle_disposition_gate_scope_guard() RETURNS trigger AS $$
BEGIN
 IF NOT EXISTS(SELECT 1 FROM lifecycle_disposition_approval a JOIN lifecycle_disposition_plan_target t ON t.workspace_id=a.workspace_id AND t.plan_id=a.plan_id
  WHERE a.workspace_id=NEW.workspace_id AND a.id=NEW.approval_id AND a.plan_id=NEW.plan_id AND t.copy_id=NEW.copy_id) THEN
  RAISE EXCEPTION 'disposition gate scope mismatch';
 END IF;
 RETURN NEW;
END; $$ LANGUAGE plpgsql;
CREATE TRIGGER lifecycle_gate_scope BEFORE INSERT ON lifecycle_disposition_gate_evaluation FOR EACH ROW EXECUTE FUNCTION lifecycle_disposition_gate_scope_guard();
-- Typed immutable evaluation inputs, including explicit UNKNOWN, explain each
-- decision without placing policy time/identity or occupancy in opaque JSON.
CREATE TABLE lifecycle_disposition_dependency_evaluation (
 evaluation_id uuid PRIMARY KEY, workspace_id uuid NOT NULL, disposal_due_at timestamptz,
 inventory_complete varchar(16) NOT NULL CHECK(inventory_complete IN ('SATISFIED','DENIED','UNKNOWN')),
 references_released varchar(16) NOT NULL CHECK(references_released IN ('SATISFIED','DENIED','UNKNOWN')),
 provider_retention_allows varchar(16) NOT NULL CHECK(provider_retention_allows IN ('SATISFIED','DENIED','UNKNOWN')),
 ledger_current varchar(16) NOT NULL CHECK(ledger_current IN ('SATISFIED','DENIED','UNKNOWN')),
 use_restricted varchar(16) NOT NULL CHECK(use_restricted IN ('SATISFIED','DENIED','UNKNOWN')),
 hold_free varchar(16) NOT NULL CHECK(hold_free IN ('SATISFIED','DENIED','UNKNOWN')),
 readers_drained varchar(16) NOT NULL CHECK(readers_drained IN ('SATISFIED','DENIED','UNKNOWN')),
 unresolved_dispatches bigint NOT NULL CHECK(unresolved_dispatches>=0),
 window_mode varchar(32) NOT NULL CHECK(window_mode IN ('EXPLICIT_UNBOUNDED','DEADLINE','UNKNOWN')), valid_until timestamptz,
 FOREIGN KEY(workspace_id,evaluation_id) REFERENCES lifecycle_disposition_gate_evaluation(workspace_id,id)
);
CREATE TABLE lifecycle_disposition_retention_evaluation (
 evaluation_id uuid NOT NULL, workspace_id uuid NOT NULL, ordinal integer NOT NULL CHECK(ordinal>=0),
 policy_id uuid, start_event_id uuid, starts_at timestamptz, retain_until timestamptz,
 PRIMARY KEY(evaluation_id,ordinal), FOREIGN KEY(workspace_id,evaluation_id) REFERENCES lifecycle_disposition_gate_evaluation(workspace_id,id)
);
CREATE FUNCTION lifecycle_gate_input_guard() RETURNS trigger AS $$
DECLARE s text;
BEGIN
 SELECT status INTO STRICT s FROM lifecycle_disposition_gate_evaluation WHERE workspace_id=NEW.workspace_id AND id=NEW.evaluation_id FOR UPDATE;
 IF s<>'DRAFT' THEN RAISE EXCEPTION 'gate inputs are frozen'; END IF;
 RETURN NEW;
END; $$ LANGUAGE plpgsql;
CREATE TRIGGER lifecycle_dependency_input_scope BEFORE INSERT ON lifecycle_disposition_dependency_evaluation FOR EACH ROW EXECUTE FUNCTION lifecycle_gate_input_guard();
CREATE TRIGGER lifecycle_retention_input_scope BEFORE INSERT ON lifecycle_disposition_retention_evaluation FOR EACH ROW EXECUTE FUNCTION lifecycle_gate_input_guard();
CREATE FUNCTION lifecycle_gate_inputs_hash(gate_id uuid) RETURNS text AS $$
 SELECT encode(digest(
  lifecycle_plan_frame(coalesce(to_char(d.disposal_due_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS.US"Z"'),'UNKNOWN'))||
  lifecycle_plan_frame(d.inventory_complete)||lifecycle_plan_frame(d.references_released)||lifecycle_plan_frame(d.provider_retention_allows)||
  lifecycle_plan_frame(d.ledger_current)||lifecycle_plan_frame(d.use_restricted)||lifecycle_plan_frame(d.hold_free)||
  lifecycle_plan_frame(d.readers_drained)||lifecycle_plan_frame(d.unresolved_dispatches::text)||lifecycle_plan_frame(d.window_mode)||
  lifecycle_plan_frame(coalesce(to_char(d.valid_until AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS.US"Z"'),'UNKNOWN'))||
  coalesce((SELECT string_agg(lifecycle_plan_frame(r.ordinal::text)||lifecycle_plan_frame(coalesce(r.policy_id::text,'UNKNOWN'))||
   lifecycle_plan_frame(coalesce(r.start_event_id::text,'UNKNOWN'))||
   lifecycle_plan_frame(coalesce(to_char(r.starts_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS.US"Z"'),'UNKNOWN'))||
   lifecycle_plan_frame(coalesce(to_char(r.retain_until AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS.US"Z"'),'UNKNOWN')),'' ORDER BY r.ordinal)
   FROM lifecycle_disposition_retention_evaluation r WHERE r.evaluation_id=gate_id),'')
 ,'sha256'),'hex') FROM lifecycle_disposition_dependency_evaluation d WHERE d.evaluation_id=gate_id;
$$ LANGUAGE SQL STABLE;
CREATE FUNCTION lifecycle_gate_finalize_guard() RETURNS trigger AS $$
BEGIN
 IF TG_OP='INSERT' THEN
  IF NEW.status<>'DRAFT' OR NEW.content_hash IS NOT NULL THEN RAISE EXCEPTION 'gate starts in DRAFT'; END IF;
  RETURN NEW;
 END IF;
 IF TG_OP='DELETE' OR OLD.status<>'DRAFT' OR NEW.status<>'FINALIZED' OR
  (to_jsonb(NEW)-'status'-'content_hash') IS DISTINCT FROM (to_jsonb(OLD)-'status'-'content_hash') THEN
  RAISE EXCEPTION 'gate history is immutable';
 END IF;
 IF (SELECT count(*) FROM lifecycle_disposition_dependency_evaluation WHERE evaluation_id=OLD.id)<>1 OR
  (SELECT count(*) FROM lifecycle_disposition_retention_evaluation WHERE evaluation_id=OLD.id)<>OLD.retention_count OR
  EXISTS(SELECT 1 FROM lifecycle_disposition_retention_evaluation WHERE evaluation_id=OLD.id AND ordinal>=OLD.retention_count) OR
  NEW.content_hash IS DISTINCT FROM lifecycle_gate_inputs_hash(OLD.id) THEN RAISE EXCEPTION 'gate inputs are incomplete or hash differs'; END IF;
 RETURN NEW;
END; $$ LANGUAGE plpgsql;
CREATE TRIGGER lifecycle_gate_finalize BEFORE INSERT OR UPDATE OR DELETE ON lifecycle_disposition_gate_evaluation FOR EACH ROW EXECUTE FUNCTION lifecycle_gate_finalize_guard();
CREATE TRIGGER lifecycle_gate_no_truncate BEFORE TRUNCATE ON lifecycle_disposition_gate_evaluation FOR EACH STATEMENT EXECUTE FUNCTION lifecycle_fixture_identity_immutable();
CREATE TABLE lifecycle_disposition_dispatch (
 id uuid PRIMARY KEY CHECK(id <> '00000000-0000-0000-0000-000000000000'), workspace_id uuid NOT NULL,
 plan_id uuid NOT NULL, approval_id uuid NOT NULL, copy_id uuid NOT NULL, evaluation_id uuid NOT NULL UNIQUE,
 dispatched_at timestamptz NOT NULL, dependency_revision bigint NOT NULL CHECK(dependency_revision>0),
 FOREIGN KEY(workspace_id,plan_id) REFERENCES lifecycle_disposition_plan(workspace_id,id),
 FOREIGN KEY(workspace_id,approval_id) REFERENCES lifecycle_disposition_approval(workspace_id,id),
 FOREIGN KEY(workspace_id,copy_id) REFERENCES lifecycle_fixture_copy(workspace_id,id),
 FOREIGN KEY(workspace_id,evaluation_id) REFERENCES lifecycle_disposition_gate_evaluation(workspace_id,id),
 UNIQUE(workspace_id,id), UNIQUE(workspace_id,id,plan_id,copy_id)
);
CREATE INDEX lifecycle_dispatch_copy ON lifecycle_disposition_dispatch(workspace_id,copy_id);
CREATE TABLE lifecycle_provider_attempt (
 id uuid PRIMARY KEY CHECK(id <> '00000000-0000-0000-0000-000000000000'), workspace_id uuid NOT NULL,
 dispatch_id uuid NOT NULL UNIQUE, plan_id uuid NOT NULL, copy_id uuid NOT NULL,
 action varchar(32) NOT NULL CHECK(action='DELETE_EXACT'), provider_request_key varchar(128) NOT NULL UNIQUE,
 CHECK(provider_request_key='lifecycle-delete:'||id::text),
 started_at timestamptz NOT NULL, dependency_revision bigint NOT NULL CHECK(dependency_revision>0),
 FOREIGN KEY(workspace_id,dispatch_id,plan_id,copy_id) REFERENCES lifecycle_disposition_dispatch(workspace_id,id,plan_id,copy_id), UNIQUE(workspace_id,id)
);
CREATE TABLE lifecycle_provider_observation (
 id uuid PRIMARY KEY CHECK(id <> '00000000-0000-0000-0000-000000000000'), workspace_id uuid NOT NULL, attempt_id uuid NOT NULL,
 certainty varchar(16) NOT NULL CHECK(certainty IN ('UNCERTAIN','CONFIRMED','NOT_SENT')),
 outcome varchar(16) NOT NULL CHECK(outcome IN ('ACCEPTED','DENIED','FAILED','UNKNOWN','NOT_SENT')),
 proof_sha256 varchar(64) NOT NULL CHECK(proof_sha256 ~ '^[0-9a-f]{64}$'), observed_at timestamptz NOT NULL, invoked_at timestamptz,
 actor_id uuid NOT NULL CHECK(actor_id <> '00000000-0000-0000-0000-000000000000'), principal_ref varchar(255) NOT NULL CHECK(principal_ref<>''),
 dependency_revision bigint NOT NULL CHECK(dependency_revision>0),
 CHECK((certainty='UNCERTAIN' AND outcome='UNKNOWN' AND invoked_at IS NULL) OR
       (certainty='NOT_SENT' AND outcome='NOT_SENT' AND invoked_at IS NULL) OR
       (certainty='CONFIRMED' AND outcome IN ('ACCEPTED','DENIED','FAILED','UNKNOWN') AND invoked_at IS NOT NULL AND invoked_at<=observed_at)),
 FOREIGN KEY(workspace_id,attempt_id) REFERENCES lifecycle_provider_attempt(workspace_id,id), UNIQUE(workspace_id,id)
);
CREATE INDEX lifecycle_observation_attempt ON lifecycle_provider_observation(attempt_id);
CREATE FUNCTION lifecycle_dispatch_complete_guard() RETURNS trigger AS $$
BEGIN
 IF NOT EXISTS(SELECT 1 FROM lifecycle_provider_attempt a WHERE a.workspace_id=NEW.workspace_id AND a.dispatch_id=NEW.id
  AND a.started_at>=NEW.dispatched_at AND a.dependency_revision=NEW.dependency_revision) THEN RAISE EXCEPTION 'dispatch requires committed physical start'; END IF;
 RETURN NULL;
END; $$ LANGUAGE plpgsql;
CREATE CONSTRAINT TRIGGER lifecycle_dispatch_complete AFTER INSERT ON lifecycle_disposition_dispatch
 DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION lifecycle_dispatch_complete_guard();
CREATE FUNCTION lifecycle_observation_guard() RETURNS trigger AS $$
DECLARE a lifecycle_provider_attempt;
BEGIN
 SELECT * INTO STRICT a FROM lifecycle_provider_attempt WHERE workspace_id=NEW.workspace_id AND id=NEW.attempt_id FOR UPDATE;
 IF NEW.observed_at<a.started_at OR NEW.invoked_at<a.started_at THEN RAISE EXCEPTION 'observation precedes physical start'; END IF;
 IF EXISTS(SELECT 1 FROM lifecycle_provider_observation o WHERE o.attempt_id=a.id AND
  ((o.certainty='CONFIRMED' AND NEW.certainty='NOT_SENT') OR (o.certainty='NOT_SENT' AND NEW.certainty='CONFIRMED') OR
   (o.certainty='CONFIRMED' AND NEW.certainty='CONFIRMED' AND o.invoked_at IS DISTINCT FROM NEW.invoked_at))) THEN
  RAISE EXCEPTION 'physical invocation observations conflict';
 END IF;
 RETURN NEW;
END; $$ LANGUAGE plpgsql;
CREATE TRIGGER lifecycle_observation_scope BEFORE INSERT ON lifecycle_provider_observation FOR EACH ROW EXECUTE FUNCTION lifecycle_observation_guard();
CREATE TABLE lifecycle_dispatch_release (
 dispatch_id uuid PRIMARY KEY, workspace_id uuid NOT NULL, observation_id uuid NOT NULL,
 FOREIGN KEY(workspace_id,dispatch_id) REFERENCES lifecycle_disposition_dispatch(workspace_id,id),
 FOREIGN KEY(workspace_id,observation_id) REFERENCES lifecycle_provider_observation(workspace_id,id)
);
CREATE FUNCTION lifecycle_dispatch_release_guard() RETURNS trigger AS $$
BEGIN
 IF NOT EXISTS(SELECT 1 FROM lifecycle_provider_observation o JOIN lifecycle_provider_attempt a ON a.id=o.attempt_id
  WHERE o.workspace_id=NEW.workspace_id AND o.id=NEW.observation_id AND a.dispatch_id=NEW.dispatch_id AND o.certainty='NOT_SENT') THEN
  RAISE EXCEPTION 'reservation release requires authoritative NOT_SENT';
 END IF;
 RETURN NEW;
END; $$ LANGUAGE plpgsql;
CREATE TRIGGER lifecycle_dispatch_release_scope BEFORE INSERT ON lifecycle_dispatch_release FOR EACH ROW EXECUTE FUNCTION lifecycle_dispatch_release_guard();
CREATE FUNCTION lifecycle_dispatch_guard() RETURNS trigger AS $$
DECLARE g lifecycle_disposition_gate_evaluation;
BEGIN
 -- The copy parent serializes exclusive admission even for raw SQL writers.
 PERFORM id FROM lifecycle_fixture_copy WHERE workspace_id=NEW.workspace_id AND id=NEW.copy_id FOR UPDATE;
 SELECT * INTO STRICT g FROM lifecycle_disposition_gate_evaluation WHERE workspace_id=NEW.workspace_id AND id=NEW.evaluation_id;
 IF NOT g.eligible OR g.status<>'FINALIZED' OR g.plan_id<>NEW.plan_id OR g.approval_id<>NEW.approval_id OR g.copy_id<>NEW.copy_id OR
  NEW.dispatched_at<g.checked_at OR NEW.dependency_revision<=g.dependency_revision THEN RAISE EXCEPTION 'dispatch requires exact allowed gate'; END IF;
 IF EXISTS(SELECT 1 FROM lifecycle_disposition_dispatch d WHERE d.workspace_id=NEW.workspace_id AND d.copy_id=NEW.copy_id
  AND NOT EXISTS(SELECT 1 FROM lifecycle_dispatch_release r WHERE r.dispatch_id=d.id)) THEN RAISE EXCEPTION 'copy has unresolved dispatch'; END IF;
 RETURN NEW;
END; $$ LANGUAGE plpgsql;
CREATE TRIGGER lifecycle_dispatch_scope BEFORE INSERT ON lifecycle_disposition_dispatch FOR EACH ROW EXECUTE FUNCTION lifecycle_dispatch_guard();
CREATE TABLE lifecycle_hold_dispatch_conflict (
 hold_id uuid NOT NULL, workspace_id uuid NOT NULL, dispatch_id uuid NOT NULL,
 PRIMARY KEY(hold_id,dispatch_id), FOREIGN KEY(workspace_id,hold_id) REFERENCES lifecycle_content_hold(workspace_id,id),
 FOREIGN KEY(workspace_id,dispatch_id) REFERENCES lifecycle_disposition_dispatch(workspace_id,id)
);
CREATE FUNCTION lifecycle_hold_conflict_guard() RETURNS trigger AS $$
BEGIN
 IF NOT EXISTS(SELECT 1 FROM lifecycle_content_hold h JOIN lifecycle_disposition_dispatch d ON d.workspace_id=h.workspace_id AND d.copy_id=h.copy_id
  WHERE h.workspace_id=NEW.workspace_id AND h.id=NEW.hold_id AND d.id=NEW.dispatch_id) THEN RAISE EXCEPTION 'hold conflict scope mismatch'; END IF;
 RETURN NEW;
END; $$ LANGUAGE plpgsql;
CREATE TRIGGER lifecycle_hold_conflict_scope BEFORE INSERT ON lifecycle_hold_dispatch_conflict FOR EACH ROW EXECUTE FUNCTION lifecycle_hold_conflict_guard();
DO $$ DECLARE name text; BEGIN
 FOREACH name IN ARRAY ARRAY['lifecycle_disposition_approval','lifecycle_approval_revocation',
  'lifecycle_disposition_dependency_evaluation','lifecycle_disposition_retention_evaluation',
  'lifecycle_disposition_dispatch','lifecycle_provider_attempt','lifecycle_provider_observation','lifecycle_dispatch_release','lifecycle_hold_dispatch_conflict'] LOOP
  EXECUTE format('CREATE TRIGGER %I BEFORE UPDATE OR DELETE ON %I FOR EACH ROW EXECUTE FUNCTION lifecycle_fixture_identity_immutable()',name||'_immutable',name);
  EXECUTE format('CREATE TRIGGER %I BEFORE TRUNCATE ON %I FOR EACH STATEMENT EXECUTE FUNCTION lifecycle_fixture_identity_immutable()',name||'_no_truncate',name);
 END LOOP;
END $$;

-- One physical non-destructive HTTP call, distinct from delete dispatch. The
-- complete exact target is resolved through immutable registered copy facts.
CREATE TABLE lifecycle_storage_call (
 id uuid PRIMARY KEY CHECK(id <> '00000000-0000-0000-0000-000000000000'),
 workspace_id uuid NOT NULL,
 copy_id uuid NOT NULL,
 operation varchar(40) NOT NULL CHECK(operation IN ('STAT_EXACT','INSPECT_RETENTION','INSPECT_LEGAL_HOLD','LIST_NAMESPACE_VERSIONS')),
 started_at timestamptz NOT NULL,
 actor_id uuid NOT NULL CHECK(actor_id <> '00000000-0000-0000-0000-000000000000'),
 principal_ref varchar(255) NOT NULL CHECK(principal_ref <> ''),
 dependency_revision bigint NOT NULL CHECK(dependency_revision >= 0),
 FOREIGN KEY(workspace_id,copy_id) REFERENCES lifecycle_fixture_copy(workspace_id,id),
 UNIQUE(workspace_id,id)
);
CREATE TABLE lifecycle_storage_observation (
 id uuid PRIMARY KEY,
 workspace_id uuid NOT NULL,
 call_id uuid NOT NULL,
 observed_at timestamptz NOT NULL,
 invoked_at timestamptz,
 http_status integer NOT NULL CHECK(http_status=0 OR http_status BETWEEN 100 AND 599),
 request_ref varchar(256) NOT NULL,
 response_sha256 varchar(64) NOT NULL CHECK(response_sha256='' OR response_sha256 ~ '^[0-9a-f]{64}$'),
 failure varchar(16) NOT NULL CHECK(failure IN ('','TRANSPORT','CONTRACT','JOURNAL')),
 proof_sha256 varchar(64) NOT NULL CHECK(proof_sha256 ~ '^[0-9a-f]{64}$'),
 FOREIGN KEY(workspace_id,call_id) REFERENCES lifecycle_storage_call(workspace_id,id),
 UNIQUE(call_id,proof_sha256),
 CHECK(invoked_at IS NULL OR invoked_at <= observed_at),
 CHECK(http_status=0 OR invoked_at IS NOT NULL)
);
CREATE FUNCTION lifecycle_storage_observation_guard() RETURNS trigger AS $$
DECLARE started timestamptz;
BEGIN
 SELECT started_at INTO started FROM lifecycle_storage_call WHERE id=NEW.call_id AND workspace_id=NEW.workspace_id FOR UPDATE;
 IF started IS NULL OR NEW.observed_at < started OR NEW.invoked_at < started THEN
  RAISE EXCEPTION 'storage observation precedes exact call start';
 END IF;
 IF NEW.invoked_at IS NOT NULL AND EXISTS(SELECT 1 FROM lifecycle_storage_observation
  WHERE call_id=NEW.call_id AND invoked_at IS NOT NULL AND invoked_at <> NEW.invoked_at) THEN
  RAISE EXCEPTION 'one storage call cannot hide multiple physical invocations';
 END IF;
 RETURN NEW;
END; $$ LANGUAGE plpgsql;
CREATE TRIGGER lifecycle_storage_observation_scope BEFORE INSERT ON lifecycle_storage_observation
 FOR EACH ROW EXECUTE FUNCTION lifecycle_storage_observation_guard();
DO $$ DECLARE name text; BEGIN
 FOREACH name IN ARRAY ARRAY['lifecycle_storage_call','lifecycle_storage_observation'] LOOP
  EXECUTE format('CREATE TRIGGER %I BEFORE UPDATE OR DELETE ON %I FOR EACH ROW EXECUTE FUNCTION lifecycle_fixture_identity_immutable()',name||'_immutable',name);
  EXECUTE format('CREATE TRIGGER %I BEFORE TRUNCATE ON %I FOR EACH STATEMENT EXECUTE FUNCTION lifecycle_fixture_identity_immutable()',name||'_no_truncate',name);
 END LOOP;
END $$;

ALTER TABLE cost_allocation ADD COLUMN lifecycle_provider_attempt_id uuid REFERENCES lifecycle_provider_attempt(id);
ALTER TABLE cost_allocation ADD COLUMN lifecycle_storage_call_id uuid REFERENCES lifecycle_storage_call(id);
ALTER TABLE cost_allocation DROP CONSTRAINT ck_cost_allocation_single_subject;
ALTER TABLE cost_allocation ADD CONSTRAINT ck_cost_allocation_single_subject CHECK (
 num_nonnulls(delivery_operation_id,quality_assessment_id,quality_assessment_attempt_id,
 rights_declaration_id,rights_declaration_verification_id,rights_declaration_disposition_id,
 authorization_provenance_binding_id,authorization_provenance_binding_disposition_id,
 effective_rights_snapshot_id,dataset_certification_id,certification_disposition_id,
 annotation_review_attempt_id,annotation_engine_attempt_id,ingestion_invocation_id,external_provider_call_id,external_usage_call_id,lifecycle_provider_attempt_id,lifecycle_storage_call_id)=1
);
CREATE INDEX idx_cost_allocation_lifecycle ON cost_allocation(lifecycle_provider_attempt_id) WHERE lifecycle_provider_attempt_id IS NOT NULL;
CREATE INDEX idx_cost_allocation_lifecycle_storage ON cost_allocation(lifecycle_storage_call_id) WHERE lifecycle_storage_call_id IS NOT NULL;
CREATE FUNCTION lifecycle_cost_attempt_guard() RETURNS trigger AS $$
BEGIN
 IF NEW.lifecycle_provider_attempt_id IS NOT NULL AND NOT EXISTS(
  SELECT 1 FROM cost_event e JOIN lifecycle_provider_attempt a ON a.id=NEW.lifecycle_provider_attempt_id
  JOIN lifecycle_provider_observation o ON o.attempt_id=a.id AND o.certainty='CONFIRMED'
  WHERE e.id=NEW.cost_event_id AND e.workspace_id=a.workspace_id AND e.activity_id=a.id
   AND e.cost_type='LIFECYCLE_PROVIDER_INVOCATION' AND e.quantity=1 AND e.unit='invocation'
   AND e.amount IS NULL AND e.pricing_mode='ACTUAL' AND e.occurred_at=o.invoked_at) THEN
  RAISE EXCEPTION 'lifecycle cost requires one confirmed physical invocation';
 END IF;
 IF NEW.lifecycle_storage_call_id IS NOT NULL AND NOT EXISTS(
  SELECT 1 FROM cost_event e JOIN lifecycle_storage_call a ON a.id=NEW.lifecycle_storage_call_id
  JOIN lifecycle_storage_observation o ON o.call_id=a.id AND o.invoked_at IS NOT NULL
  WHERE e.id=NEW.cost_event_id AND e.workspace_id=a.workspace_id AND e.activity_id=a.id
   AND e.cost_type='LIFECYCLE_PROVIDER_INVOCATION' AND e.quantity=1 AND e.unit='invocation'
   AND e.amount IS NULL AND e.pricing_mode='ACTUAL' AND e.occurred_at=o.invoked_at) THEN
  RAISE EXCEPTION 'lifecycle inspection cost requires a confirmed physical invocation';
 END IF;
 RETURN NEW;
END; $$ LANGUAGE plpgsql;
CREATE TRIGGER lifecycle_cost_attempt BEFORE INSERT ON cost_allocation FOR EACH ROW EXECUTE FUNCTION lifecycle_cost_attempt_guard();
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
 ELSIF NEW.lifecycle_storage_call_id IS NOT NULL THEN
  SELECT workspace_id INTO subject_workspace FROM lifecycle_storage_call WHERE id=NEW.lifecycle_storage_call_id;
 ELSIF NEW.lifecycle_provider_attempt_id IS NOT NULL THEN
  SELECT workspace_id INTO subject_workspace FROM lifecycle_provider_attempt WHERE id=NEW.lifecycle_provider_attempt_id;
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
