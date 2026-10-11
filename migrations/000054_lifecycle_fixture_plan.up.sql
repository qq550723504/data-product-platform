-- D1 controlled fixture scope, immutable planning, use restriction, holds and
-- reader admission/termination. No destructive dispatch or provider call.
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
